// The compactd command serves HTTP, compacts JSON files, and recalls messages.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/muratmirgun/compact-engine/archive"
	"github.com/muratmirgun/compact-engine/compact"
	"github.com/muratmirgun/compact-engine/jev"
	"github.com/muratmirgun/compact-engine/server"
	"github.com/muratmirgun/compact-engine/token"
)

type streams struct {
	in       io.Reader
	out, err io.Writer
}
type options struct{ archive, encoding, scores, model, endpoint string }

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], streams{in: os.Stdin, out: os.Stdout, err: os.Stderr})
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdio streams) error {
	if len(args) == 0 {
		args = []string{"serve"}
	}
	switch args[0] {
	case "serve":
		return serve(ctx, args[1:], stdio)
	case "compact":
		return compactFile(ctx, args[1:], stdio)
	case "recall":
		return recall(ctx, args[1:], stdio)
	case "help", "-h", "--help":
		_, err := fmt.Fprintln(stdio.out, "compactd serve|compact|recall [flags]\nUse compactd <command> -h for options.")
		return err
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func commonFlags(fs *flag.FlagSet) *options {
	opts := &options{}
	fs.StringVar(&opts.archive, "archive", ".compact-data", "private snapshot directory")
	fs.StringVar(&opts.encoding, "encoding", "o200k_base", "o200k_base or cl100k_base")
	fs.StringVar(&opts.scores, "scores", "", "explicit score JSON file for offline replay; never contacts Jev")
	fs.StringVar(&opts.model, "model", "jev-latest", "Jev model ID")
	fs.StringVar(&opts.endpoint, "jev-endpoint", "https://api.typesafe.ai/v1/systemone", "Jev endpoint; HTTP only on loopback")
	return opts
}

func build(opts *options) (*compact.Engine, *archive.Store, func(), error) {
	store, err := archive.New(opts.archive)
	if err != nil {
		return nil, nil, nil, err
	}
	counter, err := token.New(opts.encoding)
	if err != nil {
		return nil, nil, nil, err
	}
	var scorer compact.Scorer
	cleanup := func() {}
	if opts.scores != "" {
		var scores map[string]compact.Score
		if err := decodeFile(opts.scores, &scores); err != nil {
			return nil, nil, nil, err
		}
		scorer = compact.NewReplay(scores)
	} else {
		client, err := jev.New(jev.Config{APIKey: os.Getenv("TYPESAFE_API_KEY"), Model: opts.model, Endpoint: opts.endpoint})
		if err != nil {
			return nil, nil, nil, err
		}
		scorer = client
		cleanup = client.Close
	}
	engine, err := compact.New(scorer, counter, store)
	if err != nil {
		cleanup()
		return nil, nil, nil, err
	}
	return engine, store, cleanup, nil
}

func serve(ctx context.Context, args []string, stdio streams) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stdio.err)
	opts := commonFlags(fs)
	listen := fs.String("listen", "127.0.0.1:8787", "listen address; non-loopback requires COMPACT_API_TOKEN")
	concurrent := fs.Int("concurrency", 4, "maximum active HTTP requests")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("serve accepts flags only")
	}
	auth := os.Getenv("COMPACT_API_TOKEN")
	if err := validateListen(*listen, auth); err != nil {
		return err
	}
	engine, store, cleanup, err := build(opts)
	if err != nil {
		return err
	}
	defer cleanup()
	handler, err := server.New(engine, store, server.Config{Token: auth, MaxConcurrent: *concurrent})
	if err != nil {
		return err
	}
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", *listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	logger := slog.New(slog.NewJSONHandler(stdio.err, nil))
	mode := "jev"
	if opts.scores != "" {
		mode = "replay"
	}
	logger.Info("compaction service listening", "address", listener.Addr().String(), "scorer", mode)
	// The server goroutine exits on Serve failure or Shutdown/Close below.
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		shutdownErr := srv.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			_ = srv.Close()
		} // Force-close after the graceful deadline.
		serveErr := <-done
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		return errors.Join(shutdownErr, serveErr)
	}
}

func compactFile(ctx context.Context, args []string, stdio streams) error {
	fs := flag.NewFlagSet("compact", flag.ContinueOnError)
	fs.SetOutput(stdio.err)
	opts := commonFlags(fs)
	input := fs.String("input", "-", "request JSON path, or - for stdin")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("compact accepts flags only")
	}
	var req compact.Request
	if *input == "-" {
		if err := decode(stdio.in, &req); err != nil {
			return err
		}
	} else if err := decodeFile(*input, &req); err != nil {
		return err
	}
	engine, _, cleanup, err := build(opts)
	if err != nil {
		return err
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := engine.Compact(ctx, req)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdio.out).Encode(result)
}

func recall(ctx context.Context, args []string, stdio streams) error {
	fs := flag.NewFlagSet("recall", flag.ContinueOnError)
	fs.SetOutput(stdio.err)
	dir := fs.String("archive", ".compact-data", "snapshot directory")
	id := fs.String("snapshot", "", "snapshot ID")
	message := fs.String("message", "", "optional original message ID")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 || *id == "" {
		return errors.New("recall requires -snapshot and accepts flags only")
	}
	store, err := archive.New(*dir)
	if err != nil {
		return err
	}
	if *message != "" {
		m, err := store.Recall(ctx, *id, *message)
		if err != nil {
			return err
		}
		return json.NewEncoder(stdio.out).Encode(m)
	}
	req, err := store.Load(ctx, *id)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdio.out).Encode(req)
}

func decodeFile(path string, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return decode(f, value)
}

func decode(r io.Reader, value any) error {
	data, err := io.ReadAll(io.LimitReader(r, (8<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 8<<20 {
		return errors.New("json exceeds 8 mib limit")
	}
	if !utf8.Valid(data) {
		return errors.New("json must use utf-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("expected one json object")
	}
	return nil
}

func validateListen(address, tokenValue string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("listen address: %w", err)
	}
	ip := net.ParseIP(host)
	if tokenValue == "" && (ip == nil || !ip.IsLoopback()) {
		return errors.New("non-loopback listen requires COMPACT_API_TOKEN")
	}
	return nil
}
