package compact

import (
	"context"
	"maps"
)

// Replay reuses explicit scores for local experiments. It is not a model.
// It never contacts Jev and must not be used as a semantic quality benchmark.
type Replay struct{ scores map[string]Score }

// NewReplay copies supplied group scores. Missing scores fail safely in Engine.
func NewReplay(scores map[string]Score) *Replay {
	copyScores := make(map[string]Score, len(scores))
	for id, score := range scores {
		score.Loss = maps.Clone(score.Loss)
		if score.Keep != nil {
			keep := *score.Keep
			score.Keep = &keep
		}
		copyScores[id] = score
	}
	return &Replay{scores: copyScores}
}

// Name distinguishes replay output from live Jev output.
func (r *Replay) Name() string { return "replay" }

// Score returns independent copies of configured scores for requested groups.
func (r *Replay) Score(ctx context.Context, eval Evaluation) (map[string]Score, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make(map[string]Score, len(eval.Candidates))
	for _, c := range eval.Candidates {
		if score, ok := r.scores[c.ID]; ok {
			score.Loss = maps.Clone(score.Loss)
			if score.Keep != nil {
				keep := *score.Keep
				score.Keep = &keep
			}
			out[c.ID] = score
		}
	}
	return out, nil
}
