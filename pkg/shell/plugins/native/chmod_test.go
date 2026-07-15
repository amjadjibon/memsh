package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestChmodOctalMode(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/a.txt", []byte("data"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `chmod 0755 /a.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := fs.Stat("/a.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestChmodSymbolicMode(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/a.txt", []byte("data"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `chmod u+x /a.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := fs.Stat("/a.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("mode = %v, want owner execute bit set", info.Mode().Perm())
	}
}

func TestChmodRecursive(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/dir", 0o755)
	_ = afero.WriteFile(fs, "/dir/a.txt", []byte("data"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `chmod -R 0700 /dir`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := fs.Stat("/dir/a.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %v, want 0700", info.Mode().Perm())
	}
}

func TestChmodMissingOperand(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `chmod 0755`); err == nil {
		t.Fatal("expected error for missing operand")
	}
}

func TestChmodInvalidOption(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `chmod -Z 0755 /a.txt`); err == nil {
		t.Fatal("expected error for invalid option")
	}
}
