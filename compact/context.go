package compact

import (
	"fmt"
	"strings"
)

// scoringContext retains chronological call identities and result metadata.
// Shortening affects scoring evidence only, never the returned conversation.
func scoringContext(messages []Message, goal string) string {
	for _, limit := range []int{800, 240, 60} {
		var b strings.Builder
		for _, m := range messages {
			fmt.Fprintf(&b, "[%s %s]\n", m.ID, m.Role)
			if m.Role == "tool" {
				fmt.Fprintf(&b, "result call=%s bytes=%d unresolved=%t (content omitted)\n", m.ToolCallID, len(m.Text), m.Unresolved)
			} else if m.Text != "" {
				b.WriteString(excerpt(m.Text, goal, limit))
				b.WriteByte('\n')
			}
			for _, c := range m.ToolCalls {
				fmt.Fprintf(&b, "call=%s tool=%s side_effect=%t input=%s\n", c.ID, c.Name, c.SideEffect, excerpt(string(c.Arguments), goal, limit))
			}
		}
		if b.Len() <= 12000 {
			return b.String()
		}
		if limit == 60 {
			return excerpt(b.String(), goal, 12000)
		}
	}
	return ""
}
