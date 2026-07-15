package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestRmRemovesFile(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/a.txt", []byte("data"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `rm /a.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := fs.Stat("/a.txt"); err == nil {
		t.Fatal("expected file to be removed")
	}
}

func TestRmDirectoryRequiresRecursive(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/dir", 0o755)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `rm /dir`); err == nil {
		t.Fatal("expected error removing directory without -r")
	}
}

func TestRmRecursiveVerbose(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/dir", 0o755)
	_ = afero.WriteFile(fs, "/dir/a.txt", []byte("data"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `rm -rv /dir`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "removed directory") {
		t.Fatalf("output = %q, want verbose removal message", buf.String())
	}
	if _, err := fs.Stat("/dir"); err == nil {
		t.Fatal("expected directory to be removed")
	}
}

func TestRmForceMissingFileNoError(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `rm -f /missing.txt`); err != nil {
		t.Fatalf("unexpected error with -f on missing file: %v", err)
	}
}

func TestRmMissingOperandWithoutForce(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `rm`); err == nil {
		t.Fatal("expected error for missing operand")
	}
}

func TestRmDirFlagRemovesEmptyDir(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/empty", 0o755)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `rm -d /empty`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
