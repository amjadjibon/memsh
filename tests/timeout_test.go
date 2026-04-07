package tests

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/shell"
)

func TestTimeout(t *testing.T) {
	ctx := context.Background()

	newShell := func(t *testing.T, buf *strings.Builder) *shell.Shell {
		t.Helper()
		return NewTestShell(t, buf, shell.WithFS(afero.NewMemMapFs()))
	}

	t.Run("command completes before deadline", func(t *testing.T) {
		var buf strings.Builder
		s := newShell(t, &buf)
		if err := s.Run(ctx, "timeout 5 echo hello"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "hello" {
			t.Errorf("got %q, want hello", buf.String())
		}
	})

	t.Run("command times out returns exit 124", func(t *testing.T) {
		var buf strings.Builder
		s := newShell(t, &buf)
		start := time.Now()
		err := s.Run(ctx, "timeout 0.05 sleep 10")
		elapsed := time.Since(start)
		if elapsed > 2*time.Second {
			t.Errorf("timeout took too long: %v", elapsed)
		}
		if err == nil {
			t.Fatal("expected non-zero exit")
		}
		if exitCode(err) != 124 {
			t.Errorf("expected exit 124, got %v", err)
		}
	})

	t.Run("duration with s suffix", func(t *testing.T) {
		var buf strings.Builder
		s := newShell(t, &buf)
		if err := s.Run(ctx, "timeout 5s echo ok"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "ok" {
			t.Errorf("got %q", buf.String())
		}
	})

	t.Run("duration with m suffix", func(t *testing.T) {
		var buf strings.Builder
		s := newShell(t, &buf)
		if err := s.Run(ctx, "timeout 1m echo ok"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "ok" {
			t.Errorf("got %q", buf.String())
		}
	})

	t.Run("zero timeout means no timeout", func(t *testing.T) {
		var buf strings.Builder
		s := newShell(t, &buf)
		if err := s.Run(ctx, "timeout 0 echo zero"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "zero" {
			t.Errorf("got %q", buf.String())
		}
	})

	t.Run("passes command exit code through", func(t *testing.T) {
		var buf strings.Builder
		s := newShell(t, &buf)
		err := s.Run(ctx, "timeout 5 false")
		if err == nil {
			t.Fatal("expected non-zero exit from false")
		}
		if exitCode(err) == 124 {
			t.Error("should not be 124 (timed out), command failed normally")
		}
	})

	t.Run("wraps pipeline output", func(t *testing.T) {
		var buf strings.Builder
		s := newShell(t, &buf)
		if err := s.Run(ctx, "timeout 5 echo 'hello world' | wc -w"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "2" {
			t.Errorf("got %q, want 2", buf.String())
		}
	})

	t.Run("-s flag accepted (signal ignored)", func(t *testing.T) {
		var buf strings.Builder
		s := newShell(t, &buf)
		if err := s.Run(ctx, "timeout -s TERM 5 echo sig"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "sig" {
			t.Errorf("got %q", buf.String())
		}
	})

	t.Run("missing duration is error 125", func(t *testing.T) {
		var buf strings.Builder
		s := newShell(t, &buf)
		err := s.Run(ctx, "timeout")
		if exitCode(err) != 125 {
			t.Errorf("expected exit 125, got %v", err)
		}
	})

	t.Run("bad duration is error 125", func(t *testing.T) {
		var buf strings.Builder
		s := newShell(t, &buf)
		err := s.Run(ctx, "timeout notaduration echo x")
		if exitCode(err) != 125 {
			t.Errorf("expected exit 125, got %v", err)
		}
	})
}

func TestTput(t *testing.T) {
	ctx := context.Background()

	run := func(t *testing.T, script string) string {
		t.Helper()
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, script); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return buf.String()
	}

	t.Run("cols returns 80", func(t *testing.T) {
		if strings.TrimSpace(run(t, "tput cols")) != "80" {
			t.Errorf("expected 80")
		}
	})

	t.Run("lines returns 24", func(t *testing.T) {
		if strings.TrimSpace(run(t, "tput lines")) != "24" {
			t.Errorf("expected 24")
		}
	})

	t.Run("colors returns 256", func(t *testing.T) {
		if strings.TrimSpace(run(t, "tput colors")) != "256" {
			t.Errorf("expected 256")
		}
	})

	t.Run("sgr0 emits ANSI reset", func(t *testing.T) {
		out := run(t, "tput sgr0")
		if !strings.Contains(out, "\033[0m") {
			t.Errorf("expected ANSI reset, got %q", out)
		}
	})

	t.Run("bold emits ANSI bold", func(t *testing.T) {
		out := run(t, "tput bold")
		if !strings.Contains(out, "\033[1m") {
			t.Errorf("expected ANSI bold, got %q", out)
		}
	})

	t.Run("setaf emits foreground colour", func(t *testing.T) {
		out := run(t, "tput setaf 1")
		if !strings.Contains(out, "\033[38;5;1m") {
			t.Errorf("expected setaf sequence, got %q", out)
		}
	})

	t.Run("clear emits clear-screen", func(t *testing.T) {
		out := run(t, "tput clear")
		if !strings.Contains(out, "\033[H\033[2J") {
			t.Errorf("expected clear sequence, got %q", out)
		}
	})

	t.Run("unknown cap exits 1", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		err := s.Run(ctx, "tput unknowncap")
		if err == nil {
			t.Error("expected non-zero exit for unknown cap")
		}
	})

	t.Run("used in conditional (cols check)", func(t *testing.T) {
		out := run(t, `[ $(tput cols) -ge 80 ] && echo wide || echo narrow`)
		if strings.TrimSpace(out) != "wide" {
			t.Errorf("got %q, want wide", out)
		}
	})
}

func TestStty(t *testing.T) {
	ctx := context.Background()

	run := func(t *testing.T, script string) string {
		t.Helper()
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, script); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return buf.String()
	}

	t.Run("size returns rows cols", func(t *testing.T) {
		out := strings.TrimSpace(run(t, "stty size"))
		if out != "24 80" {
			t.Errorf("got %q, want '24 80'", out)
		}
	})

	t.Run("cols returns 80", func(t *testing.T) {
		if strings.TrimSpace(run(t, "stty cols")) != "80" {
			t.Errorf("expected 80")
		}
	})

	t.Run("rows returns 24", func(t *testing.T) {
		if strings.TrimSpace(run(t, "stty rows")) != "24" {
			t.Errorf("expected 24")
		}
	})

	t.Run("-a dumps settings", func(t *testing.T) {
		out := run(t, "stty -a")
		if !strings.Contains(out, "baud") || !strings.Contains(out, "rows") {
			t.Errorf("expected settings dump, got %q", out)
		}
	})

	t.Run("bare stty prints speed and size", func(t *testing.T) {
		out := run(t, "stty")
		if !strings.Contains(out, "baud") {
			t.Errorf("expected baud in output, got %q", out)
		}
	})

	t.Run("setting no-ops accepted silently", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		for _, cmd := range []string{"stty echo", "stty -echo", "stty raw", "stty sane"} {
			if err := s.Run(ctx, cmd); err != nil {
				t.Errorf("%q: unexpected error: %v", cmd, err)
			}
		}
	})

	t.Run("cols captured in variable", func(t *testing.T) {
		out := run(t, `W=$(stty cols) && echo "width=$W"`)
		if strings.TrimSpace(out) != "width=80" {
			t.Errorf("got %q", out)
		}
	})
}

// exitCode extracts the integer exit status from an interp.ExitStatus error.
// interp.ExitStatus implements error as "exit status N".
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	s := err.Error()
	if after, ok := strings.CutPrefix(s, "exit status "); ok {
		n := 0
		fmt.Sscanf(after, "%d", &n)
		return n
	}
	return -1
}
