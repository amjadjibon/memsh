package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestPaste(t *testing.T) {
	ctx := context.Background()

	t.Run("merges two files with tab delimiter", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/a.txt", []byte("1\n2\n3\n"), 0o644)
		afero.WriteFile(fs, "/b.txt", []byte("a\nb\nc\n"), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "paste /a.txt /b.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 3 || lines[0] != "1\ta" || lines[1] != "2\tb" {
			t.Errorf("unexpected output: %q", buf.String())
		}
	})

	t.Run("-d changes delimiter", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/a.txt", []byte("x\ny\n"), 0o644)
		afero.WriteFile(fs, "/b.txt", []byte("1\n2\n"), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "paste -d , /a.txt /b.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 || lines[0] != "x,1" {
			t.Errorf("expected 'x,1', got %q", buf.String())
		}
	})

	t.Run("-s serial mode", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/c.txt", []byte("a\nb\nc\n"), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "paste -s /c.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "a\tb\tc" {
			t.Errorf("expected 'a\\tb\\tc', got %q", out)
		}
	})
}
