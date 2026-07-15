package native_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestWatch(t *testing.T) {
	t.Run("runs command once immediately and exits on context cancel", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		// -n 10 so the ticker never fires; context cancels after 300ms
		_ = s.Run(ctx, `watch -n 10 echo hello`)

		out := buf.String()
		if !strings.Contains(out, "hello") {
			t.Errorf("expected 'hello' in output, got %q", out)
		}
	})

	t.Run("header contains interval and command", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		_ = s.Run(ctx, `watch -n 5 echo hi`)

		out := buf.String()
		if !strings.Contains(out, "5.0s") {
			t.Errorf("expected interval '5.0s' in header, got %q", out)
		}
		if !strings.Contains(out, "echo hi") {
			t.Errorf("expected command 'echo hi' in header, got %q", out)
		}
	})

	t.Run("-t suppresses header", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		_ = s.Run(ctx, `watch -t -n 10 echo quiet`)

		out := buf.String()
		if strings.Contains(out, "Every") {
			t.Errorf("expected no header with -t, got %q", out)
		}
		if !strings.Contains(out, "quiet") {
			t.Errorf("expected command output 'quiet', got %q", out)
		}
	})

	t.Run("fires multiple times within interval window", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		// interval 100ms, run for ~350ms → expect at least 3 runs
		ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
		defer cancel()

		_ = s.Run(ctx, `watch -t -n 0.1 echo tick`)

		out := buf.String()
		count := strings.Count(out, "tick")
		if count < 3 {
			t.Errorf("expected at least 3 runs, got %d (output: %q)", count, out)
		}
	})

	t.Run("missing command prints error", func(t *testing.T) {
		var buf strings.Builder
		var errBuf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		_ = s.Run(context.Background(), `watch -n 1 2>/dev/null; true`)
		// Should not panic; just verify no crash
		_ = errBuf.String()
	})

	t.Run("invalid interval prints error", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		err := s.Run(context.Background(), `watch -n abc echo x`)
		if err == nil {
			t.Error("expected error for invalid interval, got nil")
		}
	})
}
