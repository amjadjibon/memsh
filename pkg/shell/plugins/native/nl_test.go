package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestNl(t *testing.T) {
	ctx := context.Background()

	t.Run("numbers non-empty lines by default", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `printf 'a\nb\nc\n' | nl`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "1") || !strings.Contains(out, "a") {
			t.Errorf("expected numbered lines, got %q", out)
		}
	})

	t.Run("-b a numbers all lines including blank", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `printf 'a\n\nb\n' | nl -b a`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "2") {
			t.Errorf("expected blank line to be numbered, got %q", out)
		}
	})

	t.Run("-v sets start number", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `printf 'x\ny\n' | nl -v 10`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "10") {
			t.Errorf("expected line starting at 10, got %q", out)
		}
	})

	t.Run("numbers file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("foo\nbar\n"), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "nl /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Errorf("expected 2 numbered lines, got %d: %q", len(lines), buf.String())
		}
	})
}
