package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestRev(t *testing.T) {
	ctx := context.Background()

	t.Run("reverses chars in a line", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo hello | rev`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "olleh" {
			t.Errorf("expected 'olleh', got %q", buf.String())
		}
	})

	t.Run("reverses multiple lines", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `printf 'abc\n123\n' | rev`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 || lines[0] != "cba" || lines[1] != "321" {
			t.Errorf("expected cba/321, got %q", buf.String())
		}
	})

	t.Run("reverses from file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/words.txt", []byte("dog\ncat\n"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "rev /words.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 || lines[0] != "god" || lines[1] != "tac" {
			t.Errorf("expected god/tac, got %q", buf.String())
		}
	})

	t.Run("double rev is identity", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo hello | rev | rev`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "hello" {
			t.Errorf("expected 'hello', got %q", buf.String())
		}
	})

	t.Run("empty line stays empty", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo "" | rev`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "" {
			t.Errorf("expected empty, got %q", buf.String())
		}
	})
}
