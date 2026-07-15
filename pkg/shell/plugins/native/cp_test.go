package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestCpFile(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/src.txt", []byte("hello"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `cp /src.txt /dst.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := afero.ReadFile(fs, "/dst.txt")
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("dst content = %q, want hello", got)
	}
}

func TestCpDirRecursive(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/src", 0o755)
	_ = afero.WriteFile(fs, "/src/a.txt", []byte("data"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `cp -r /src /dst`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := fs.Stat("/dst/a.txt"); err != nil {
		t.Fatalf("expected copied file: %v", err)
	}
}

func TestCpDirWithoutRecursiveFails(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/src", 0o755)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `cp /src /dst`); err == nil {
		t.Fatal("expected error copying directory without -r")
	}
}

func TestCpMissingSource(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `cp /missing.txt /dst.txt`); err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestCpMissingDestinationOperand(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/src.txt", []byte("hello"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `cp /src.txt`); err == nil {
		t.Fatal("expected error for missing destination")
	}
}
