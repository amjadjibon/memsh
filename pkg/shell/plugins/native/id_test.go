package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestId(t *testing.T) {
	ctx := context.Background()

	t.Run("default output contains uid gid groups", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{"USER": "alice", "UID": "1001", "GID": "1001"}),
		)
		if err := s.Run(ctx, "id"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, "uid=1001(alice)") {
			t.Errorf("expected uid=1001(alice) in %q", out)
		}
		if !strings.Contains(out, "gid=") {
			t.Errorf("expected gid= in %q", out)
		}
		if !strings.Contains(out, "groups=") {
			t.Errorf("expected groups= in %q", out)
		}
	})

	t.Run("-u prints only uid", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{"UID": "42"}),
		)
		if err := s.Run(ctx, "id -u"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "42" {
			t.Errorf("expected '42', got %q", buf.String())
		}
	})

	t.Run("-u -n prints username", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{"USER": "bob", "UID": "99"}),
		)
		if err := s.Run(ctx, "id -u -n"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "bob" {
			t.Errorf("expected 'bob', got %q", buf.String())
		}
	})

	t.Run("-g prints only gid", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{"GID": "7"}),
		)
		if err := s.Run(ctx, "id -g"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "7" {
			t.Errorf("expected '7', got %q", buf.String())
		}
	})

	t.Run("falls back to OS user when env unset", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "id"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "uid=") {
			t.Errorf("expected uid= in output, got %q", out)
		}
	})
}
