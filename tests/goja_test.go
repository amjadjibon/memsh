package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestGoja(t *testing.T) {
	ctx := context.Background()

	t.Run("goja -e executes inline code", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `goja -e 'console.log("hello from goja")'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "hello from goja" {
			t.Errorf("expected 'hello from goja', got %q", out)
		}
	})

	t.Run("goja -e supports math operations", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `goja -e 'console.log(2 + 3)'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "5" {
			t.Errorf("expected '5', got %q", out)
		}
	})

	t.Run("goja executes file from virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/script.js", []byte(`console.log("hello from file")`), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, `goja /script.js`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "hello from file" {
			t.Errorf("expected 'hello from file', got %q", out)
		}
	})

	t.Run("goja reads from stdin", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo 'console.log("test")' | goja`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "test" {
			t.Errorf("expected 'test', got %q", out)
		}
	})

	t.Run("goja -e handles multiple console.log arguments", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `goja -e 'console.log("hello", "world", 42)'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "hello world 42" {
			t.Errorf("expected 'hello world 42', got %q", out)
		}
	})

	t.Run("goja -e supports modern JavaScript features", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `goja -e 'const arr = [1,2,3]; const doubled = arr.map(x => x * 2); console.log(doubled.join(","))'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "2,4,6" {
			t.Errorf("expected '2,4,6', got %q", out)
		}
	})

	t.Run("goja -e reports syntax errors", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		err := s.Run(ctx, `goja -e 'console.log('`)
		if err == nil {
			t.Fatalf("expected error for invalid syntax, got nil")
		}
		if !strings.Contains(err.Error(), "goja:") {
			t.Errorf("expected 'goja:' error prefix, got %v", err)
		}
	})

	t.Run("goja fs.readFile reads from virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/data.txt", []byte("hello from fs"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, `goja -e 'const content = fs.readFile("/data.txt"); console.log(content)'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "hello from fs" {
			t.Errorf("expected 'hello from fs', got %q", out)
		}
	})
}
