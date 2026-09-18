package compact

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestCompactUsesEvaluatedReplacement(t *testing.T) {
	t.Parallel()
	req := fixture()
	req.Messages[3].Text = strings.Repeat("noise\n", 1000) + "error CACHE_EVIDENCE at cache.go:42\n" + strings.Repeat("noise\n", 1000)
	var proposed []Message
	scorer := scorerFunc(func(_ context.Context, eval Evaluation) (map[string]Score, error) {
		c := eval.Candidates[0]
		loss := make(map[string]float64, len(c.Variants))
		for _, v := range c.Variants {
			loss[v.Action] = .99
			if v.Action == "brief" {
				loss[v.Action] = .05
				proposed = v.Messages
			}
		}
		// High legacy scores must not override a concrete low-loss replacement.
		return map[string]Score{c.ID: {Relevance: .99, Detail: .99, Loss: loss}}, nil
	})
	r, err := newTestEngine(t, scorer, &memoryStore{}).Compact(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Applied || !r.BudgetMet || len(proposed) != 2 {
		t.Fatalf("Compact(brief) = %s proposals=%d, want applied brief under budget", r.Status, len(proposed))
	}
	if !reflect.DeepEqual(r.Messages[2:4], proposed) {
		t.Error("Compact(brief) differs from replacement shown to scorer")
	}
	if !strings.Contains(r.Messages[3].Text, "CACHE_EVIDENCE") || !strings.Contains(r.Messages[3].Text, "archived original:") {
		t.Error("Compact(brief) lost selected evidence or recall reference")
	}
	for _, i := range []int{0, 1, 4, 5} {
		if !reflect.DeepEqual(req.Messages[i], r.Messages[i]) {
			t.Errorf("Compact(brief) changed protected message %q", req.Messages[i].ID)
		}
	}
}

func TestPartialBudgetRequiresOptIn(t *testing.T) {
	t.Parallel()
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "atomic", true: "partial"}[allow], func(t *testing.T) {
			t.Parallel()
			req := fixture()
			req.TargetTokens, req.AllowPartial = 1, allow
			scorer := NewReplay(map[string]Score{"call-old": {Loss: map[string]float64{"extract": .01, "brief": .02, "reference": .05, "drop": .09}}})
			r, err := newTestEngine(t, scorer, &memoryStore{}).Compact(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			if r.BudgetMet || r.Applied != allow {
				t.Errorf("Compact(allow_partial=%v) applied=%v budget_met=%v", allow, r.Applied, r.BudgetMet)
			}
			if !allow {
				if r.Status != "budget_unmet" || !reflect.DeepEqual(r.Messages, req.Messages) {
					t.Error("Compact(atomic overflow) did not retain original history")
				}
				return
			}
			if r.Status != "partial" || r.Stats.OutputTokens >= r.Stats.InputTokens || len(r.Decisions) == 0 {
				t.Errorf("Compact(partial overflow) status=%s stats=%+v, want documented reduction", r.Status, r.Stats)
			}
			want := append(append([]Message{}, req.Messages[:2]...), req.Messages[4:]...)
			if !reflect.DeepEqual(r.Messages, want) {
				t.Error("Compact(partial overflow) changed protected state or retained obsolete group")
			}
		})
	}
}

func TestVariantScoresFailClosed(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		loss   map[string]float64
		status string
	}{
		{name: "missing actions", loss: map[string]float64{"brief": .01}, status: "scoring_failed"},
		{name: "nan", loss: map[string]float64{"extract": math.NaN(), "brief": .01, "reference": .01, "drop": .01}, status: "scoring_failed"},
		{name: "out of bounds", loss: map[string]float64{"extract": .01, "brief": -1, "reference": .01, "drop": .01}, status: "scoring_failed"},
		{name: "ambiguous", loss: map[string]float64{"extract": .2, "brief": .3, "reference": .5, "drop": .9}, status: "budget_unmet"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := fixture()
			req.AllowPartial = true
			r, err := newTestEngine(t, NewReplay(map[string]Score{"call-old": {Loss: tt.loss}}), &memoryStore{}).Compact(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != tt.status || r.Applied || !reflect.DeepEqual(r.Messages, req.Messages) {
				t.Errorf("Compact(%s) status=%s applied=%v, want %s with original history", tt.name, r.Status, r.Applied, tt.status)
			}
		})
	}
}

func TestReplayCopiesLossMaps(t *testing.T) {
	t.Parallel()
	loss := map[string]float64{"brief": .1}
	replay := NewReplay(map[string]Score{"old": {Loss: loss}})
	loss["brief"] = .9
	eval := Evaluation{Candidates: []Candidate{{ID: "old"}}}
	first, err := replay.Score(t.Context(), eval)
	if err != nil {
		t.Fatal(err)
	}
	if first["old"].Loss["brief"] != .1 {
		t.Error("Replay.Score() aliases constructor input")
	}
	first["old"].Loss["brief"] = .8
	second, err := replay.Score(t.Context(), eval)
	if err != nil {
		t.Fatal(err)
	}
	if second["old"].Loss["brief"] != .1 {
		t.Error("Replay.Score() aliases previous result")
	}
}
