// Package server exposes compaction and archive recovery over HTTP.
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muratmirgun/compact-engine/archive"
	"github.com/muratmirgun/compact-engine/compact"
)

// Compactor is the HTTP boundary's minimal engine interface.
type Compactor interface {
	Compact(context.Context, compact.Request) (compact.Result, error)
}

// Reader supplies original snapshots and messages.
type Reader interface {
	Load(context.Context, string) (compact.Request, error)
	Recall(context.Context, string, string) (compact.Message, error)
}

// Config sets per-service limits. An empty Token is for trusted loopback only.
type Config struct {
	Token         string
	MaxBodyBytes  int64
	MaxConcurrent int
	Timeout       time.Duration
}

// Handler serves a single trusted agent or workspace, not isolated tenants.
type Handler struct {
	engine  Compactor
	archive Reader
	config  Config
	slots   chan struct{}
	routes  *http.ServeMux
}

// New builds a handler. Admission is bounded; overload returns 429 immediately.
func New(engine Compactor, store Reader, cfg Config) (*Handler, error) {
	if engine == nil || store == nil {
		return nil, errors.New("server: engine and archive are required")
	}
	if cfg.MaxBodyBytes == 0 {
		cfg.MaxBodyBytes = 8 << 20
	}
	if cfg.MaxConcurrent == 0 {
		cfg.MaxConcurrent = 4
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxBodyBytes < 1 || cfg.MaxConcurrent < 1 || cfg.MaxConcurrent > 256 || cfg.Timeout < 0 {
		return nil, errors.New("server: invalid limits")
	}
	// Capacity is the documented bound on active requests, not an unbounded queue.
	h := &Handler{engine: engine, archive: store, config: cfg, slots: make(chan struct{}, cfg.MaxConcurrent), routes: http.NewServeMux()}
	h.routes.HandleFunc("POST /v1/compact", h.compact)
	h.routes.HandleFunc("GET /v1/snapshots/{snapshot}", h.snapshot)
	h.routes.HandleFunc("GET /v1/snapshots/{snapshot}/messages/{message}", h.recall)
	return h, nil
}

// ServeHTTP authenticates all data routes and applies request deadlines.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if h.config.Token != "" {
		got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		want := sha256.Sum256([]byte("Bearer " + h.config.Token))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	case <-r.Context().Done():
		writeError(w, http.StatusRequestTimeout, "request canceled")
		return
	default:
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, "service is busy")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.config.Timeout)
	defer cancel()
	h.routes.ServeHTTP(w, r.WithContext(ctx))
}

func (h *Handler) compact(w http.ResponseWriter, r *http.Request) {
	if media := strings.Split(r.Header.Get("Content-Type"), ";")[0]; strings.TrimSpace(media) != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "use application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.config.MaxBodyBytes)
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body exceeds limit")
			return
		}
		writeError(w, http.StatusBadRequest, "cannot read request")
		return
	}
	if !utf8.Valid(data) {
		writeError(w, http.StatusBadRequest, "request must use utf-8")
		return
	}
	var req compact.Request
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request schema")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "expected one json object")
		return
	}
	result, err := h.engine.Compact(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) snapshot(w http.ResponseWriter, r *http.Request) {
	req, err := h.archive.Load(r.Context(), r.PathValue("snapshot"))
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (h *Handler) recall(w http.ResponseWriter, r *http.Request) {
	m, err := h.archive.Recall(r.Context(), r.PathValue("snapshot"), r.PathValue("message"))
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, compact.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, archive.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "request deadline or cancellation")
	default:
		writeError(w, http.StatusInternalServerError, "internal service error")
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// A disconnected client cannot receive a second error response.
	if _, err := fmt.Fprintln(w, string(data)); err != nil {
		return
	}
}
