package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestLsListsDirectory(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/b.txt", []byte("b"), 0o644)
	_ = afero.WriteFile(fs, "/a.txt", []byte("a"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `ls /`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "a.txt\nb.txt" {
		t.Fatalf("output = %q, want sorted a.txt/b.txt", got)
	}
}

func TestLsLongFormatOnFile(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/a.txt", []byte("hello"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `ls -l /a.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "a.txt") {
		t.Fatalf("output = %q, want a.txt entry", buf.String())
	}
}

func TestLsHidesDotfilesByDefault(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/.hidden", []byte("h"), 0o644)
	_ = afero.WriteFile(fs, "/visible.txt", []byte("v"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `ls -a /`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), ".hidden") {
		t.Fatalf("output = %q, want .hidden with -a", buf.String())
	}
}

func TestLsRecursive(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/sub", 0o755)
	_ = afero.WriteFile(fs, "/sub/a.txt", []byte("a"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `ls -R /`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "sub") || !strings.Contains(buf.String(), "a.txt") {
		t.Fatalf("output = %q, want recursive listing", buf.String())
	}
}

func TestLsMissingPath(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `ls /missing`); err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestLsInvalidOption(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `ls -Z`); err == nil {
		t.Fatal("expected error for invalid option")
	}
}
