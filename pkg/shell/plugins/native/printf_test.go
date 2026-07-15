package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

// "printf" is a recognized builtin name in mvdan.cc/sh's runner, so a literal
// leading "printf" is always handled by its own implementation, never
// reaching PrintfPlugin.Run. Reach it dynamically via "xargs" instead.

func TestPrintfPluginViaXargs(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `echo world | xargs printf "hello %s\n"`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "hello world" {
		t.Fatalf("output = %q, want hello world", got)
	}
}

func TestPrintfPluginViaXargsNoArgs(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `echo x | xargs printf`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
