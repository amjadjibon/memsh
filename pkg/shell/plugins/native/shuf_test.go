package native_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestShuf(t *testing.T) {
	ctx := context.Background()

	t.Run("shuffles lines from stdin", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/nums.txt", []byte("1\n2\n3\n4\n5\n"), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "shuf /nums.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 5 {
			t.Errorf("expected 5 lines, got %d", len(lines))
		}
		// Sort and check all values present.
		sort.Strings(lines)
		for i, want := range []string{"1", "2", "3", "4", "5"} {
			if lines[i] != want {
				t.Errorf("expected %q at sorted[%d], got %q", want, i, lines[i])
			}
		}
	})

	t.Run("-n limits output count", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/nums.txt", []byte("1\n2\n3\n4\n5\n"), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "shuf -n 3 /nums.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 3 {
			t.Errorf("expected 3 lines, got %d", len(lines))
		}
	})

	t.Run("-e shuffles echo args", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "shuf -e a b c d"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 4 {
			t.Errorf("expected 4 lines, got %d: %q", len(lines), buf.String())
		}
	})

	t.Run("-i generates range", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "shuf -i 1-5"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 5 {
			t.Errorf("expected 5 lines from range 1-5, got %d", len(lines))
		}
		sort.Strings(lines)
		for i, want := range []string{"1", "2", "3", "4", "5"} {
			if lines[i] != want {
				t.Errorf("expected %q, got %q", want, lines[i])
			}
		}
	})

	t.Run("-i -n picks subset of range", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "shuf -i 1-100 -n 5"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 5 {
			t.Errorf("expected 5 lines, got %d", len(lines))
		}
	})
}
