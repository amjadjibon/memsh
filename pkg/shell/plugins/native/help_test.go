package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

// "help" is a recognized builtin name in mvdan.cc/sh's runner, so it is
// intercepted before reaching our exec handler and always fails with
// "unsupported builtin" — the plugin can only be reached through "man",
// which forwards to it directly (see man.go's runMan).

func TestManListsCommands(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `man`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Available commands:") {
		t.Fatalf("output = %q, want command listing", buf.String())
	}
}

func TestManForKnownCommand(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `man cat`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "cat") {
		t.Fatalf("output = %q, want description for cat", buf.String())
	}
}

func TestManForUnknownCommand(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `man nosuchcmd`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "no help entry") {
		t.Fatalf("output = %q, want no help entry message", buf.String())
	}
}
