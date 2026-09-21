package jev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muratmirgun/compact-engine/compact"
)

func parallelEvaluation() compact.Evaluation {
	e := compact.Evaluation{Goal: "continue"}
	for i := range 6 {
		e.Candidates = append(e.Candidates, compact.Candidate{ID: fmt.Sprint(i), Preview: strings.Repeat("evidence ", 100), Complete: true})
	}
	return e
}
func respond(w http.ResponseWriter, r *http.Request) {
	var q request
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		w.WriteHeader(400)
		return
	}
	answers := map[string]any{}
	for k := range q.Questions {
		answers[k] = map[string]any{"type": "noul", "noul": 0.1}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
}
func TestParallelRequestsBoundedAndCancel(t *testing.T) {
	for _, cancelPass := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelPass), func(t *testing.T) {
			entered := make(chan struct{}, 8)
			release := make(chan struct{})
			var active, peak atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := active.Add(1)
				defer active.Add(-1)
				for old := peak.Load(); n > old; old = peak.Load() {
					if peak.CompareAndSwap(old, n) {
						break
					}
				}
				entered <- struct{}{}
				select {
				case <-release:
					respond(w, r)
				case <-r.Context().Done():
				}
			}))
			defer srv.Close()
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			c, err := New(Config{APIKey: "test", Endpoint: srv.URL, MaxRequestBytes: 2400, MaxConcurrent: 2, KeepScoring: true})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				scores, err := c.Score(ctx, parallelEvaluation())
				if err == nil && len(scores) != 6 {
					err = fmt.Errorf("got %d scores", len(scores))
				}
				done <- err
			}()
			for range 2 {
				select {
				case <-entered:
				case <-ctx.Done():
					unblock()
					t.Fatal("two requests did not overlap")
				}
			}
			if cancelPass {
				cancel()
			} else {
				unblock()
			}
			select {
			case err := <-done:
				if (err != nil) != cancelPass {
					t.Errorf("error=%v cancel=%v", err, cancelPass)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("request workers did not stop")
			}
			if peak.Load() != 2 {
				t.Errorf("peak=%d, want two", peak.Load())
			}
		})
	}
}
func TestTokenBudgetAndContextFit(t *testing.T) {
	c, err := New(Config{APIKey: "test", KeepScoring: true, MaxRequestTokens: 600, MaxRequestBytes: 24000})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	batches, err := c.batches(parallelEvaluation())
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range batches {
		n, err := c.counter.CountText(string(b.body))
		if err != nil || n > 600 {
			t.Fatalf("request tokens=%d error=%v", n, err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(respond))
	defer srv.Close()
	c.config.Endpoint = srv.URL
	eval := parallelEvaluation()
	eval.Context = strings.Repeat("old context 日本語 ", 10000)
	scores, stats, err := c.ScoreWithStats(t.Context(), eval)
	if err != nil || len(scores) != 6 || !stats.ContextShortened {
		t.Fatalf("scores=%d stats=%+v err=%v", len(scores), stats, err)
	}
}
func BenchmarkScoringBatches(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(10 * time.Millisecond); respond(w, r) }))
	defer srv.Close()
	for _, n := range []int{1, 2} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			c, err := New(Config{APIKey: "test", Endpoint: srv.URL, MaxRequestBytes: 2400, MaxConcurrent: n, KeepScoring: true})
			if err != nil {
				b.Fatal(err)
			}
			defer c.Close()
			eval := parallelEvaluation()
			b.ResetTimer()
			for b.Loop() {
				if _, err := c.Score(b.Context(), eval); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
