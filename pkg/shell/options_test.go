package shell_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/pkg/network"
	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestWithBuiltinRegistersRawFunction(t *testing.T) {
	var buf bytes.Buffer
	called := false
	fn := func(ctx context.Context, args []string) error {
		called = true
		hc := interp.HandlerCtx(ctx)
		hc.Stdout.Write([]byte("custom builtin ran\n"))
		return nil
	}

	s := newTestShell(t, &buf, shell.WithBuiltin("mybuiltin", fn))
	defer s.Close()

	if err := s.Run(context.Background(), "mybuiltin"); err != nil {
		t.Fatalf("mybuiltin: %v", err)
	}
	if !called {
		t.Error("registered builtin function was not called")
	}
	if got := buf.String(); got != "custom builtin ran\n" {
		t.Errorf("output = %q", got)
	}
}

func TestWithAllowExternalCommandsDefaultBlocked(t *testing.T) {
	var buf bytes.Buffer
	s := newTestShell(t, &buf)
	defer s.Close()

	err := s.Run(context.Background(), "nonexistent-cmd-abc123")
	if err == nil {
		t.Fatal("expected error for unregistered command")
	}
	if !strings.Contains(err.Error(), "command not found") {
		t.Errorf("error = %v, want 'command not found'", err)
	}
}

func TestWithAllowExternalCommandsEnabled(t *testing.T) {
	var buf bytes.Buffer
	s := newTestShell(t, &buf, shell.WithAllowExternalCommands(true))
	defer s.Close()

	err := s.Run(context.Background(), "nonexistent-cmd-abc123")
	if err == nil {
		t.Fatal("expected error: the binary genuinely does not exist on the OS")
	}
	// With external commands allowed, the shell falls through to a real OS
	// exec attempt instead of memsh's own "command not found" message.
	if strings.Contains(err.Error(), "command not found") {
		t.Errorf("error = %v, want an OS-exec error, not memsh's builtin 'command not found'", err)
	}
}

func TestWithNetworkLimitsBlocksDialBeforeNetworkAccess(t *testing.T) {
	var buf bytes.Buffer
	s := newTestShell(t, &buf,
		shell.WithNetworkPolicy(network.Policy{Mode: network.ModeFull}),
		shell.WithNetworkLimits(network.Limits{MaxRequests: 1}),
		shell.WithNetworkUsage(network.Usage{Requests: 1}),
	)
	defer s.Close()

	err := s.Run(context.Background(), "curl http://127.0.0.1:1/should-not-be-reached")
	if err == nil {
		t.Fatal("expected curl to fail due to the pre-exhausted request limit")
	}
	if !strings.Contains(buf.String(), "limit exceeded") {
		t.Errorf("curl output = %q, want it to mention the network request limit", buf.String())
	}
}
