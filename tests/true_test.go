package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestTrue(t *testing.T) {
	ctx := context.Background()

	t.Run("true exits 0", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "true"); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("true produces no output", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		_ = s.Run(ctx, "true")
		if buf.String() != "" {
			t.Errorf("expected no output, got %q", buf.String())
		}
	})

	t.Run("true usable in while loop guard", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		// Runs exactly once because we break immediately.
		if err := s.Run(ctx, `i=0; while true; do i=$((i+1)); break; done; echo $i`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "1" {
			t.Errorf("expected '1', got %q", buf.String())
		}
	})
}

func TestFalse(t *testing.T) {
	ctx := context.Background()

	t.Run("false exits non-zero", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "false"); err == nil {
			t.Error("expected non-zero exit, got nil")
		}
	})

	t.Run("false produces no output", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		_ = s.Run(ctx, "false")
		if buf.String() != "" {
			t.Errorf("expected no output, got %q", buf.String())
		}
	})

	t.Run("false triggers else branch", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `if false; then echo yes; else echo no; fi`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "no" {
			t.Errorf("expected 'no', got %q", buf.String())
		}
	})
}
