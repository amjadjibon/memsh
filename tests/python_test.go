package tests

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestPython(t *testing.T) {
	// Skip unless CI_PYTHON_TEST=1 is set
	if os.Getenv("CI_PYTHON_TEST") != "1" {
		t.Skip("Skipping Python tests: set CI_PYTHON_TEST=1 to enable")
	}

	ctx := context.Background()

	t.Run("python -c executes inline code", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `python -c 'print("hello from python")'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "hello from python") {
			t.Errorf("expected 'hello from python', got %q", out)
		}
	})

	t.Run("python -c with math operations", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `python -c 'print(2 + 3)'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "5" {
			t.Errorf("expected '5', got %q", out)
		}
	})

	t.Run("python executes file from virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/script.py", []byte(`print("hello from file")`), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `python /script.py`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "hello from file") {
			t.Errorf("expected 'hello from file', got %q", out)
		}
	})

	t.Run("python reads from stdin", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `echo 'print("stdin test")' | python`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "stdin test") {
			t.Errorf("expected 'stdin test', got %q", out)
		}
	})

	t.Run("python with list operations", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `python -c 'nums = [1, 2, 3, 4, 5]; print(sum(nums))'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "15" {
			t.Errorf("expected '15', got %q", out)
		}
	})

	t.Run("python file I/O operations", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/test.txt", []byte("hello from fs"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `python -c 'print(open("/test.txt").read())'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "hello from fs" {
			t.Errorf("expected 'hello from fs', got %q", out)
		}
	})

	t.Run("python with syntax error", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		err := s.Run(ctx, `python -c 'print("unclosed string'`)
		if err == nil {
			t.Fatal("expected error for invalid Python syntax")
		}
		// Python errors should contain useful information
		if !strings.Contains(err.Error(), "python:") && !strings.Contains(err.Error(), "SyntaxError") {
			t.Errorf("error should mention python: or SyntaxError, got %v", err)
		}
	})
}

// TestPythonFile is kept for backward compatibility
func TestPythonFile(t *testing.T) {
	if os.Getenv("CI_PYTHON_TEST") == "" {
		t.Skip("Skipping Python tests: set CI_PYTHON_TEST=1 to enable")
	}
	TestPython(t)
}
