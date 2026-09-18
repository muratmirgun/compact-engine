package compact

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func FuzzCompactionInvariants(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6}, uint16(500), true)
	f.Add([]byte{7, 255, 128, 64}, uint16(1), false)
	f.Fuzz(func(t *testing.T, flags []byte, budget uint16, partial bool) {
		flags = flags[:min(len(flags), 24)]
		zero := 0
		req := Request{Goal: "fix cache", TargetTokens: 1 + int(budget), RecentMessages: &zero, AllowPartial: partial,
			Messages: []Message{{ID: "rules", Role: "system", Text: "Do not edit generated files."}}}
		scores := make(map[string]Score, len(flags))
		protected := map[string]bool{"rules": true}
		for i, flag := range flags {
			callID, resultID := fmt.Sprintf("call-%d", i), fmt.Sprintf("result-%d", i)
			call := Message{ID: callID, Role: "assistant", ToolCalls: []ToolCall{{ID: callID, Name: "read", Arguments: json.RawMessage(`{}`), SideEffect: flag&1 != 0}}}
			result := Message{ID: resultID, Role: "tool", ToolCallID: callID, Text: strings.Repeat("cache diagnostic line\n", 100), Pinned: flag&2 != 0, Unresolved: flag&4 != 0}
			req.Messages = append(req.Messages, call, result)
			protected[callID], protected[resultID] = flag&7 != 0, flag&7 != 0
			loss := float64(flag>>3) / 31
			scores[callID] = Score{Loss: map[string]float64{"extract": loss, "brief": loss, "reference": loss, "drop": loss}}
		}
		original, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		store := &memoryStore{}
		r, err := newTestEngine(t, NewReplay(scores), store).Compact(t.Context(), req)
		if err != nil {
			t.Fatal(err)
		}
		gotCount, err := (byteCounter{}).Count(r.Messages)
		if err != nil {
			t.Fatal(err)
		}
		if r.BudgetMet != (gotCount <= req.TargetTokens) || gotCount != r.Stats.OutputTokens || gotCount > r.Stats.InputTokens {
			t.Fatalf("Compact() budget invariant failed: target=%d stats=%+v counted=%d", req.TargetTokens, r.Stats, gotCount)
		}
		selected := make(map[string]Message, len(r.Messages))
		last := -1
		positions := make(map[string]int, len(req.Messages))
		for i, m := range req.Messages {
			positions[m.ID] = i
		}
		for _, m := range r.Messages {
			pos, ok := positions[m.ID]
			if !ok || pos <= last {
				t.Fatalf("Compact() changed message order at %q", m.ID)
			}
			last = pos
			selected[m.ID] = m
		}
		for _, m := range req.Messages {
			if protected[m.ID] && !reflect.DeepEqual(selected[m.ID], m) {
				t.Fatalf("Compact() changed protected message %q", m.ID)
			}
		}
		if _, err := collectGroups(Request{Goal: req.Goal, TargetTokens: req.TargetTokens, Messages: r.Messages, RecentMessages: &zero}); err != nil {
			t.Fatalf("Compact() broke call/result protocol: %v", err)
		}
		after, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		if string(original) != string(after) || !reflect.DeepEqual(store.saved, req) {
			t.Fatal("Compact() mutated input or archived a different request")
		}
		if !r.Applied && !reflect.DeepEqual(r.Messages, req.Messages) {
			t.Fatal("Compact() changed history without applied status")
		}
	})
}
