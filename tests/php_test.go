package tests

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestPhp(t *testing.T) {
	// Skip unless CI_PHP_TEST=1 is set
	if os.Getenv("CI_PHP_TEST") != "1" {
		t.Skip("Skipping PHP tests: set CI_PHP_TEST=1 to enable")
	}

	ctx := context.Background()

	t.Run("php executes code from stdin", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `echo '<?php echo "hello from php\n"; ?>' | php`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "hello from php") {
			t.Errorf("expected 'hello from php', got %q", out)
		}
	})

	t.Run("php with math operations", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `echo '<?php echo 2 + 3; ?>' | php`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		// PHP outputs CGI headers, so we need to extract the actual output
		lines := strings.Split(out, "\n")
		result := ""
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "5" {
				result = trimmed
				break
			}
		}
		if result != "5" {
			t.Errorf("expected '5', got %q (full output: %q)", result, out)
		}
	})

	t.Run("php executes file from virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/script.php", []byte(`<?php echo "hello from file\n";`), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `php /script.php`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "hello from file") {
			t.Errorf("expected 'hello from file', got %q", out)
		}
	})

	t.Run("php reads from stdin", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `echo '<?php echo "stdin test\n"; ?>' | php`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "stdin test") {
			t.Errorf("expected 'stdin test', got %q", out)
		}
	})

	t.Run("php with array operations", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `echo '<?php $nums = [1,2,3,4,5]; echo array_sum($nums); ?>' | php`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, "15") {
			t.Errorf("expected output to contain '15', got %q", out)
		}
	})

	t.Run("php file I/O operations", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/test.txt", []byte("hello from fs"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `echo '<?php echo file_get_contents("/test.txt"); ?>' | php`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		// PHP outputs CGI headers, so we need to extract the actual output
		lines := strings.Split(out, "\n")
		result := ""
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "hello from fs" {
				result = trimmed
				break
			}
		}
		if result != "hello from fs" {
			t.Errorf("expected 'hello from fs', got %q (full output: %q)", result, out)
		}
	})

	t.Run("php with string interpolation", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `echo '<?php $name = "PHP"; echo "Hello from $name\n"; ?>' | php`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Hello from PHP") {
			t.Errorf("expected 'Hello from PHP', got %q", out)
		}
	})

	t.Run("php with variables and echo", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `echo '<?php $x = 10; $y = 20; echo $x + $y; ?>' | php`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, "30") {
			t.Errorf("expected output to contain '30', got %q", out)
		}
	})

	t.Run("php with syntax error", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		err := s.Run(ctx, `echo '<?php echo "unclosed string; ?>' | php`)
		if err == nil {
			t.Fatal("expected error for invalid PHP syntax")
		}
		// PHP errors should contain useful information
		if !strings.Contains(err.Error(), "php:") {
			t.Errorf("error should mention php:, got %v", err)
		}
	})

	t.Run("php version constant", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `echo '<?php echo phpversion(); ?>' | php`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, "8.2") {
			t.Errorf("expected PHP version to contain '8.2', got %q", out)
		}
	})

	t.Run("php write to file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `echo '<?php file_put_contents("/output.txt", "written by php"); ?>' | php`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify the file was written
		content, err := afero.ReadFile(fs, "/output.txt")
		if err != nil {
			t.Fatalf("failed to read written file: %v", err)
		}
		if string(content) != "written by php" {
			t.Errorf("expected 'written by php', got %q", string(content))
		}
	})

	t.Run("php with foreach loop", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(true))

		if err := s.Run(ctx, `echo '<?php $arr = [1,2,3]; foreach($arr as $n) { echo "$n "; } ?>' | php`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		// Check for the output pattern
		if !strings.Contains(out, "1") && !strings.Contains(out, "2") && !strings.Contains(out, "3") {
			t.Errorf("expected output to contain numbers, got %q", out)
		}
	})
}

// TestPhpFile is kept for backward compatibility
func TestPhpFile(t *testing.T) {
	if os.Getenv("CI_PYTHON_TEST") == "" {
		t.Skip("Skipping PHP tests: set CI_PYTHON_TEST=1 to enable")
	}
	TestPhp(t)
}
