package compact

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

type memoryStore struct {
	saved Request
	err   error
}

func (s *memoryStore) Save(_ context.Context, req Request) (string, error) {
	s.saved = req
	return strings.Repeat("a", 64), s.err
}

type byteCounter struct{}

func (byteCounter) Name() string { return "test-bytes" }
func (byteCounter) Count(ms []Message) (int, error) {
	n := 0
	for _, m := range ms {
		n += len(m.Text) + 32
		for _, c := range m.ToolCalls {
			n += len(c.Arguments) + len(c.Name) + 16
		}
	}
	return n, nil
}

type scorerFunc func(context.Context, Evaluation) (map[string]Score, error)

func (f scorerFunc) Name() string { return "test" }
func (f scorerFunc) Score(ctx context.Context, e Evaluation) (map[string]Score, error) {
	return f(ctx, e)
}

func fixture() Request {
	zero := 0
	return Request{Goal: "fix cache regression", TargetTokens: 1800, RecentMessages: &zero, Messages: []Message{
		{ID: "rules", Role: "system", Text: "Never edit generated files."},
		{ID: "goal", Role: "user", Text: "Fix cache regression."},
		{ID: "call-old", Role: "assistant", ToolCalls: []ToolCall{{ID: "old", Name: "read", Arguments: json.RawMessage(`{"path":"old.log"}`)}}},
		{ID: "result-old", Role: "tool", ToolCallID: "old", Text: strings.Repeat("completed unrelated progress\n", 400)},
		{ID: "call-current", Role: "assistant", ToolCalls: []ToolCall{{ID: "current", Name: "test", Arguments: json.RawMessage(`{"target":"cache"}`)}}},
		{ID: "result-current", Role: "tool", ToolCallID: "current", Text: "error: cache regression at cache.go:42", Unresolved: true},
	}}
}

func newTestEngine(t *testing.T, scorer Scorer, store *memoryStore) *Engine {
	t.Helper()
	e, err := New(scorer, byteCounter{}, store)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestCompactPreservesCriticalStateAndInput(t *testing.T) {
	t.Parallel()
	req := fixture()
	original, _ := json.Marshal(req)
	store := &memoryStore{}
	e := newTestEngine(t, NewReplay(map[string]Score{"call-old": {Relevance: .05, Detail: .05}}), store)
	r, err := e.Compact(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Applied || !r.BudgetMet || r.Stats.OutputTokens > req.TargetTokens {
		t.Fatalf("Compact() status=%s tokens=%d, want applied under %d", r.Status, r.Stats.OutputTokens, req.TargetTokens)
	}
	for _, id := range []string{"rules", "goal", "call-current", "result-current"} {
		found := false
		for _, m := range r.Messages {
			if m.ID == id {
				found = true
			}
		}
		if !found {
			t.Errorf("Compact() omitted protected message %q", id)
		}
	}
	if _, err := collectGroups(Request{Goal: req.Goal, Messages: r.Messages, TargetTokens: req.TargetTokens, RecentMessages: req.RecentMessages}); err != nil {
		t.Errorf("Compact() produced invalid protocol: %v", err)
	}
	if !reflect.DeepEqual(store.saved.Messages, req.Messages) {
		t.Error("Compact() archive differs from original messages")
	}
	for i := range r.Messages {
		if len(r.Messages[i].ToolCalls) > 0 {
			r.Messages[i].ToolCalls[0].Arguments[0] = '!'
		}
	}
	after, _ := json.Marshal(req)
	if string(after) != string(original) {
		t.Error("Compact() aliases caller input")
	}
}

func TestCompactFailsClosed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		scorer Scorer
		target int
		want   string
	}{
		{name: "unavailable scorer", scorer: scorerFunc(func(context.Context, Evaluation) (map[string]Score, error) { return nil, errors.New("network") }), target: 1800, want: "scoring_failed"},
		{name: "missing answers", scorer: NewReplay(map[string]Score{}), target: 1800, want: "scoring_failed"},
		{name: "nan probability", scorer: NewReplay(map[string]Score{"call-old": {Relevance: math.NaN()}}), target: 1800, want: "scoring_failed"},
		{name: "ambiguous", scorer: NewReplay(map[string]Score{"call-old": {Relevance: .5, Detail: .5}}), target: 1800, want: "budget_unmet"},
		{name: "full details needed", scorer: NewReplay(map[string]Score{"call-old": {Relevance: .05, Detail: .95}}), target: 1800, want: "budget_unmet"},
		{name: "protected overflow", scorer: NewReplay(nil), target: 1, want: "budget_unmet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := fixture()
			req.TargetTokens = tt.target
			r, err := newTestEngine(t, tt.scorer, &memoryStore{}).Compact(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != tt.want || r.Applied || r.BudgetMet || !reflect.DeepEqual(r.Messages, req.Messages) {
				t.Fatalf("Compact() = %s applied=%v met=%v, want %s with unchanged history", r.Status, r.Applied, r.BudgetMet, tt.want)
			}
		})
	}
}

func TestCompactSkipsScoringUnderBudget(t *testing.T) {
	t.Parallel()
	req := fixture()
	req.TargetTokens = 100000
	s := scorerFunc(func(context.Context, Evaluation) (map[string]Score, error) {
		t.Error("unexpected paid scorer call")
		return nil, nil
	})
	r, err := newTestEngine(t, s, &memoryStore{}).Compact(t.Context(), req)
	if err != nil || !r.BudgetMet || r.Applied {
		t.Fatalf("Compact()=%+v err=%v, want unchanged within budget", r.Stats, err)
	}
}

func TestGroupProtection(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"side effect", "pending", "dependency", "named group", "recent"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			req := fixture()
			switch kind {
			case "side effect":
				req.Messages[2].ToolCalls[0].SideEffect = true
			case "pending":
				req.Messages = append(req.Messages[:3], req.Messages[4:]...)
			case "dependency":
				req.Messages[5].DependsOn = []string{"result-old"}
			case "named group":
				req.Messages[3].Group = "work"
				req.Messages[5].Group = "work"
			case "recent":
				n := 4
				req.RecentMessages = &n
			}
			groups, err := collectGroups(req)
			if err != nil {
				t.Fatal(err)
			}
			for _, g := range groups {
				if g.id == "call-old" && !g.protected {
					t.Errorf("collectGroups(%s) left old call unprotected", kind)
				}
			}
		})
	}
}

func TestInvalidTranscripts(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"duplicate id", "orphan result", "duplicate result", "missing dependency", "invalid role", "invalid json", "future call", "negative recent", "unsafe id", "invalid tool utf8"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			req := fixture()
			switch kind {
			case "duplicate id":
				req.Messages[1].ID = "rules"
			case "orphan result":
				req.Messages[3].ToolCallID = "absent"
			case "duplicate result":
				req.Messages[5].ToolCallID = "old"
			case "missing dependency":
				req.Messages[3].DependsOn = []string{"absent"}
			case "invalid role":
				req.Messages[0].Role = "unknown"
			case "invalid json":
				req.Messages[2].ToolCalls[0].Arguments = json.RawMessage(`{`)
			case "future call":
				req.Messages[2], req.Messages[3] = req.Messages[3], req.Messages[2]
			case "negative recent":
				n := -1
				req.RecentMessages = &n
			case "unsafe id":
				req.Messages[0].ID = "id\ninjected"
			case "invalid tool utf8":
				req.Messages[2].ToolCalls[0].Arguments = []byte{'"', 255, '"'}
			}
			_, err := newTestEngine(t, NewReplay(nil), &memoryStore{}).Compact(t.Context(), req)
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("Compact(%s) err=%v, want ErrInvalid", kind, err)
			}
		})
	}
}

func TestScorerSeesActualResult(t *testing.T) {
	t.Parallel()
	req := fixture()
	req.Messages[3].Text = strings.Repeat("noise\n", 2000) + "\nerror UNIQUE_MIDDLE\n" + strings.Repeat("noise\n", 2000)
	s := scorerFunc(func(_ context.Context, e Evaluation) (map[string]Score, error) {
		if len(e.Candidates) != 1 || !strings.Contains(e.Candidates[0].Preview, "UNIQUE_MIDDLE") || e.Candidates[0].Complete {
			t.Errorf("Score() candidate = %+v, want marked partial preview with error", e.Candidates)
		}
		return map[string]Score{"call-old": {Relevance: .9, Detail: .9}}, nil
	})
	if _, err := newTestEngine(t, s, &memoryStore{}).Compact(t.Context(), req); err != nil {
		t.Fatal(err)
	}
}

func TestCancellationAndArchiveFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	e := newTestEngine(t, NewReplay(nil), &memoryStore{})
	if _, err := e.Compact(ctx, fixture()); !errors.Is(err, context.Canceled) {
		t.Errorf("Compact(canceled)=%v, want canceled", err)
	}
	failure := errors.New("disk full")
	e = newTestEngine(t, NewReplay(nil), &memoryStore{err: failure})
	if _, err := e.Compact(t.Context(), fixture()); !errors.Is(err, failure) {
		t.Errorf("Compact(disk full)=%v, want disk failure", err)
	}
}

func FuzzExcerpt(f *testing.F) {
	f.Add("Merhaba dünya 🧠\nerror: cache", 128)
	f.Add(strings.Repeat("a", 4000), 400)
	f.Fuzz(func(t *testing.T, text string, limit int) {
		if !utf8.ValidString(text) {
			t.Skip()
		}
		limit = 64 + int(uint(limit)%4096)
		got := excerpt(text, "cache", limit)
		if !utf8.ValidString(got) {
			t.Error("excerpt() broke utf-8")
		}
		if len(got) > limit {
			t.Errorf("excerpt() length=%d, want <=%d", len(got), limit)
		}
	})
}

func BenchmarkCompactReplay(b *testing.B) {
	req := fixture()
	e, err := New(NewReplay(map[string]Score{"call-old": {Relevance: .05, Detail: .05}}), byteCounter{}, &memoryStore{})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := e.Compact(b.Context(), req); err != nil {
			b.Fatal(err)
		}
	}
}
