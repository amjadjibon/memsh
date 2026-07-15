package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestMvRenamesFile(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/a.txt", []byte("data"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `mv /a.txt /b.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := fs.Stat("/a.txt"); err == nil {
		t.Fatal("expected source to be gone")
	}
	if _, err := fs.Stat("/b.txt"); err != nil {
		t.Fatalf("expected destination to exist: %v", err)
	}
}

func TestMvIntoDirectory(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/a.txt", []byte("data"), 0o644)
	_ = fs.MkdirAll("/dst", 0o755)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `mv /a.txt /dst`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := fs.Stat("/dst/a.txt"); err != nil {
		t.Fatalf("expected file moved into dir: %v", err)
	}
}

func TestMvMissingDestination(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `mv /a.txt`); err == nil {
		t.Fatal("expected error for missing destination")
	}
}

func TestMvMissingSource(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `mv /missing.txt /b.txt`); err == nil {
		t.Fatal("expected error for missing source")
	}
}
