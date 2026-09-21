package compact

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestIndividualResultsPreserveMixedCalls(t *testing.T) {
	t.Parallel()
	zero := 0
	req := Request{Goal: "continue", TargetTokens: 1, AllowPartial: true, RecentMessages: &zero, ReduceResultsIndividually: true, Messages: []Message{
		{ID: "calls", Role: "assistant", Text: "Keep this explanation", ToolCalls: []ToolCall{
			{ID: "read", Name: "read", Arguments: json.RawMessage(`{}`)},
			{ID: "write", Name: "write", Arguments: json.RawMessage(`{}`), SideEffect: true},
		}},
		{ID: "read-result", Role: "tool", ToolCallID: "read", Text: strings.Repeat("old output ", 1000)},
		{ID: "write-result", Role: "tool", ToolCallID: "write", Text: "changed file"},
	}}
	original, _ := json.Marshal(req)
	e := newTestEngine(t, scorerFunc(func(_ context.Context, eval Evaluation) (map[string]Score, error) {
		if len(eval.Candidates) != 1 || eval.Candidates[0].ID != "read-result" {
			t.Errorf("candidates=%+v, want read-result only", eval.Candidates)
		}
		for _, v := range eval.Candidates[0].Variants {
			if v.Action == "drop" {
				t.Error("result-only mode proposed a drop")
			}
		}
		return map[string]Score{"read-result": {Keep: &KeepScore{Call: 0, Result: 0}}}, nil
	}), &memoryStore{})
	r, err := e.Compact(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Applied || r.Stats.ReducedGroups != 1 {
		t.Fatalf("result=%+v, want one reduction", r.Stats)
	}
	if len(r.Messages) != 3 || !reflect.DeepEqual(r.Messages[0], req.Messages[0]) || !reflect.DeepEqual(r.Messages[2], req.Messages[2]) {
		t.Fatal("protected messages or call records changed")
	}
	if r.Messages[1].Text == req.Messages[1].Text {
		t.Error("read result unchanged")
	}
	after, _ := json.Marshal(req)
	if string(after) != string(original) {
		t.Error("input mutated")
	}
}

func TestIndividualResultProtection(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"pinned", "error", "recent", "dependency", "group", "pending sibling"} {
		t.Run(kind, func(t *testing.T) {
			req := fixture()
			req.ReduceResultsIndividually = true
			switch kind {
			case "pinned":
				req.Messages[2].Pinned = true
			case "error":
				req.Messages[3].Unresolved = true
			case "recent":
				n := 4
				req.RecentMessages = &n
			case "dependency":
				req.Messages[5].DependsOn = []string{"call-old"}
			case "group":
				req.Messages[2].Group = "explicit"
			case "pending sibling":
				req.Messages[2].ToolCalls = append(req.Messages[2].ToolCalls, ToolCall{ID: "pending", Name: "read", Arguments: json.RawMessage(`{}`)})
			}
			groups, err := collectGroups(req)
			if err != nil {
				t.Fatal(err)
			}
			for _, g := range groups {
				if g.id == "result-old" && g.protected != (kind != "pending sibling") {
					t.Errorf("protection=%v for %s", g.protected, kind)
				}
			}
		})
	}
}

func TestScoringContextIncludesChronology(t *testing.T) {
	t.Parallel()
	req := fixture()
	text := scoringContext(req.Messages, req.Goal)
	for _, expected := range []string{"call=old", "tool=read", "result call=old", "cache regression", "unresolved=true"} {
		if !strings.Contains(text, expected) {
			t.Errorf("context lacks %q", expected)
		}
	}
	if strings.Contains(text, strings.Repeat("completed unrelated progress", 20)) {
		t.Error("context includes full tool output")
	}
}
