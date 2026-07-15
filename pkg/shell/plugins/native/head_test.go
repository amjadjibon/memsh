package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestHeadDefaultLines(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/a.txt", []byte("1\n2\n3\n"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `head -n 2 /a.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := buf.String(); got != "1\n2\n" {
		t.Fatalf("output = %q, want 1\\n2\\n", got)
	}
}

func TestHeadByteMode(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/a.txt", []byte("hello world"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `head -c 5 /a.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := buf.String(); got != "hello" {
		t.Fatalf("output = %q, want hello", got)
	}
}

func TestHeadFromStdin(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `printf 'a\nb\nc\n' | head -n1`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "a" {
		t.Fatalf("output = %q, want a", got)
	}
}

func TestHeadInvalidLineCount(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `head -n bogus`); err == nil {
		t.Fatal("expected error for invalid line count")
	}
}

func TestHeadMissingFile(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `head /missing.txt`); err == nil {
		t.Fatal("expected error for missing file")
	}
}
