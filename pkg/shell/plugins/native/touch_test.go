package native_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestTouchCreatesFile(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `touch /a.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := fs.Stat("/a.txt"); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestTouchNoCreateSkipsMissingFile(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `touch -c /missing.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := fs.Stat("/missing.txt"); err == nil {
		t.Fatal("expected file to still be missing with -c")
	}
}

func TestTouchWithReference(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/ref.txt", []byte("data"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `touch -r /ref.txt /new.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	refInfo, _ := fs.Stat("/ref.txt")
	newInfo, err := fs.Stat("/new.txt")
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if delta := newInfo.ModTime().Sub(refInfo.ModTime()); delta < -time.Second || delta > time.Second {
		t.Fatalf("modtime = %v, want close to %v", newInfo.ModTime(), refInfo.ModTime())
	}
}

func TestTouchMissingOperand(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `touch`); err == nil {
		t.Fatal("expected error for missing operand")
	}
}
