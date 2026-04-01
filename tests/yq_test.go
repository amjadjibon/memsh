package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/shell"
)

func TestYq(t *testing.T) {
	ctx := context.Background()

	t.Run("yq field selection from stdin", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo 'name: alice' | yq .name`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "alice" {
			t.Errorf("expected 'alice', got %q", out)
		}
	})

	t.Run("yq identity outputs YAML", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `printf 'name: alice\nage: 30\n' | yq .`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "alice") || !strings.Contains(out, "30") {
			t.Errorf("unexpected output: %q", out)
		}
	})

	t.Run("yq -j outputs JSON", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo 'name: alice' | yq -j .`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, `"name"`) || !strings.Contains(out, `"alice"`) {
			t.Errorf("expected JSON output, got %q", out)
		}
	})

	t.Run("yq -jc compact JSON output", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo 'name: alice' | yq -jc .`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if strings.Contains(out, "\n  ") {
			t.Errorf("expected compact JSON, got: %q", out)
		}
		if !strings.Contains(out, `"alice"`) {
			t.Errorf("unexpected compact output: %q", out)
		}
	})

	t.Run("yq reads from virtual FS file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/config.yaml", []byte("host: localhost\nport: 8080\n"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, `yq .host /config.yaml`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "localhost" {
			t.Errorf("expected 'localhost', got %q", out)
		}
	})

	t.Run("yq processes JSON input", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo '{"name":"bob"}' | yq .name`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "bob" {
			t.Errorf("expected 'bob', got %q", out)
		}
	})

	t.Run("yq array element access", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `printf 'items:\n  - a\n  - b\n  - c\n' | yq '.items[1]'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "b" {
			t.Errorf("expected 'b', got %q", out)
		}
	})

	t.Run("yq -r raw string output", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo 'greeting: hello world' | yq -r .greeting`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "hello world" {
			t.Errorf("expected 'hello world', got %q", out)
		}
	})

	t.Run("yq -n null input", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `yq -n '{a: 1}'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, "a") {
			t.Errorf("unexpected output: %q", out)
		}
	})
}
