package compact

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// Engine combines a scorer, token counter, and durable archive.
// Concurrent use is safe when the injected dependencies are concurrency-safe.
type Engine struct {
	scorer  Scorer
	counter Counter
	store   Store
}

// New builds an engine. All dependencies are required, including the archive.
func New(scorer Scorer, counter Counter, store Store) (*Engine, error) {
	if scorer == nil || counter == nil || store == nil {
		return nil, fmt.Errorf("%w: scorer, counter and store are required", ErrInvalid)
	}
	return &Engine{scorer: scorer, counter: counter, store: store}, nil
}

// Compact archives the original input, then selects whole dependency groups.
// It never mutates req. Errors never contain a partially rewritten transcript.
func (e *Engine) Compact(ctx context.Context, req Request) (Result, error) {
	start := time.Now()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	groups, err := collectGroups(req)
	if err != nil {
		return Result{}, err
	}
	req, err = cloneRequest(req)
	if err != nil {
		return Result{}, err
	}
	before, err := e.counter.Count(req.Messages)
	if err != nil {
		return Result{}, fmt.Errorf("count input: %w", err)
	}
	snapshot, err := e.store.Save(ctx, req)
	if err != nil {
		return Result{}, fmt.Errorf("archive input: %w", err)
	}
	result := Result{
		Version: "1", Status: "unchanged", SnapshotID: snapshot,
		Messages: req.Messages, Decisions: []Decision{}, Warnings: []string{},
		Stats: Stats{InputTokens: before, OutputTokens: before, CandidateOutputTokens: before, Counter: e.counter.Name(), Scorer: e.scorer.Name()},
	}
	finish := func() Result {
		result.Stats.TotalMillis = float64(time.Since(start)) / float64(time.Millisecond)
		return result
	}
	if before <= req.TargetTokens {
		result.BudgetMet = true
		return finish(), nil
	}
	protected := make([]Message, 0)
	for _, g := range groups {
		if g.protected {
			protected = append(protected, groupMessages(req.Messages, g)...)
		}
	}
	minimum, err := e.counter.Count(protected)
	if err != nil {
		return Result{}, err
	}
	result.Stats.ProtectedTokens = minimum
	if minimum > req.TargetTokens && !req.AllowPartial {
		result.Status = "budget_unmet"
		result.Warnings = append(result.Warnings, "protected messages exceed target; original history retained")
		return finish(), nil
	}
	evaluation, err := e.evaluate(req, groups, snapshot)
	if err != nil {
		return Result{}, err
	}
	if len(evaluation.Candidates) == 0 {
		result.Status = "budget_unmet"
		result.Warnings = append(result.Warnings, "no reducible tool outputs; protected conversation retained")
		return finish(), nil
	}
	scoreStart := time.Now()
	scores, scoreErr := e.scorer.Score(ctx, evaluation)
	result.Stats.ScoringMillis = float64(time.Since(scoreStart)) / float64(time.Millisecond)
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if scoreErr == nil {
		scoreErr = validateScores(evaluation.Candidates, scores)
	}
	if scoreErr != nil {
		result.Status = "scoring_failed"
		result.Warnings = append(result.Warnings, "scoring failed or returned incomplete decisions; original history retained")
		return finish(), nil
	}
	result.Scores = scores
	selected, decisions, err := e.selectMessages(ctx, req, groups, scores, evaluation)
	if err != nil {
		return Result{}, err
	}
	after, err := e.counter.Count(selected)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	result.Stats.CandidateOutputTokens = after
	if after > req.TargetTokens && (!req.AllowPartial || after >= before) {
		result.Status = "budget_unmet"
		reason := "safe reductions cannot meet target; original history retained"
		if after >= before {
			reason = "no proposed reduction passed the scoring policy; original history retained"
		}
		result.Warnings = append(result.Warnings, reason)
		return finish(), nil
	}
	result.Status, result.Applied, result.BudgetMet = "compacted", true, after <= req.TargetTokens
	if !result.BudgetMet {
		result.Status = "partial"
		result.Warnings = append(result.Warnings, "reductions applied but target not met; further budget handling required")
	}
	result.Messages, result.Decisions = selected, decisions
	result.Stats.OutputTokens = after
	result.Stats.Reduction = 1 - float64(after)/float64(before)
	return finish(), nil
}

func (e *Engine) evaluate(req Request, groups []group, snapshot string) (Evaluation, error) {
	eval := Evaluation{Goal: req.Goal, Candidates: []Candidate{}}
	// Global context is explicitly sampled; it is not a claim of complete history.
	var contextText strings.Builder
	for _, m := range req.Messages {
		if m.Role != "tool" && m.Text != "" {
			fmt.Fprintf(&contextText, "[%s %s]\n%s\n", m.ID, m.Role, excerpt(m.Text, req.Goal, 800))
		}
	}
	eval.Context = excerpt(contextText.String(), req.Goal, 4000)
	for _, g := range groups {
		if g.protected {
			continue
		}
		messages := groupMessages(req.Messages, g)
		count, err := e.counter.Count(messages)
		if err != nil {
			return Evaluation{}, err
		}
		var b strings.Builder
		for _, m := range messages {
			fmt.Fprintf(&b, "[%s %s call=%s]\n", m.ID, m.Role, m.ToolCallID)
			for _, c := range m.ToolCalls {
				fmt.Fprintf(&b, "tool=%s arguments=%s\n", c.Name, c.Arguments)
			}
			b.WriteString(m.Text)
			b.WriteByte('\n')
		}
		full := b.String()
		variants, err := e.variants(messages, req.Goal, snapshot, count)
		if err != nil {
			return Evaluation{}, err
		}
		eval.Candidates = append(eval.Candidates, Candidate{ID: g.id, Preview: excerpt(full, req.Goal, 16000), Complete: len(full) <= 16000, Tokens: count, Variants: variants})
	}
	return eval, nil
}

func groupMessages(messages []Message, g group) []Message {
	out := make([]Message, 0, len(g.indices))
	for _, i := range g.indices {
		out = append(out, messages[i])
	}
	return out
}

func validateScores(candidates []Candidate, scores map[string]Score) error {
	for _, c := range candidates {
		s, ok := scores[c.ID]
		if !ok {
			return fmt.Errorf("missing score for %q", c.ID)
		}
		if s.Keep != nil && s.Loss != nil {
			return fmt.Errorf("ambiguous scoring policy for %q", c.ID)
		}
		probabilities := []float64{s.Relevance, s.Detail}
		if s.Keep != nil {
			probabilities = []float64{s.Keep.Call, s.Keep.Result}
		}
		if s.Loss != nil {
			probabilities = make([]float64, 0, len(c.Variants))
			for _, v := range c.Variants {
				p, ok := s.Loss[v.Action]
				if !ok {
					return fmt.Errorf("missing loss for %q action %q", c.ID, v.Action)
				}
				probabilities = append(probabilities, p)
			}
		}
		for _, p := range probabilities {
			if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
				return fmt.Errorf("invalid probability for %q", c.ID)
			}
		}
	}
	return nil
}

func cloneRequest(req Request) (Request, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return Request{}, fmt.Errorf("%w: encode transcript: %v", ErrInvalid, err)
	}
	var clone Request
	if err := json.Unmarshal(data, &clone); err != nil {
		return Request{}, fmt.Errorf("%w: decode transcript: %v", ErrInvalid, err)
	}
	return clone, nil
}
