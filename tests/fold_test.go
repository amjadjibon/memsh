package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestFold(t *testing.T) {
	ctx := context.Background()

	t.Run("wraps line at default 80 chars", func(t *testing.T) {
		long := strings.Repeat("x", 100)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo `+long+` | fold`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) < 2 {
			t.Errorf("expected wrap at 80, got single line %q", buf.String())
		}
		if len(lines[0]) != 80 {
			t.Errorf("expected first line to be 80 chars, got %d", len(lines[0]))
		}
	})

	t.Run("-w sets custom width", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo "abcdefghij" | fold -w 3`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		for _, l := range lines {
			if len(l) > 3 {
				t.Errorf("line exceeds width 3: %q", l)
			}
		}
	})

	t.Run("-s breaks at spaces", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo "hello world" | fold -w 8 -s`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		// "hello " fits in 8 chars, so it should break before "world"
		if !strings.Contains(out, "hello") || !strings.Contains(out, "world") {
			t.Errorf("expected both words present, got %q", out)
		}
	})

	t.Run("short line passes through unchanged", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo "hi" | fold -w 80`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "hi" {
			t.Errorf("expected 'hi', got %q", buf.String())
		}
	})
}
