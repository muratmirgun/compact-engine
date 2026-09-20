package compact

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
)

const maxLoss = 0.2

func (e *Engine) variants(messages []Message, goal, snapshot string, originalTokens int) ([]Variant, error) {
	variants := make([]Variant, 0, 4)
	for _, action := range []string{"extract", "brief", "reference", "drop"} {
		if action == "drop" && containsAssistantText(messages) {
			continue
		}
		replacement := represent(messages, action, goal, snapshot)
		count, err := e.counter.Count(replacement)
		if err != nil {
			return nil, err
		}
		if count < originalTokens {
			variants = append(variants, Variant{Action: action, Messages: replacement, Tokens: count})
		}
	}
	return variants, nil
}

type selection struct {
	messages []Message
	tokens   int
	action   string
	loss     float64
}

func (e *Engine) selectMessages(ctx context.Context, req Request, groups []group, scores map[string]Score, eval Evaluation) ([]Message, []Decision, error) {
	proposals := make(map[string][]Variant, len(eval.Candidates))
	for _, c := range eval.Candidates {
		proposals[c.ID] = c.Variants
	}
	selected := make([]selection, len(groups))
	total := 0
	for i, g := range groups {
		messages := groupMessages(req.Messages, g)
		count, err := e.counter.Count(messages)
		if err != nil {
			return nil, nil, err
		}
		selected[i] = selection{messages: messages, tokens: count, action: "keep"}
		total += count
	}
	// Recompute marginal cost after each choice. Ranking only against originals
	// incorrectly prices transitions from an already reduced representation.
	for total > req.TargetTokens {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		bestGroup, bestWeight := -1, math.Inf(1)
		var best selection
		for i, g := range groups {
			if g.protected {
				continue
			}
			for _, v := range proposals[g.id] {
				loss, allowed := actionLoss(scores[g.id], v.Action)
				if !allowed || v.Tokens >= selected[i].tokens {
					continue
				}
				weight := (0.001 + math.Max(0, loss-selected[i].loss)) / float64(selected[i].tokens-v.Tokens)
				if weight < bestWeight {
					bestGroup, bestWeight = i, weight
					best = selection{messages: v.Messages, tokens: v.Tokens, action: v.Action, loss: loss}
				}
			}
		}
		if bestGroup < 0 {
			break
		}
		total -= selected[bestGroup].tokens - best.tokens
		selected[bestGroup] = best
	}
	return assemble(req, groups, selected, scores)
}

func actionLoss(score Score, action string) (float64, bool) {
	if score.Loss != nil {
		loss, ok := score.Loss[action]
		return loss, ok && loss < maxLoss
	}
	// Legacy replay fixtures and custom scorers retain their original gates.
	if score.Detail >= maxLoss || (action == "drop" && score.Relevance >= maxLoss) {
		return 0, false
	}
	switch action {
	case "extract":
		return 0.01 + 0.05*score.Detail, true
	case "brief":
		return 0.02 + 0.1*score.Detail, true
	case "reference":
		return 0.05 + 0.2*score.Detail, true
	case "drop":
		return 0.1 + 0.2*(score.Relevance+score.Detail), true
	default:
		return 0, false
	}
}

func assemble(req Request, groups []group, selected []selection, scores map[string]Score) ([]Message, []Decision, error) {
	byID := make(map[string]Message, len(req.Messages))
	decisions := make([]Decision, 0, len(groups))
	for i, g := range groups {
		ids := make([]string, 0, len(g.indices))
		for _, j := range g.indices {
			ids = append(ids, req.Messages[j].ID)
		}
		d := Decision{GroupID: g.id, MessageIDs: ids, Action: selected[i].action, Reason: g.reason}
		if !g.protected {
			s := scores[g.id]
			d.Score = &s
			d.Reason = "budget selection using per-action loss estimates"
			if s.Loss == nil {
				d.Reason = "budget selection using legacy probability gates"
			}
		}
		decisions = append(decisions, d)
		for _, m := range selected[i].messages {
			byID[m.ID] = m
		}
	}
	// Dependencies may join nonadjacent messages. Keep transcript order intact.
	messages := make([]Message, 0, len(byID))
	for _, m := range req.Messages {
		if replacement, ok := byID[m.ID]; ok {
			messages = append(messages, replacement)
		}
	}
	return messages, decisions, nil
}

func represent(messages []Message, action, goal, snapshot string) []Message {
	if action == "drop" {
		return []Message{}
	}
	output := slices.Clone(messages)
	for i, m := range output {
		if m.Role != "tool" {
			continue
		}
		ref := fmt.Sprintf("[archived original: snapshot=%s message=%s]", snapshot, m.ID)
		switch action {
		case "extract":
			output[i].Text = excerpt(m.Text, goal, 1600) + "\n" + ref
		case "brief":
			// Literal evidence only: no inferred exit status or generated summary.
			lines := strings.Count(m.Text, "\n") + 1
			output[i].Text = fmt.Sprintf("[output: %d bytes, %d lines; selected literal evidence]\n%s\n%s", len(m.Text), lines, excerpt(m.Text, goal, 320), ref)
		case "reference":
			output[i].Text = ref
		}
	}
	return output
}

func containsAssistantText(messages []Message) bool {
	for _, m := range messages {
		if m.Role == "assistant" && strings.TrimSpace(m.Text) != "" {
			return true
		}
	}
	return false
}
