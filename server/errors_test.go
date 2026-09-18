package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muratmirgun/compact-engine/archive"
	"github.com/muratmirgun/compact-engine/compact"
	"github.com/muratmirgun/compact-engine/server"
)

type failingEngine struct{ err error }

func (e failingEngine) Compact(context.Context, compact.Request) (compact.Result, error) {
	return compact.Result{}, e.err
}

func TestHTTPErrorContract(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{name: "invalid", err: compact.ErrInvalid, status: 400},
		{name: "not found", err: archive.ErrNotFound, status: 404},
		{name: "canceled", err: context.Canceled, status: 504},
		{name: "deadline", err: context.DeadlineExceeded, status: 504},
		{name: "internal", err: errors.New("private-path-and-content"), status: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, err := archive.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			h, err := server.New(failingEngine{err: tc.err}, store, server.Config{})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/compact", strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || body["error"] == "" {
				t.Errorf("POST(%s) status=%d body=%v, want %d with error", tc.name, w.Code, body, tc.status)
			}
			if strings.Contains(w.Body.String(), "private-path-and-content") {
				t.Error("POST(internal) exposed private error details")
			}
		})
	}
}

func TestSnapshotRoutesAndHealth(t *testing.T) {
	t.Parallel()
	store, err := archive.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := compact.Request{Goal: "restore", TargetTokens: 100, Messages: []compact.Message{{ID: "u", Role: "user", Text: "original"}}}
	id, err := store.Save(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	h, err := server.New(fakeEngine{}, store, server.Config{Token: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path   string
		auth   bool
		status int
	}{
		{path: "/healthz", status: 200},
		{path: "/v1/snapshots/" + id, auth: true, status: 200},
		{path: "/v1/snapshots/" + id, status: 401},
		{path: "/v1/snapshots/" + strings.Repeat("0", 64), auth: true, status: 404},
		{path: "/v1/snapshots/" + id + "/messages/missing", auth: true, status: 404},
	} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, nil)
		if tc.auth {
			r.Header.Set("Authorization", "Bearer test-token")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("GET(%s) status=%d cache=%q, want %d no-store", tc.path, w.Code, w.Header().Get("Cache-Control"), tc.status)
		}
		if tc.auth && tc.status == 200 {
			var got compact.Request
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got.Messages) != 1 || got.Messages[0].Text != "original" {
				t.Errorf("GET(snapshot)=%+v, want original request", got)
			}
		}
	}
}
