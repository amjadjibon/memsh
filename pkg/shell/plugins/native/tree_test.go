package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestTree(t *testing.T) {
	ctx := context.Background()

	t.Run("basic tree", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/root/sub", 0o755)
		afero.WriteFile(fs, "/root/a.txt", []byte(""), 0o644)
		afero.WriteFile(fs, "/root/b.txt", []byte(""), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "tree /root"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "a.txt") {
			t.Errorf("output %q missing 'a.txt'", out)
		}
		if !strings.Contains(out, "b.txt") {
			t.Errorf("output %q missing 'b.txt'", out)
		}
		if !strings.Contains(out, "sub") {
			t.Errorf("output %q missing 'sub'", out)
		}
		if !strings.Contains(out, "├──") && !strings.Contains(out, "└──") {
			t.Errorf("output %q missing tree connectors", out)
		}
	})

	t.Run("depth limit -L 1", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/root/sub/deep", 0o755)
		afero.WriteFile(fs, "/root/top.txt", []byte(""), 0o644)
		afero.WriteFile(fs, "/root/sub/nested.txt", []byte(""), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "tree -L 1 /root"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "top.txt") {
			t.Errorf("output %q missing 'top.txt'", out)
		}
		if strings.Contains(out, "nested.txt") {
			t.Errorf("output %q should not contain 'nested.txt' at depth > 1", out)
		}
		if strings.Contains(out, "deep") {
			t.Errorf("output %q should not contain 'deep' at depth > 1", out)
		}
	})

	t.Run("hidden files -a", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/root", 0o755)
		afero.WriteFile(fs, "/root/visible.txt", []byte(""), 0o644)
		afero.WriteFile(fs, "/root/.hidden", []byte(""), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))

		// Without -a, hidden file should not appear.
		if err := s.Run(ctx, "tree /root"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(buf.String(), ".hidden") {
			t.Errorf("output %q should not contain '.hidden' without -a", buf.String())
		}

		buf.Reset()
		s2 := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s2.Run(ctx, "tree -a /root"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), ".hidden") {
			t.Errorf("output %q should contain '.hidden' with -a", buf.String())
		}
	})

	t.Run("dirs only -d", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/root/subdir", 0o755)
		afero.WriteFile(fs, "/root/file.txt", []byte(""), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "tree -d /root"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if strings.Contains(out, "file.txt") {
			t.Errorf("output %q should not contain 'file.txt' with -d", out)
		}
		if !strings.Contains(out, "subdir") {
			t.Errorf("output %q missing 'subdir'", out)
		}
	})

	t.Run("full paths -f", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/root", 0o755)
		afero.WriteFile(fs, "/root/file.txt", []byte(""), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "tree -f /root"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "/root/file.txt") {
			t.Errorf("output %q should contain full path '/root/file.txt'", out)
		}
	})

	t.Run("summary line", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/root/sub1", 0o755)
		fs.MkdirAll("/root/sub2", 0o755)
		afero.WriteFile(fs, "/root/a.txt", []byte(""), 0o644)
		afero.WriteFile(fs, "/root/b.txt", []byte(""), 0o644)
		afero.WriteFile(fs, "/root/sub1/c.txt", []byte(""), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "tree /root"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "2 directories") {
			t.Errorf("output %q missing '2 directories'", out)
		}
		if !strings.Contains(out, "3 files") {
			t.Errorf("output %q missing '3 files'", out)
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))

		err := s.Run(ctx, "tree /nonexistent")
		if err == nil {
			t.Fatal("expected error for non-existent path, got nil")
		}
		if !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("error %q should mention the path 'nonexistent'", err.Error())
		}
	})

	t.Run("path is a file not a directory", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/root/file.txt", []byte(""), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))

		err := s.Run(ctx, "tree /root/file.txt")
		if err == nil {
			t.Fatal("expected error when path is a file, got nil")
		}
		if !strings.Contains(err.Error(), "Not a directory") {
			t.Errorf("error %q should mention 'Not a directory'", err.Error())
		}
	})

	t.Run("-L missing argument", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/root", 0o755)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))

		err := s.Run(ctx, "tree -L")
		if err == nil {
			t.Fatal("expected error for missing -L argument, got nil")
		}
		if !strings.Contains(err.Error(), "-L") {
			t.Errorf("error %q should mention '-L'", err.Error())
		}
	})

	t.Run("-L invalid non-numeric argument", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/root", 0o755)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))

		err := s.Run(ctx, "tree -L abc /root")
		if err == nil {
			t.Fatal("expected error for non-numeric -L value, got nil")
		}
		if !strings.Contains(err.Error(), "abc") {
			t.Errorf("error %q should mention the invalid value 'abc'", err.Error())
		}
	})

	t.Run("-L zero is invalid", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/root", 0o755)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))

		err := s.Run(ctx, "tree -L 0 /root")
		if err == nil {
			t.Fatal("expected error for -L 0, got nil")
		}
	})

	t.Run("unknown flag returns error", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/root", 0o755)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))

		err := s.Run(ctx, "tree -z /root")
		if err == nil {
			t.Fatal("expected error for unknown flag -z, got nil")
		}
		if !strings.Contains(err.Error(), "z") {
			t.Errorf("error %q should mention the invalid flag 'z'", err.Error())
		}
	})

	t.Run("combined flags -ad", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/root/.hidden_dir", 0o755)
		afero.WriteFile(fs, "/root/.hidden_file", []byte(""), 0o644)
		afero.WriteFile(fs, "/root/visible.txt", []byte(""), 0o644)
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "tree -ad /root"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, ".hidden_dir") {
			t.Errorf("output %q missing '.hidden_dir'", out)
		}
		if strings.Contains(out, ".hidden_file") {
			t.Errorf("output %q should not contain '.hidden_file' with -d", out)
		}
		if strings.Contains(out, "visible.txt") {
			t.Errorf("output %q should not contain 'visible.txt' with -d", out)
		}
	})
}
