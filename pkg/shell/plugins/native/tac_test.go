package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestTac(t *testing.T) {
	ctx := context.Background()

	t.Run("reverses lines from stdin", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `printf 'a\nb\nc\n' | tac`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 3 || lines[0] != "c" || lines[1] != "b" || lines[2] != "a" {
			t.Errorf("expected c b a, got %q", buf.String())
		}
	})

	t.Run("reverses lines from file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/lines.txt", []byte("1\n2\n3\n"), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "tac /lines.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 3 || lines[0] != "3" || lines[2] != "1" {
			t.Errorf("expected 3 2 1, got %q", buf.String())
		}
	})

	t.Run("tac of single line is same line", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo hello | tac`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "hello" {
			t.Errorf("expected 'hello', got %q", buf.String())
		}
	})

	t.Run("cat then tac round-trips", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("a\nb\nc\n"), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "tac /f.txt | tac"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 3 || lines[0] != "a" || lines[2] != "c" {
			t.Errorf("expected original order a b c, got %q", buf.String())
		}
	})
}
