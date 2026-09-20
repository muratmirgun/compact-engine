package compact

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

type group struct {
	id        string
	indices   []int
	protected bool
	reason    string
}

// collectGroups also validates the provider-neutral protocol before any I/O.
func collectGroups(req Request) ([]group, error) {
	if strings.TrimSpace(req.Goal) == "" || len(req.Goal) > 16000 || req.TargetTokens < 1 {
		return nil, fmt.Errorf("%w: goal must contain 1..16000 bytes and target_tokens must be positive", ErrInvalid)
	}
	if len(req.Messages) == 0 || len(req.Messages) > 10000 || !utf8.ValidString(req.Goal) {
		return nil, fmt.Errorf("%w: require 1..10000 messages and utf-8 text", ErrInvalid)
	}
	recent := 4
	if req.RecentMessages != nil {
		recent = *req.RecentMessages
	}
	if recent < 0 {
		return nil, fmt.Errorf("%w: recent_messages cannot be negative", ErrInvalid)
	}
	parents := make([]int, len(req.Messages))
	ids := make(map[string]int, len(req.Messages))
	calls := make(map[string]int)
	results := make(map[string]int)
	named := make(map[string]int)
	for i, m := range req.Messages {
		parents[i] = i
		if !validMessageID(m.ID) {
			return nil, fmt.Errorf("%w: message %d has an invalid id", ErrInvalid, i)
		}
		if _, ok := ids[m.ID]; ok {
			return nil, fmt.Errorf("%w: duplicate message id %q", ErrInvalid, m.ID)
		}
		ids[m.ID] = i
		if !utf8.ValidString(m.Text) || !utf8.ValidString(m.Group) || !utf8.ValidString(m.ToolCallID) {
			return nil, fmt.Errorf("%w: message %q contains invalid utf-8", ErrInvalid, m.ID)
		}
		switch m.Role {
		case "system", "developer", "user", "assistant", "tool":
		default:
			return nil, fmt.Errorf("%w: unknown role %q", ErrInvalid, m.Role)
		}
		if (m.Role == "tool") != (m.ToolCallID != "") || (len(m.ToolCalls) > 0 && m.Role != "assistant") {
			return nil, fmt.Errorf("%w: inconsistent tool fields on %q", ErrInvalid, m.ID)
		}
		if m.ToolCallID != "" {
			if _, ok := results[m.ToolCallID]; ok {
				return nil, fmt.Errorf("%w: duplicate result for %q", ErrInvalid, m.ToolCallID)
			}
			results[m.ToolCallID] = i
		}
		for _, call := range m.ToolCalls {
			if call.ID == "" || call.Name == "" || !json.Valid(call.Arguments) || !utf8.Valid(call.Arguments) || !utf8.ValidString(call.ID) || !utf8.ValidString(call.Name) {
				return nil, fmt.Errorf("%w: invalid tool call on %q", ErrInvalid, m.ID)
			}
			if _, ok := calls[call.ID]; ok {
				return nil, fmt.Errorf("%w: duplicate tool call %q", ErrInvalid, call.ID)
			}
			calls[call.ID] = i
		}
	}
	for i, m := range req.Messages {
		for _, dep := range m.DependsOn {
			j, ok := ids[dep]
			if !ok {
				return nil, fmt.Errorf("%w: missing dependency %q", ErrInvalid, dep)
			}
			join(parents, i, j)
		}
		if m.Group != "" {
			if j, ok := named[m.Group]; ok {
				join(parents, i, j)
			}
			named[m.Group] = i
		}
	}
	for id, result := range results {
		call, ok := calls[id]
		if !ok || call >= result {
			return nil, fmt.Errorf("%w: result %q requires an earlier call", ErrInvalid, id)
		}
		join(parents, call, result)
	}
	groups := make([]group, 0, len(req.Messages))
	positions := make(map[int]int)
	for i, m := range req.Messages {
		root := find(parents, i)
		pos, ok := positions[root]
		if !ok {
			pos = len(groups)
			positions[root] = pos
			groups = append(groups, group{id: m.ID, indices: []int{}})
		}
		g := &groups[pos]
		g.indices = append(g.indices, i)
		if reason := protection(m, i >= len(req.Messages)-recent, results); reason != "" {
			g.protected = true
			g.reason = reason
		}
	}
	protectConversationGroups(groups, req.Messages)
	return groups, nil
}

func protection(m Message, isRecent bool, results map[string]int) string {
	switch {
	case m.Pinned:
		return "explicitly pinned"
	case m.Unresolved:
		return "unresolved work"
	case m.Role == "system" || m.Role == "developer" || m.Role == "user":
		return "instruction or user message"
	case isRecent:
		return "recent message"
	}
	for _, call := range m.ToolCalls {
		if call.SideEffect {
			return "side effect"
		}
		if _, ok := results[call.ID]; !ok {
			return "pending tool call"
		}
	}
	return ""
}

func find(parents []int, i int) int {
	for parents[i] != i {
		parents[i] = parents[parents[i]]
		i = parents[i]
	}
	return i
}

func join(parents []int, a, b int) {
	parents[find(parents, a)] = find(parents, b)
}

func validMessageID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		isLetter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		isDigit := r >= '0' && r <= '9'
		if !isLetter && !isDigit && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}

func protectConversationGroups(groups []group, messages []Message) {
	// Assistant narration must stay verbatim, but must not prevent reduction
	// of a completed read-only tool result in the same protocol group.
	for i := range groups {
		g := &groups[i]
		hasToolResult := false
		for _, index := range g.indices {
			hasToolResult = hasToolResult || messages[index].Role == "tool"
		}
		if !g.protected && !hasToolResult {
			g.protected, g.reason = true, "conversation text preserved"
		}
	}
}
