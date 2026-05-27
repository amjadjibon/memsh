package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestBasename(t *testing.T) {
	ctx := context.Background()

	cases := []struct{ cmd, want string }{
		{"basename /foo/bar", "bar"},
		{"basename /foo/bar.txt", "bar.txt"},
		{"basename /foo/bar.txt .txt", "bar"},
		{"basename bar", "bar"},
		{"basename /", "/"},
		{"basename /foo/", "foo"},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			var buf strings.Builder
			s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
			if err := s.Run(ctx, tc.cmd); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := strings.TrimSpace(buf.String()); got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}

	t.Run("-a multiple names", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "basename -a /foo/a /bar/b"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 || lines[0] != "a" || lines[1] != "b" {
			t.Errorf("expected a\\nb, got %q", buf.String())
		}
	})

	t.Run("-s strips suffix from multiple", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "basename -s .go /pkg/foo.go /pkg/bar.go"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 || lines[0] != "foo" || lines[1] != "bar" {
			t.Errorf("expected foo\\nbar, got %q", buf.String())
		}
	})

	t.Run("missing operand returns error", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "basename"); err == nil {
			t.Error("expected error for missing operand")
		}
	})
}

func TestDirname(t *testing.T) {
	ctx := context.Background()

	cases := []struct{ cmd, want string }{
		{"dirname /foo/bar", "/foo"},
		{"dirname /foo/bar.txt", "/foo"},
		{"dirname bar", "."},
		{"dirname /", "/"},
		{"dirname /foo/", "/foo"},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			var buf strings.Builder
			s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
			if err := s.Run(ctx, tc.cmd); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := strings.TrimSpace(buf.String()); got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}

	t.Run("missing operand returns error", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "dirname"); err == nil {
			t.Error("expected error for missing operand")
		}
	})
}
