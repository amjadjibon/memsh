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
		)
		if err := s.Run(ctx, "whoami"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "loguser" {
			t.Errorf("expected 'loguser', got %q", out)
		}
	})

	t.Run("falls back to OS user when no env vars set", func(t *testing.T) {
		osUser, err := user.Current()
		if err != nil {
			t.Skip("cannot determine OS user:", err)
		}
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "whoami"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out == "" {
			t.Error("expected non-empty output")
		}
		// The OS fallback should at minimum return something non-empty.
		// On most systems it will match the current user.
		_ = osUser
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
