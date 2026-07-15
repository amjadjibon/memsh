package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestRmdirRemovesEmptyDirectory(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/empty", 0o755)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `rmdir /empty`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := fs.Stat("/empty"); err == nil {
		t.Fatal("expected directory to be removed")
	}
}

func TestRmdirNonEmptyFails(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/dir", 0o755)
	_ = afero.WriteFile(fs, "/dir/a.txt", []byte("data"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `rmdir /dir`); err == nil {
		t.Fatal("expected error removing non-empty directory")
	}
}

func TestRmdirNotADirectory(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/a.txt", []byte("data"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `rmdir /a.txt`); err == nil {
		t.Fatal("expected error for non-directory target")
	}
}

func TestRmdirMultipleReportsEach(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/a", 0o755)
	_ = fs.MkdirAll("/b", 0o755)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `rmdir /a /b`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "removing directory") {
		t.Fatalf("output = %q, want removing directory message", buf.String())
	}
}

func TestRmdirMissingOperand(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `rmdir`); err == nil {
		t.Fatal("expected error for missing operand")
	}
}
