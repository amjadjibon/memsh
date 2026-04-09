package tests

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestRuby(t *testing.T) {
	// Skip unless CI_RUBY_TEST=1 is set
	if os.Getenv("CI_RUBY_TEST") != "1" {
		t.Skip("Skipping Ruby tests: set CI_RUBY_TEST=1 to enable")
	}

	ctx := context.Background()

	t.Run("ruby -e executes inline code", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `ruby -e 'puts "hello from ruby"'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "hello from ruby" {
			t.Errorf("expected 'hello from ruby', got %q", out)
		}
	})

	t.Run("ruby -e with math operations", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `ruby -e 'puts 2 + 3'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "5" {
			t.Errorf("expected '5', got %q", out)
		}
	})

	t.Run("ruby executes file from virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/script.rb", []byte(`puts "hello from file"`), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `ruby /script.rb`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "hello from file" {
			t.Errorf("expected 'hello from file', got %q", out)
		}
	})

	t.Run("ruby reads from stdin", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `echo 'puts "stdin test"' | ruby`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "stdin test") {
			t.Errorf("expected 'stdin test', got %q", out)
		}
	})

	t.Run("ruby with array operations", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `ruby -e 'puts [1,2,3,4,5].sum'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "15" {
			t.Errorf("expected '15', got %q", out)
		}
	})

	t.Run("ruby with hash operations", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `ruby -e 'h = {a: 1, b: 2}; puts h[:a]'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "1" {
			t.Errorf("expected '1', got %q", out)
		}
	})

	t.Run("ruby file I/O operations", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/test.txt", []byte("hello from fs"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `ruby -e 'puts File.read("/test.txt")'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "hello from fs" {
			t.Errorf("expected 'hello from fs', got %q", out)
		}
	})

	t.Run("ruby with string interpolation", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `ruby -e 'name = "Ruby"; puts "Hello from #{name}"'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "Hello from Ruby" {
			t.Errorf("expected 'Hello from Ruby', got %q", out)
		}
	})

	t.Run("ruby with syntax error", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		err := s.Run(ctx, `ruby -e 'puts "unclosed string'`)
		if err == nil {
			t.Fatal("expected error for invalid Ruby syntax")
		}
		// Ruby errors should contain useful information
		if !strings.Contains(err.Error(), "ruby:") {
			t.Errorf("error should mention ruby:, got %v", err)
		}
	})

	t.Run("ruby version constant", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `ruby -e 'puts RUBY_VERSION'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, "3.") {
			t.Errorf("expected Ruby version to contain '3.', got %q", out)
		}
	})
}

// TestRubyFile is kept for backward compatibility
func TestRubyFile(t *testing.T) {
	if os.Getenv("CI_PYTHON_TEST") == "" {
		t.Skip("Skipping Ruby tests: set CI_PYTHON_TEST=1 to enable")
	}
	TestRuby(t)
}
