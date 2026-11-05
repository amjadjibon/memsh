package tests

import (
	"context"
	"os/user"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestWhoami(t *testing.T) {
	ctx := context.Background()

	t.Run("returns USER env variable when set", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{"USER": "testuser"}),
		)
		if err := s.Run(ctx, "whoami"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "testuser" {
			t.Errorf("expected 'testuser', got %q", out)
		}
	})

	t.Run("prefers USER over LOGNAME", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{
				"USER":    "primary",
				"LOGNAME": "secondary",
			}),
		)
		if err := s.Run(ctx, "whoami"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "primary" {
			t.Errorf("expected 'primary', got %q", out)
		}
	})

	t.Run("falls back to LOGNAME when USER is unset", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{"LOGNAME": "loguser"}),
			shell.WithInheritEnv(false), // keep the host USER out
		)
		if err := s.Run(ctx, "whoami"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "loguser" {
			t.Errorf("expected 'loguser', got %q", out)
		}
	})

	t.Run("uses a virtual identity, never the host user, when env unset", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithInheritEnv(false))
		if err := s.Run(ctx, "whoami; id"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.HasPrefix(out, "memsh\n") || !strings.Contains(out, "uid=1000(memsh)") {
			t.Errorf("output = %q, want virtual identity memsh/1000", out)
		}
		if u, err := user.Current(); err == nil && u.Username != "memsh" && strings.Contains(out, u.Username) {
			t.Errorf("output leaked host user %q: %q", u.Username, out)
		}
	})

	t.Run("output usable in a pipeline", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{"USER": "pipeuser"}),
		)
		if err := s.Run(ctx, `whoami | tr '[:lower:]' '[:upper:]'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "PIPEUSER" {
			t.Errorf("expected 'PIPEUSER', got %q", out)
		}
	})
}
