package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muratmirgun/compact-engine/archive"
	"github.com/muratmirgun/compact-engine/compact"
	"github.com/muratmirgun/compact-engine/jev"
	"github.com/muratmirgun/compact-engine/server"
	"github.com/muratmirgun/compact-engine/token"
)

func TestHTTPJevArchiveRoundTrip(t *testing.T) {
	t.Parallel()
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Questions map[string]any `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		answers := map[string]any{}
		for key := range payload.Questions {
			answers[key] = map[string]any{"type": "noul", "noul": .05}
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"answers": answers}); err != nil {
			t.Error(err)
		}
	}))
	defer mock.Close()
	scorer, err := jev.New(jev.Config{APIKey: "test-key", Endpoint: mock.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer scorer.Close()
	store, err := archive.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	counter, err := token.New("o200k_base")
	if err != nil {
		t.Fatal(err)
	}
	engine, err := compact.New(scorer, counter, store)
	if err != nil {
		t.Fatal(err)
	}
	h, err := server.New(engine, store, server.Config{Token: "local-token"})
	if err != nil {
		t.Fatal(err)
	}
	zero := 0
	input := compact.Request{Goal: "fix cache", TargetTokens: 180, RecentMessages: &zero, Messages: []compact.Message{
		{ID: "user", Role: "user", Text: "fix cache; do not edit generated files"},
		{ID: "call", Role: "assistant", ToolCalls: []compact.ToolCall{{ID: "c1", Name: "read_log", Arguments: json.RawMessage(`{"path":"old.log"}`)}}},
		{ID: "result", Role: "tool", ToolCallID: "c1", Text: strings.Repeat("old unrelated log line\n", 2000)},
	}}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/compact", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer local-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("POST compact status=%d body=%s", w.Code, w.Body.String())
	}
	var result compact.Result
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Applied || !result.BudgetMet || result.Stats.OutputTokens > 180 {
		t.Fatalf("POST compact result=%+v", result)
	}
	get := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/snapshots/"+result.SnapshotID+"/messages/result", nil)
	get.Header.Set("Authorization", "Bearer local-token")
	got := httptest.NewRecorder()
	h.ServeHTTP(got, get)
	var original compact.Message
	if err := json.Unmarshal(got.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}
	if got.Code != 200 || original.Text != input.Messages[2].Text {
		t.Error("GET recall did not restore exact original text")
	}
	get.Header.Del("Authorization")
	unauthorized := httptest.NewRecorder()
	h.ServeHTTP(unauthorized, get)
	if unauthorized.Code != 401 {
		t.Errorf("GET recall without auth=%d, want 401", unauthorized.Code)
	}
}

type fakeEngine struct{}

func (fakeEngine) Compact(context.Context, compact.Request) (compact.Result, error) {
	return compact.Result{Status: "unchanged"}, nil
}

type blockingEngine struct {
	started chan struct{}
	release chan struct{}
}

func (e blockingEngine) Compact(ctx context.Context, _ compact.Request) (compact.Result, error) {
	close(e.started)
	select {
	case <-e.release:
		return compact.Result{Status: "unchanged"}, nil
	case <-ctx.Done():
		return compact.Result{}, ctx.Err()
	}
}

func TestAdmissionRejectsOverload(t *testing.T) {
	t.Parallel()
	store, err := archive.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := blockingEngine{started: make(chan struct{}), release: make(chan struct{})}
	h, err := server.New(e, store, server.Config{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request := func() *http.Request {
		r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/compact", strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		return r
	}
	first := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); h.ServeHTTP(first, request()) }()
	<-e.started
	second := httptest.NewRecorder()
	h.ServeHTTP(second, request())
	close(e.release)
	<-done
	if first.Code != 200 || second.Code != 429 {
		t.Errorf("admission first=%d second=%d, want 200/429", first.Code, second.Code)
	}
}

func TestHTTPBoundaries(t *testing.T) {
	t.Parallel()
	store, err := archive.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h, err := server.New(fakeEngine{}, store, server.Config{Token: "token", MaxBodyBytes: 128, MaxConcurrent: 32})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, body, contentType, auth string
		status                        int
	}{
		{name: "auth", body: `{}`, contentType: "application/json", status: 401},
		{name: "unknown field", body: `{"opaque_reasoning":"must not silently discard"}`, contentType: "application/json", auth: "Bearer token", status: 400},
		{name: "multiple objects", body: `{} {}`, contentType: "application/json", auth: "Bearer token", status: 400},
		{name: "oversize", body: strings.Repeat(" ", 129), contentType: "application/json", auth: "Bearer token", status: 413},
		{name: "wrong media", body: `{}`, contentType: "text/plain", auth: "Bearer token", status: 415},
		{name: "invalid utf8", body: string([]byte{255}), contentType: "application/json", auth: "Bearer token", status: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/compact", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", tt.contentType)
			r.Header.Set("Authorization", tt.auth)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Errorf("POST %s status=%d, want %d", tt.name, w.Code, tt.status)
			}
		})
	}
}

func BenchmarkHTTPReplay(b *testing.B) {
	store, err := archive.New(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	counter, err := token.New("o200k_base")
	if err != nil {
		b.Fatal(err)
	}
	engine, err := compact.New(compact.NewReplay(map[string]compact.Score{"call": {Relevance: .05, Detail: .05}}), counter, store)
	if err != nil {
		b.Fatal(err)
	}
	h, err := server.New(engine, store, server.Config{})
	if err != nil {
		b.Fatal(err)
	}
	text := strings.Repeat("completed old log\n", 1000)
	body := fmt.Sprintf(`{"goal":"fix cache","target_tokens":200,"recent_messages":0,"messages":[{"id":"u","role":"user","text":"fix cache"},{"id":"call","role":"assistant","text":"","tool_calls":[{"id":"c","name":"read","arguments":{}}]},{"id":"r","role":"tool","tool_call_id":"c","text":%q}]}`, text)
	b.ReportAllocs()
	for b.Loop() {
		r := httptest.NewRequestWithContext(b.Context(), http.MethodPost, "/v1/compact", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			b.Fatalf("status=%d", w.Code)
		}
	}
}
