package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/amjadjibon/memsh/shell"
)

func TestFind(t *testing.T) {
	ctx := context.Background()

	t.Run("find lists all entries under path", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/d", 0755)
		afero.WriteFile(fs, "/d/a.txt", []byte(""), 0644)
		afero.WriteFile(fs, "/d/b.go", []byte(""), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "find /d"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "a.txt") {
			t.Errorf("output %q does not contain 'a.txt'", out)
		}
		if !strings.Contains(out, "b.go") {
			t.Errorf("output %q does not contain 'b.go'", out)
		}
	})

	t.Run("find -name filters by glob", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/d", 0755)
		afero.WriteFile(fs, "/d/a.txt", []byte(""), 0644)
		afero.WriteFile(fs, "/d/b.go", []byte(""), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "find /d -name *.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "a.txt") {
			t.Errorf("output %q does not contain 'a.txt'", out)
		}
		if strings.Contains(out, "b.go") {
			t.Errorf("output %q should not contain 'b.go'", out)
		}
	})

	t.Run("find -type f lists only files", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/d/sub", 0755)
		afero.WriteFile(fs, "/d/f.txt", []byte(""), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "find /d -type f"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "f.txt") {
			t.Errorf("output %q does not contain 'f.txt'", out)
		}
		if strings.Contains(out, "sub") {
			t.Errorf("output %q should not contain directory 'sub'", out)
		}
	})

	t.Run("find -type d lists only directories", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/d/sub", 0755)
		afero.WriteFile(fs, "/d/f.txt", []byte(""), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "find /d -type d"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if strings.Contains(out, "f.txt") {
			t.Errorf("output %q should not contain file 'f.txt'", out)
		}
		if !strings.Contains(out, "sub") {
			t.Errorf("output %q should contain directory 'sub'", out)
		}
	})
}
