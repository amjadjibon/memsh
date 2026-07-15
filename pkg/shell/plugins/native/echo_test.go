package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

// A literal leading "echo" is a recognized builtin name in mvdan.cc/sh's
// runner and is always handled by its own case "echo" implementation, never
// reaching EchoPlugin.Run. It only reaches our plugin when invoked
// dynamically, e.g. as the target command of "xargs".

func TestEchoPluginViaXargsCombinedFlags(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `echo hi | xargs echo -ne`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := buf.String(); got != "hi" {
		t.Fatalf("output = %q, want hi (no newline, no literal -ne)", got)
	}
}

func TestEchoPluginViaXargsPlainArgs(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `echo one two | xargs echo`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "one two" {
		t.Fatalf("output = %q, want one two", got)
	}
}
