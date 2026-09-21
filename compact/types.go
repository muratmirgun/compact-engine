// Package compact selects reversible context reductions for coding agents.
// It never calls tools and never replaces text with generated summaries.
package compact

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrInvalid marks an invalid transcript or request.
var ErrInvalid = errors.New("compact: invalid request")

// ToolCall describes a pending or completed tool invocation.
type ToolCall struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Arguments  json.RawMessage `json:"arguments"`
	SideEffect bool            `json:"side_effect,omitempty"`
}

// Message is a provider-neutral text message. IDs must be unique per transcript.
// Group and DependsOn express dependencies that cannot be removed separately.
// Mark unresolved errors and important decisions explicitly; they stay verbatim.
type Message struct {
	ID         string     `json:"id"`
	Role       string     `json:"role"`
	Text       string     `json:"text"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Pinned     bool       `json:"pinned,omitempty"`
	Unresolved bool       `json:"unresolved,omitempty"`
	Group      string     `json:"group,omitempty"`
	DependsOn  []string   `json:"depends_on,omitempty"`
}

// Request supplies the current goal, complete active history, and input budget.
// TargetTokens excludes tool definitions and the model's output budget.
// RecentMessages defaults to four; an explicit zero disables recency protection.
// AllowPartial permits reductions that still exceed TargetTokens.
type Request struct {
	// ReduceResultsIndividually retains call records and reduces eligible results separately.
	ReduceResultsIndividually bool      `json:"reduce_results_individually,omitempty"`
	Goal                      string    `json:"goal"`
	Messages                  []Message `json:"messages"`
	TargetTokens              int       `json:"target_tokens"`
	RecentMessages            *int      `json:"recent_messages,omitempty"`
	AllowPartial              bool      `json:"allow_partial,omitempty"`
}

// Candidate shows literal excerpts of a dependency group to a scorer.
type Candidate struct {
	ID       string    `json:"id"`
	Preview  string    `json:"preview"`
	Complete bool      `json:"complete"`
	Tokens   int       `json:"tokens"`
	Variants []Variant `json:"variants,omitempty"`
}

// Variant contains the exact proposed replacement for an entire group.
// An empty Messages slice means removal of the whole dependency group.
type Variant struct {
	Action   string    `json:"action"`
	Messages []Message `json:"messages"`
	Tokens   int       `json:"tokens"`
}

// Evaluation contains data, not instructions from the transcript.
type Evaluation struct {
	Goal       string      `json:"goal"`
	Context    string      `json:"context"`
	Candidates []Candidate `json:"candidates"`
}

// Score contains estimated loss for each proposed action. Lower means less loss.
// Loss must cover all proposals when non-nil. These estimates are not guarantees.
// Keep selects the call/result retention policy with a 0.5 threshold.
// Keep and Loss are mutually exclusive.
// Relevance and Detail support legacy scorers only when Keep and Loss are nil.
type Score struct {
	Keep      *KeepScore         `json:"keep,omitempty"`
	Relevance float64            `json:"relevance,omitempty"`
	Detail    float64            `json:"detail,omitempty"`
	Loss      map[string]float64 `json:"loss,omitempty"`
}

// KeepScore estimates whether a group still needs its calls or full results.
// A score of 0.5 or higher retains that information. Full results imply calls.
// Groups with multiple calls use the probability that any member needs retention.
type KeepScore struct {
	Call   float64 `json:"call"`
	Result float64 `json:"result"`
}

// Scorer ranks dependency groups. It must return every requested candidate ID.
type Scorer interface {
	Score(context.Context, Evaluation) (map[string]Score, error)
	Name() string
}

// Counter measures model-visible messages using a named encoding and framing.
// Counts must be additive across messages and safe for concurrent calls.
type Counter interface {
	Count([]Message) (int, error)
	Name() string
}

// Store persists original requests before any reduction or external scoring.
type Store interface {
	Save(context.Context, Request) (string, error)
}

// Decision records the final representation of one dependency group.
type Decision struct {
	GroupID    string   `json:"group_id"`
	MessageIDs []string `json:"message_ids"`
	Action     string   `json:"action"`
	Reason     string   `json:"reason"`
	Score      *Score   `json:"score,omitempty"`
}

// Stats separates local work from scoring latency. Durations are milliseconds.
type Stats struct {
	Scoring         ScoringStats `json:"scoring"`
	ProtectedGroups int          `json:"protected_groups"`
	KeptGroups      int          `json:"kept_groups"`
	ReducedGroups   int          `json:"reduced_groups"`
	DroppedGroups   int          `json:"dropped_groups"`
	RejectedGroups  int          `json:"rejected_groups"`
	// ProtectedTokens counts groups that must remain verbatim.
	ProtectedTokens int `json:"protected_tokens"`
	// CandidateOutputTokens reports the evaluated output even if not applied.
	CandidateOutputTokens int     `json:"candidate_output_tokens"`
	InputTokens           int     `json:"input_tokens"`
	OutputTokens          int     `json:"output_tokens"`
	Reduction             float64 `json:"reduction_ratio"`
	Counter               string  `json:"counter"`
	Scorer                string  `json:"scorer"`
	ScoringMillis         float64 `json:"scoring_ms"`
	TotalMillis           float64 `json:"total_ms"`
}

// Result reports whether a reduction was applied and whether the budget was met.
// Status partial requires AllowPartial and still exceeds the input budget.
// Scorer failures always retain the original history.
// SnapshotID allows recovery of every original message, including dropped ones.
type Result struct {
	Version    string           `json:"version"`
	Status     string           `json:"status"`
	Applied    bool             `json:"applied"`
	BudgetMet  bool             `json:"budget_met"`
	SnapshotID string           `json:"snapshot_id"`
	Messages   []Message        `json:"messages"`
	Decisions  []Decision       `json:"decisions"`
	Scores     map[string]Score `json:"scores,omitempty"`
	Warnings   []string         `json:"warnings"`
	Stats      Stats            `json:"stats"`
}

// ScoringStats describes one scoring pass; Requests counts planned HTTP batches.
type ScoringStats struct {
	Requests         int  `json:"requests"`
	ContextShortened bool `json:"context_shortened"`
}

// DiagnosticScorer supplies per-call statistics without a shared last-result field.
type DiagnosticScorer interface {
	Scorer
	ScoreWithStats(context.Context, Evaluation) (map[string]Score, ScoringStats, error)
}
