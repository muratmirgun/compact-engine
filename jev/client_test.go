package jev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/muratmirgun/compact-engine/compact"
)

func TestClientBatchesAndSendsContent(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing authorization")
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		var state compact.Evaluation
		if err := json.Unmarshal([]byte(req.State), &state); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if state.Goal != "fix cache" {
			t.Errorf("state goal=%q", state.Goal)
		}
		answers := make(map[string]any, len(req.Questions))
		for key := range req.Questions {
			answers[key] = map[string]any{"type": "noul", "noul": 0.1}
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"answers": answers}); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	c, err := New(Config{APIKey: "test-key", Endpoint: srv.URL, MaxRequestBytes: 2400})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	eval := compact.Evaluation{Goal: "fix cache", Context: "protect instructions", Candidates: []compact.Candidate{}}
	for i := range 8 {
		eval.Candidates = append(eval.Candidates, compact.Candidate{ID: strconv.Itoa(i), Preview: "error actual result", Complete: true})
	}
	scores, err := c.Score(t.Context(), eval)
	if err != nil {
		t.Fatal(err)
	}
	if len(scores) != 8 || calls.Load() < 2 {
		t.Errorf("Score() scores=%d requests=%d, want 8 and multiple batches", len(scores), calls.Load())
	}
	for id, s := range scores {
		if s.Relevance != .1 || s.Detail != .1 {
			t.Errorf("score %s=%+v", id, s)
		}
	}
}

func TestClientRejectsMalformedResponses(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`{}`, `{"answers":{"r0":{"type":"noul","noul":0.1}}}`, `{"answers":{"r0":{"type":"noul","noul":2},"d0":{"type":"noul","noul":0.1}}}`, `{"answers":{"r0":{"type":"choice","noul":0.1},"d0":{"type":"noul","noul":0.1}}}`, `not json`, strings.Repeat(" ", 1<<20+1)} {
		t.Run(fmt.Sprintf("bytes=%d", len(body)), func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, body) }))
			defer srv.Close()
			c, err := New(Config{APIKey: "test-key", Endpoint: srv.URL})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_, err = c.Score(t.Context(), compact.Evaluation{Goal: "test", Candidates: []compact.Candidate{{ID: "one"}}})
			if err == nil {
				t.Error("Score(malformed) succeeded")
			}
		})
	}
}

func TestClientNoRedirectOrRetry(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(status)
			}))
			defer srv.Close()
			c, err := New(Config{APIKey: "test-key", Endpoint: srv.URL})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_, err = c.Score(t.Context(), compact.Evaluation{Goal: "test", Candidates: []compact.Candidate{{ID: "one"}}})
			if err == nil || calls.Load() != 1 {
				t.Errorf("Score(status=%d) calls=%d err=%v, want one failed call", status, calls.Load(), err)
			}
		})
	}
}

func TestClientBudgetAndCancellation(t *testing.T) {
	t.Parallel()
	c, err := New(Config{APIKey: "test-key", MaxRequestBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	eval := compact.Evaluation{Goal: strings.Repeat("goal ", 1000), Candidates: []compact.Candidate{{ID: "one"}}}
	if _, err := c.Score(t.Context(), eval); err == nil {
		t.Error("Score(oversize) succeeded")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	c.config.MaxRequestBytes = 24000
	if _, err := c.Score(ctx, compact.Evaluation{Goal: "test", Candidates: []compact.Candidate{{ID: "one"}}}); !errors.Is(err, context.Canceled) {
		t.Errorf("Score(canceled)=%v", err)
	}
}

func TestOversizedCandidateSplitsExactVariants(t *testing.T) {
	t.Parallel()
	for _, incomplete := range []bool{false, true} {
		t.Run(fmt.Sprint(incomplete), func(t *testing.T) {
			var calls atomic.Int32
			variants := []compact.Variant{}
			for _, action := range []string{"extract", "brief", "reference"} {
				variants = append(variants, compact.Variant{Action: action, Messages: []compact.Message{{ID: "t", Role: "tool", ToolCallID: "call", Text: strings.Repeat(action+" evidence ", 35)}}})
			}
			eval := compact.Evaluation{Goal: "fix cache", Candidates: []compact.Candidate{{ID: "group", Preview: strings.Repeat("証拠", 3000), Complete: true, Variants: variants}}}
			before, _ := json.Marshal(eval)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(r.Body)
				if len(body) > 2400 {
					t.Error("request exceeded configured byte limit")
				}
				var req request
				if err := json.Unmarshal(body, &req); err != nil {
					t.Error(err)
					return
				}
				var state compact.Evaluation
				if err := json.Unmarshal([]byte(req.State), &state); err != nil {
					t.Error(err)
					return
				}
				candidate := state.Candidates[0]
				if candidate.Complete {
					t.Error("truncated evidence marked complete")
				}
				if len(candidate.Variants) != 1 {
					t.Error("expected one exact variant per request")
					return
				}
				v := candidate.Variants[0]
				for _, original := range variants {
					if original.Action == v.Action && !reflect.DeepEqual(original, v) {
						t.Error("replacement text was truncated")
					}
				}
				answers := map[string]any{}
				if !incomplete || v.Action != "reference" {
					answers[lossKey(0, 0)] = map[string]any{"type": "noul", "noul": .01}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
			}))
			defer srv.Close()
			client, err := New(Config{APIKey: "test", Endpoint: srv.URL, MaxRequestBytes: 2400})
			if err != nil {
				t.Fatal(err)
			}
			scores, err := client.Score(t.Context(), eval)
			if incomplete {
				if err == nil || scores != nil {
					t.Error("incomplete scores must fail closed")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if len(scores["group"].Loss) != 3 {
					t.Fatalf("lost split scores: %+v", scores)
				}
			}
			if calls.Load() != 3 {
				t.Errorf("requests=%d, want 3", calls.Load())
			}
			after, _ := json.Marshal(eval)
			if string(before) != string(after) {
				t.Error("input evaluation changed")
			}
		})
	}
}

func TestKeepScoringUsesTwoQuestions(t *testing.T) {
	t.Parallel()
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req request
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if len(req.Questions) != 2 || req.Questions["r0"].Type != "noul" || req.Questions["d0"].Type != "noul" {
					t.Error("expected separate call/result questions")
				}
				var state compact.Evaluation
				if err := json.Unmarshal([]byte(req.State), &state); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if state.Goal != "fix cache" || len(state.Candidates) != 1 {
					t.Error("missing goal or candidate")
				} else if len(state.Candidates[0].Variants) != 0 {
					t.Error("keep scoring sent replacement payloads")
				}
				answers := map[string]any{"r0": map[string]any{"type": "noul", "noul": .7}}
				if !missing {
					answers["d0"] = map[string]any{"type": "noul", "noul": .3}
				}
				if err := json.NewEncoder(w).Encode(map[string]any{"answers": answers}); err != nil {
					t.Error(err)
				}
			}))
			defer srv.Close()
			client, err := New(Config{APIKey: "test", Endpoint: srv.URL, KeepScoring: true})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			eval := compact.Evaluation{Goal: "fix cache", Candidates: []compact.Candidate{{ID: "a", Preview: "completed read", Variants: []compact.Variant{{Action: "brief"}}}}}
			scores, err := client.Score(t.Context(), eval)
			if missing {
				if err == nil {
					t.Fatal("missing result score accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scores["a"].Keep == nil || *scores["a"].Keep != (compact.KeepScore{Call: .7, Result: .3}) || scores["a"].Loss != nil {
				t.Fatalf("unexpected score: %+v", scores["a"])
			}
			if len(eval.Candidates[0].Variants) != 1 {
				t.Fatal("input mutated")
			}
		})
	}
}

func TestKeepScoringBoundsOversizedPreview(t *testing.T) {
	t.Parallel()
	client, err := New(Config{APIKey: "test", KeepScoring: true, MaxRequestBytes: 1600})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	eval := compact.Evaluation{Goal: "fix", Candidates: []compact.Candidate{{ID: "a", Preview: strings.Repeat("界", 10000), Complete: true}}}
	batches, err := client.batches(eval)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || len(batches[0].body) > 1600 {
		t.Fatal("oversized keep request")
	}
	if batches[0].candidates[0].Complete {
		t.Fatal("sampled preview claims completeness")
	}
	if len(eval.Candidates[0].Preview) != 30000 || !eval.Candidates[0].Complete {
		t.Fatal("caller input changed")
	}
	eval.Context = strings.Repeat("x", 5000)
	if _, err := client.batches(eval); err == nil {
		t.Fatal("oversized context accepted")
	}
}
