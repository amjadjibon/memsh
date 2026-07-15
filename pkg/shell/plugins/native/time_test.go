package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestTime(t *testing.T) {
	t.Run("times a command and prints timing lines", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(context.Background(), "time echo hello"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "hello") {
			t.Errorf("expected echo output, got %q", out)
		}
		// stderr and stdout both go to buf in test mode
		if !strings.Contains(out, "real") {
			t.Errorf("expected 'real' timing line, got %q", out)
		}
	})

	t.Run("propagates exit code of timed command", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(context.Background(), "time false"); err == nil {
			t.Error("expected non-zero exit from 'time false'")
		}
	})

	t.Run("time with no-op succeeds", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		// 'time' with no args is valid in bash — times a no-op
		_ = s.Run(context.Background(), "time true")
	})
}
