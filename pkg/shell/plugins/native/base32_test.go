package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestBase32(t *testing.T) {
	ctx := context.Background()

	// base32("hello") = "NBSWY3DP"  (5 bytes → 8 base32 chars, no padding)
	// base32("hello world") = "NBSWY3DPEB3W64TMMQ======"

	t.Run("encodes a string arg", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "base32 hello"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "NBSWY3DP" {
			t.Errorf("expected 'NBSWY3DP', got %q", out)
		}
	})

	t.Run("decodes with -d", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "base32 -d NBSWY3DP"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if buf.String() != "hello" {
			t.Errorf("expected 'hello', got %q", buf.String())
		}
	})

	t.Run("encodes stdin", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo -n hello | base32`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "NBSWY3DP" {
			t.Errorf("expected 'NBSWY3DP', got %q", out)
		}
	})

	t.Run("round-trip encode then decode", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `base32 "hello world" | base32 -d`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if buf.String() != "hello world" {
			t.Errorf("expected 'hello world', got %q", buf.String())
		}
	})

	t.Run("invalid base32 returns error", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		err := s.Run(ctx, "base32 -d not-valid-base32!!!")
		if err == nil {
			t.Error("expected error for invalid base32 input")
		}
	})
}
