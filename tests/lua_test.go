package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestLua(t *testing.T) {
	ctx := context.Background()

	t.Run("lua -e executes inline code", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `lua -e 'print("hello from lua")'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "hello from lua") {
			t.Errorf("expected 'hello from lua', got %q", out)
		}
	})

	t.Run("lua -e with math", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `lua -e 'print(2 + 3)'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "5" {
			t.Errorf("expected '5', got %q", out)
		}
	})

	t.Run("lua executes file from virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/script.lua", []byte(`print("hello from file")`), 0o644)

		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, `lua /script.lua`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "hello from file") {
			t.Errorf("expected file output, got %q", out)
		}
	})

	t.Run("lua reads code from stdin", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo 'print("stdin test")' | lua`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "stdin test") {
			t.Errorf("expected 'stdin test', got %q", out)
		}
	})

	t.Run("lua with syntax error", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		err := s.Run(ctx, `lua -e 'print("unclosed string'`)
		if err == nil {
			t.Fatal("expected error for invalid Lua syntax")
		}
		if !strings.Contains(err.Error(), "lua:") {
			t.Errorf("error should mention lua:, got %v", err)
		}
	})

	t.Run("lua with table operations", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `lua -e 't = {1, 2, 3}; print(#t)'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "3" {
			t.Errorf("expected '3', got %q", out)
		}
	})
}
