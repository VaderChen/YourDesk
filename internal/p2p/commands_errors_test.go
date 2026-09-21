package p2p

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCommandResponseFailureClassification(t *testing.T) {
	for _, tc := range []struct {
		code        string
		interrupted bool
	}{
		{"busy", true}, {"expired", true}, {"in_progress", true},
		{"failed", false}, {"unsupported_method", false}, {"unsupported_version", false},
		{"invalid_request", false}, {"expired_or_invalid", false}, {"invalid_result", false}, {"stale_request", false},
	} {
		err := commandResponseError(CommandResponse{Code: tc.code, Error: "arbitrary localized failure"})
		if err == nil || errors.Is(err, ErrCommandInterrupted) != tc.interrupted {
			t.Fatalf("wrong classification for %s: %v", tc.code, err)
		}
	}
	if err := commandResponseError(CommandResponse{}); err != nil {
		t.Fatal(err)
	}
}

func TestCommandLocalFailureClassification(t *testing.T) {
	p := &Peer{done: make(chan struct{})}
	p.commandsInit()
	p.commands.remote = CommandCapabilities{Version: CommandVersion, Methods: []string{"ping"}}
	if _, err := p.CallCommandParams(context.Background(), "ping", json.RawMessage(`{broken`)); err == nil || errors.Is(err, ErrCommandInterrupted) {
		t.Fatalf("invalid JSON treated as transient: %v", err)
	}
	if _, err := p.CallCommand(context.Background(), "missing"); !errors.Is(err, ErrCommandUnsupported) || errors.Is(err, ErrCommandInterrupted) {
		t.Fatalf("unsupported command treated as transient: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.CallCommand(ctx, "ping"); !errors.Is(err, context.Canceled) || !errors.Is(err, ErrCommandInterrupted) {
		t.Fatalf("cancelled command lost typed classification: %v", err)
	}
	if _, err := p.CallCommand(context.Background(), "ping"); !errors.Is(err, ErrCommandInterrupted) {
		t.Fatalf("disconnected control channel treated as permanent: %v", err)
	}
	for i := uint64(0); i < 32; i++ {
		p.commands.pending[i] = make(chan CommandResponse, 1)
	}
	if _, err := p.CallCommand(context.Background(), "ping"); !errors.Is(err, ErrCommandInterrupted) {
		t.Fatalf("full local queue treated as permanent: %v", err)
	}
}

func TestCommandFailureCodesAcrossLoopback(t *testing.T) {
	a, b := clipboardTestPeers(t, func(Control) {})
	for method, failure := range map[string]error{"xfer.missing": os.ErrNotExist, "xfer.denied": os.ErrPermission, "xfer.expired": context.DeadlineExceeded} {
		if err := b.RegisterCommand(method, func(context.Context) (any, error) { return nil, failure }); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.RegisterCommand("xfer.wait", func(ctx context.Context) (any, error) { <-ctx.Done(); return nil, ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	clipboardEventually(t, func() bool {
		return a.SupportsCommand("xfer.missing") && a.SupportsCommand("xfer.denied") && a.SupportsCommand("xfer.expired") && a.SupportsCommand("xfer.wait")
	})
	for _, tc := range []struct {
		method, code string
		interrupted  bool
	}{
		{"xfer.missing", "failed", false}, {"xfer.denied", "failed", false}, {"xfer.expired", "expired", true},
	} {
		out, err := a.CallCommand(context.Background(), tc.method)
		if err == nil || out.Code != tc.code || errors.Is(err, ErrCommandInterrupted) != tc.interrupted {
			t.Fatalf("wrong remote failure classification: %s %+v %v", tc.method, out, err)
		}
	}
	b.commands.mu.Lock()
	b.commands.active = 4
	b.commands.mu.Unlock()
	out, err := a.CallCommand(context.Background(), "ping")
	b.commands.mu.Lock()
	b.commands.active = 0
	b.commands.mu.Unlock()
	if out.Code != "busy" || !errors.Is(err, ErrCommandInterrupted) {
		t.Fatalf("remote busy was not transient: %+v %v", out, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := a.CallCommand(ctx, "xfer.wait"); !errors.Is(err, ErrCommandInterrupted) {
		t.Fatalf("live transport timeout was not transient: %v", err)
	}
	// Send wire requests directly to distinguish expired requests from malformed
	// deadlines/methods. Neither branch reaches a real filesystem handler.
	for index, tc := range []struct {
		method  string
		expires int64
		code    string
	}{
		{"ping", time.Now().Add(-time.Second).UnixMilli(), "expired"},
		{"ping", time.Now().Add(time.Minute).UnixMilli(), "invalid_request"},
		{strings.Repeat("x", 65), time.Now().Add(time.Second).UnixMilli(), "invalid_request"},
	} {
		id := uint64(1000 + index)
		ch := make(chan CommandResponse, 1)
		a.commands.mu.Lock()
		a.commands.pending[id] = ch
		a.commands.mu.Unlock()
		if err := a.SendControl(Control{Type: "command-request", CommandRequest: &CommandRequest{Version: CommandVersion, ID: id, Method: tc.method, Expires: tc.expires}}); err != nil {
			t.Fatal(err)
		}
		select {
		case response := <-ch:
			if response.Code != tc.code {
				t.Fatalf("wrong admission failure: %+v", response)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("wire rejection response missing")
		}
		a.commands.mu.Lock()
		delete(a.commands.pending, id)
		a.commands.mu.Unlock()
	}
}
