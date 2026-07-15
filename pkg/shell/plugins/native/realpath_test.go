package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestRealpath(t *testing.T) {
	ctx := context.Background()

	t.Run("absolute path returned as-is", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "realpath /foo/bar"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "/foo/bar" {
			t.Errorf("expected '/foo/bar', got %q", buf.String())
		}
	})

	t.Run("multiple paths, one per line", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "realpath /a /b /c"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 3 {
			t.Errorf("expected 3 lines, got %d: %q", len(lines), buf.String())
		}
	})

	t.Run("missing operand returns error", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "realpath"); err == nil {
			t.Error("expected error for missing operand")
		}
	})
}
