package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/goleak"

	"github.com/muratmirgun/compact-engine/compact"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestCLIReplayAndRecall(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	scores := filepath.Join(dir, "scores.json")
	if err := os.WriteFile(scores, []byte(`{"call":{"relevance":0.05,"detail":0.05}}`), 0600); err != nil {
		t.Fatal(err)
	}
	zero := 0
	req := compact.Request{Goal: "fix cache", TargetTokens: 200, RecentMessages: &zero, Messages: []compact.Message{
		{ID: "u", Role: "user", Text: "fix cache"},
		{ID: "call", Role: "assistant", ToolCalls: []compact.ToolCall{{ID: "c", Name: "read", Arguments: json.RawMessage(`{}`)}}},
		{ID: "result", Role: "tool", ToolCallID: "c", Text: strings.Repeat("old completed output\n", 1000)},
	}}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	args := []string{"compact", "-scores", scores, "-archive", filepath.Join(dir, "archive")}
	if err := run(t.Context(), args, streams{in: bytes.NewReader(data), out: &out, err: io.Discard}); err != nil {
		t.Fatal(err)
	}
	var result compact.Result
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Applied || !result.BudgetMet || result.Stats.Scorer != "replay" {
		t.Fatalf("run(compact) result=%+v", result)
	}
	out.Reset()
	if err := run(t.Context(), []string{"recall", "-archive", filepath.Join(dir, "archive"), "-snapshot", result.SnapshotID, "-message", "result"}, streams{out: &out, err: io.Discard}); err != nil {
		t.Fatal(err)
	}
	var restored compact.Message
	if err := json.Unmarshal(out.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Text != req.Messages[2].Text {
		t.Error("run(recall) did not restore original")
	}
}

func TestListenRequiresTokenOutsideLoopback(t *testing.T) {
	t.Parallel()
	for _, address := range []string{"0.0.0.0:8787", ":8787", "example.com:8787"} {
		if err := validateListen(address, ""); err == nil {
			t.Errorf("validateListen(%q) accepted anonymous public bind", address)
		}
	}
	for _, address := range []string{"127.0.0.1:8787", "[::1]:8787"} {
		if err := validateListen(address, ""); err != nil {
			t.Error(err)
		}
	}
}

func TestServeStopsOnCancellation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	scores := filepath.Join(dir, "scores.json")
	if err := os.WriteFile(scores, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Startup logging occurs after the listening socket is ready.
	err := run(ctx, []string{"serve", "-listen", "127.0.0.1:0", "-scores", scores, "-archive", filepath.Join(dir, "archive")}, streams{out: io.Discard, err: cancelWriter{cancel: cancel}})
	if err != nil {
		t.Fatalf("run(serve) shutdown=%v", err)
	}
}

type cancelWriter struct{ cancel context.CancelFunc }

func (w cancelWriter) Write(p []byte) (int, error) { w.cancel(); return len(p), nil }
