package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestWc(t *testing.T) {
	ctx := context.Background()

	t.Run("wc -l counts lines from stdin", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo -e "a\nb\nc" | wc -l`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "3" {
			t.Errorf("expected '3', got %q", out)
		}
	})

	t.Run("wc -l counts lines from virtual FS file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/lines.txt", []byte("a\nb\nc\n"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "wc -l /lines.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, "3") {
			t.Errorf("expected '3' in output, got %q", out)
		}
	})

	t.Run("wc -w counts words", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo "hello world test" | wc -w`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "3" {
			t.Errorf("expected '3', got %q", out)
		}
	})

	t.Run("wc -c counts bytes", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo "test" | wc -c`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "5" {
			t.Errorf("expected '5' (4 chars + newline), got %q", out)
		}
	})

	t.Run("wc handles multiple files", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/a.txt", []byte("line1\nline2\n"), 0o644)
		afero.WriteFile(fs, "/b.txt", []byte("single"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "wc -l /a.txt /b.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "2") || !strings.Contains(out, "1") {
			t.Errorf("expected file counts in output: %q", out)
		}
	})
}
