package jev

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muratmirgun/compact-engine/compact"
)

func TestClientScoresConcreteVariants(t *testing.T) {
	t.Parallel()
	for _, omit := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "missing variant"}[omit], func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req request
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var state compact.Evaluation
				if err := json.Unmarshal([]byte(req.State), &state); err != nil {
					t.Error(err)
					return
				}
				if len(state.Candidates) != 1 || len(state.Candidates[0].Variants) != 2 {
					t.Error("Score() request omitted concrete replacements")
					return
				}
				if got := state.Candidates[0].Variants[0].Messages[0].Text; got != "error CACHE_EVIDENCE" {
					t.Errorf("Score() replacement=%q, want exact evidence", got)
				}
				if len(req.Questions) != 2 || !strings.Contains(req.Questions["l0_0"].Instructions, `"brief"`) {
					t.Error("Score() did not ask action-specific questions")
				}
				answers := map[string]any{"l0_0": map[string]any{"type": "noul", "noul": .08}}
				if !omit {
					answers["l0_1"] = map[string]any{"type": "noul", "noul": .92}
				}
				if err := json.NewEncoder(w).Encode(map[string]any{"answers": answers}); err != nil {
					t.Error(err)
				}
			}))
			defer srv.Close()
			client, err := New(Config{APIKey: "test-key", Endpoint: srv.URL})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			eval := compact.Evaluation{Goal: "fix cache", Candidates: []compact.Candidate{{
				ID: "old", Preview: "full error CACHE_EVIDENCE", Complete: true,
				Variants: []compact.Variant{
					{Action: "brief", Messages: []compact.Message{{ID: "result", Role: "tool", Text: "error CACHE_EVIDENCE"}}},
					{Action: "drop", Messages: []compact.Message{}},
				},
			}}}
			scores, err := client.Score(t.Context(), eval)
			if omit {
				if err == nil {
					t.Error("Score(missing variant) succeeded, want failure")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scores["old"].Loss["brief"] != .08 || scores["old"].Loss["drop"] != .92 {
				t.Errorf("Score()=%+v, want independently decoded losses", scores)
			}
		})
	}
}
