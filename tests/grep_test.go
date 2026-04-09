package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestGrep(t *testing.T) {
	ctx := context.Background()

	t.Run("grep matches lines in file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("apple\nbanana\napricot\n"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "grep apple /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "apple") {
			t.Errorf("output %q does not contain 'apple'", out)
		}
		if strings.Contains(out, "banana") {
			t.Errorf("output %q should not contain 'banana'", out)
		}
	})

	t.Run("grep -i does case-insensitive match", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("Hello\nworld\n"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "grep -i hello /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "Hello") {
			t.Errorf("output %q does not contain 'Hello'", buf.String())
		}
	})

	t.Run("grep -v inverts match", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("keep\nskip\nkeep2\n"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "grep -v skip /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if strings.Contains(out, "skip") {
			t.Errorf("output %q should not contain 'skip'", out)
		}
		if !strings.Contains(out, "keep") {
			t.Errorf("output %q should contain 'keep'", out)
		}
	})

	t.Run("grep -n shows line numbers", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("foo\nbar\nfoo2\n"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "grep -n foo /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "1:") {
			t.Errorf("output %q should contain line number '1:'", out)
		}
	})

	t.Run("grep stdin via pipe", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		if err := s.Run(ctx, `echo "hello world" | grep hello`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "hello") {
			t.Errorf("output %q does not contain 'hello'", buf.String())
		}
	})
}
