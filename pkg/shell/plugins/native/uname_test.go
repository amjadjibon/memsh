package native_test

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestUname(t *testing.T) {
	ctx := context.Background()

	t.Run("default prints kernel name", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "uname"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out == "" {
			t.Error("expected non-empty kernel name")
		}
		// Should match runtime GOOS-derived value.
		switch runtime.GOOS {
		case "darwin":
			if out != "Darwin" {
				t.Errorf("expected 'Darwin', got %q", out)
			}
		case "linux":
			if out != "Linux" {
				t.Errorf("expected 'Linux', got %q", out)
			}
		}
	})

	t.Run("-s prints kernel name", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{"UNAME_KERNEL": "TestOS"}),
		)
		if err := s.Run(ctx, "uname -s"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "TestOS" {
			t.Errorf("expected 'TestOS', got %q", buf.String())
		}
	})

	t.Run("-m prints machine arch", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{"UNAME_MACHINE": "x86_64"}),
		)
		if err := s.Run(ctx, "uname -m"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "x86_64" {
			t.Errorf("expected 'x86_64', got %q", buf.String())
		}
	})

	t.Run("-a prints all fields", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{
				"UNAME_KERNEL":  "Linux",
				"UNAME_NODE":    "myhost",
				"UNAME_RELEASE": "5.15.0",
				"UNAME_MACHINE": "x86_64",
			}),
		)
		if err := s.Run(ctx, "uname -a"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		for _, want := range []string{"Linux", "myhost", "5.15.0", "x86_64"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected %q in uname -a output, got %q", want, out)
			}
		}
	})

	t.Run("combined flags -sm", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithEnv(map[string]string{
				"UNAME_KERNEL":  "Linux",
				"UNAME_MACHINE": "arm64",
			}),
		)
		if err := s.Run(ctx, "uname -sm"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "Linux arm64" {
			t.Errorf("expected 'Linux arm64', got %q", out)
		}
	})
}
