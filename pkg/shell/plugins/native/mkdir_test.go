package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestMkdirCreatesDirectory(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `mkdir -p /a/b/c`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := fs.Stat("/a/b/c")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}
}

func TestMkdirVerbose(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `mkdir -v /a`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "created directory") {
		t.Fatalf("output = %q, want verbose creation message", buf.String())
	}
}

func TestMkdirWithMode(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `mkdir -m 0700 /a`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := fs.Stat("/a"); err != nil {
		t.Fatalf("stat: %v", err)
	}
}

func TestMkdirMissingOperand(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `mkdir`); err == nil {
		t.Fatal("expected error for missing operand")
	}
}
