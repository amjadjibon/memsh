package native_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestHostname(t *testing.T) {
	ctx := context.Background()

	t.Run("returns HOSTNAME env var when set", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{"HOSTNAME": "myhost"}),
		)
		if err := s.Run(ctx, "hostname"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "myhost" {
			t.Errorf("expected 'myhost', got %q", buf.String())
		}
	})

	t.Run("falls back to OS hostname when env unset", func(t *testing.T) {
		osHost, err := os.Hostname()
		if err != nil {
			t.Skip("cannot determine OS hostname:", err)
		}
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "hostname"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != osHost {
			t.Errorf("expected %q, got %q", osHost, out)
		}
	})

	t.Run("-s trims domain suffix", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{"HOSTNAME": "web01.example.com"}),
		)
		if err := s.Run(ctx, "hostname -s"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "web01" {
			t.Errorf("expected 'web01', got %q", buf.String())
		}
	})

	t.Run("-s no-op when no dot in hostname", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{"HOSTNAME": "devbox"}),
		)
		if err := s.Run(ctx, "hostname -s"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "devbox" {
			t.Errorf("expected 'devbox', got %q", buf.String())
		}
	})

	t.Run("output usable in pipeline", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{"HOSTNAME": "myhost"}),
		)
		if err := s.Run(ctx, `hostname | tr '[:lower:]' '[:upper:]'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "MYHOST" {
			t.Errorf("expected 'MYHOST', got %q", buf.String())
		}
	})
}
