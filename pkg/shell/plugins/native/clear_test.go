package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestClearWritesAnsiResetSequence(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `clear`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := buf.String(); got != "\x1b[2J\x1b[H" {
		t.Fatalf("output = %q, want ANSI clear sequence", got)
	}
}

func TestResetDelegatesToClear(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `reset`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := buf.String(); got != "\x1b[2J\x1b[H" {
		t.Fatalf("output = %q, want ANSI clear sequence", got)
	}
}
