// Package token counts model-visible text with an explicit BPE encoding.
package token

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/tiktoken-go/tokenizer"

	"github.com/muratmirgun/compact-engine/compact"
)

// Counter owns a tokenizer. Its lock protects codecs that reuse internal state.
type Counter struct {
	mu       sync.Mutex
	codec    tokenizer.Codec
	encoding string
}

// New accepts o200k_base or cl100k_base. Vocabularies are compiled into Go.
func New(encoding string) (*Counter, error) {
	if encoding != "o200k_base" && encoding != "cl100k_base" {
		return nil, fmt.Errorf("unsupported encoding %q", encoding)
	}
	codec, err := tokenizer.Get(tokenizer.Encoding(encoding))
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}
	return &Counter{codec: codec, encoding: encoding}, nil
}

// Name identifies the exact accounting convention, not provider billing.
func (c *Counter) Name() string { return c.encoding + "+canonical-json+8/message" }

// Count tokenizes canonical model-visible JSON plus eight framing tokens per
// message. Provider templates and hidden reasoning can change actual usage.
func (c *Counter) Count(messages []compact.Message) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for _, m := range messages {
		wire := struct {
			Role   string             `json:"role"`
			Text   string             `json:"content"`
			Calls  []compact.ToolCall `json:"tool_calls,omitempty"`
			CallID string             `json:"tool_call_id,omitempty"`
		}{Role: m.Role, Text: m.Text, Calls: m.ToolCalls, CallID: m.ToolCallID}
		data, err := json.Marshal(wire)
		if err != nil {
			return 0, fmt.Errorf("encode message %q: %w", m.ID, err)
		}
		ids, _, err := c.codec.Encode(string(data))
		if err != nil {
			return 0, fmt.Errorf("tokenize message %q: %w", m.ID, err)
		}
		total += len(ids) + 8
	}
	return total, nil
}
