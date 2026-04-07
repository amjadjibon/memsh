package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/shell"
)

func TestMktemp(t *testing.T) {
	ctx := context.Background()

	newShell := func(t *testing.T, buf *strings.Builder) (*shell.Shell, afero.Fs) {
		t.Helper()
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/tmp", 0o755)
		s := NewTestShell(t, buf, shell.WithFS(fs))
		return s, fs
	}

	t.Run("creates a file in /tmp", func(t *testing.T) {
		var buf strings.Builder
		s, fs := newShell(t, &buf)
		if err := s.Run(ctx, "mktemp"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := strings.TrimSpace(buf.String())
		if !strings.HasPrefix(path, "/tmp/") {
			t.Errorf("expected /tmp/ prefix, got %q", path)
		}
		if _, err := fs.Stat(path); err != nil {
			t.Errorf("file not created: %v", err)
		}
	})

	t.Run("-d creates a directory", func(t *testing.T) {
		var buf strings.Builder
		s, fs := newShell(t, &buf)
		if err := s.Run(ctx, "mktemp -d"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := strings.TrimSpace(buf.String())
		info, err := fs.Stat(path)
		if err != nil {
			t.Fatalf("directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("expected directory, got file: %q", path)
		}
	})

	t.Run("-p uses alternate base directory", func(t *testing.T) {
		var buf strings.Builder
		s, fs := newShell(t, &buf)
		fs.MkdirAll("/var/tmp", 0o755)
		if err := s.Run(ctx, "mktemp -p /var/tmp"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := strings.TrimSpace(buf.String())
		if !strings.HasPrefix(path, "/var/tmp/") {
			t.Errorf("expected /var/tmp/ prefix, got %q", path)
		}
		if _, err := fs.Stat(path); err != nil {
			t.Errorf("file not created: %v", err)
		}
	})

	t.Run("custom template", func(t *testing.T) {
		var buf strings.Builder
		s, fs := newShell(t, &buf)
		if err := s.Run(ctx, "mktemp /tmp/myapp.XXXXXX"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := strings.TrimSpace(buf.String())
		if !strings.HasPrefix(path, "/tmp/myapp.") {
			t.Errorf("expected /tmp/myapp. prefix, got %q", path)
		}
		if _, err := fs.Stat(path); err != nil {
			t.Errorf("file not created at %q: %v", path, err)
		}
	})

	t.Run("--suffix appended after substitution", func(t *testing.T) {
		var buf strings.Builder
		s, fs := newShell(t, &buf)
		if err := s.Run(ctx, "mktemp --suffix .json"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := strings.TrimSpace(buf.String())
		if !strings.HasSuffix(path, ".json") {
			t.Errorf("expected .json suffix, got %q", path)
		}
		if _, err := fs.Stat(path); err != nil {
			t.Errorf("file not created: %v", err)
		}
	})

	t.Run("-u dry-run prints path without creating", func(t *testing.T) {
		var buf strings.Builder
		s, fs := newShell(t, &buf)
		if err := s.Run(ctx, "mktemp -u"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := strings.TrimSpace(buf.String())
		if path == "" {
			t.Fatal("expected a path to be printed")
		}
		if _, err := fs.Stat(path); err == nil {
			t.Errorf("dry-run should not create the file: %q", path)
		}
	})

	t.Run("each call returns a unique path", func(t *testing.T) {
		var buf strings.Builder
		s, _ := newShell(t, &buf)
		if err := s.Run(ctx, "mktemp && mktemp && mktemp"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		paths := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(paths) != 3 {
			t.Fatalf("expected 3 paths, got %d: %q", len(paths), buf.String())
		}
		seen := map[string]bool{}
		for _, p := range paths {
			if seen[p] {
				t.Errorf("duplicate path: %q", p)
			}
			seen[p] = true
		}
	})

	t.Run("path captured in shell variable", func(t *testing.T) {
		var buf strings.Builder
		s, fs := newShell(t, &buf)
		if err := s.Run(ctx, `F=$(mktemp) && echo "created $F"`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.HasPrefix(out, "created /tmp/") {
			t.Errorf("got %q", out)
		}
		// extract path from output
		path := strings.TrimPrefix(out, "created ")
		if _, err := fs.Stat(path); err != nil {
			t.Errorf("file not found at %q: %v", path, err)
		}
	})

	t.Run("-d used as working directory", func(t *testing.T) {
		var buf strings.Builder
		s, fs := newShell(t, &buf)
		// print the dir to stdout AND write a file inside it
		if err := s.Run(ctx, `D=$(mktemp -d) && echo "$D" && echo hello > $D/hello.txt`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		dir := strings.TrimSpace(buf.String())
		data, err := afero.ReadFile(fs, dir+"/hello.txt")
		if err != nil {
			t.Fatalf("file not found in temp dir %q: %v", dir, err)
		}
		if strings.TrimSpace(string(data)) != "hello" {
			t.Errorf("got %q", data)
		}
	})

	t.Run("template without path uses /tmp as parent", func(t *testing.T) {
		var buf strings.Builder
		s, fs := newShell(t, &buf)
		if err := s.Run(ctx, "mktemp test.XXXXXX"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := strings.TrimSpace(buf.String())
		if !strings.HasPrefix(path, "/tmp/test.") {
			t.Errorf("expected /tmp/test.* got %q", path)
		}
		if _, err := fs.Stat(path); err != nil {
			t.Errorf("file not created: %v", err)
		}
	})

	t.Run("invalid template (no Xs) exits 1", func(t *testing.T) {
		var buf strings.Builder
		s, _ := newShell(t, &buf)
		err := s.Run(ctx, "mktemp -q /tmp/noxs")
		if err == nil {
			t.Error("expected error for template without Xs")
		}
	})
}
