package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestTailDefaultLines(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/a.txt", []byte("1\n2\n3\n"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `tail -n 2 /a.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := buf.String(); got != "2\n3\n" {
		t.Fatalf("output = %q, want 2\\n3\\n", got)
	}
}

func TestTailByteMode(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/a.txt", []byte("hello world"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `tail -c 5 /a.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := buf.String(); got != "world" {
		t.Fatalf("output = %q, want world", got)
	}
}

func TestTailInvalidByteCount(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `tail -c bogus`); err == nil {
		t.Fatal("expected error for invalid byte count")
	}
}

func TestTailMissingFile(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `tail /missing.txt`); err == nil {
		t.Fatal("expected error for missing file")
	}
}
