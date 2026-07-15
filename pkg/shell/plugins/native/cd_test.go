package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/amjadjibon/memsh/pkg/shell/plugins/native"
	"github.com/amjadjibon/memsh/tests"
)

// "cd" is a recognized builtin name in mvdan.cc/sh's runner, so it is always
// handled by its own case "cd" implementation (which happens to also work
// against our virtual FS) and never reaches CdPlugin.Run. Exercise the
// plugin directly to cover runCd's branches.

func TestCdPluginDefaultsToRoot(t *testing.T) {
	var gotDir string
	ctx := plugins.WithShellContext(context.Background(), plugins.ShellContext{
		SetCwd: func(dir string) error { gotDir = dir; return nil },
	})
	if err := (native.CdPlugin{}).Run(ctx, []string{"cd"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotDir != "/" {
		t.Fatalf("dir = %q, want /", gotDir)
	}
}

func TestCdPluginChangesToGivenDirectory(t *testing.T) {
	var gotDir string
	ctx := plugins.WithShellContext(context.Background(), plugins.ShellContext{
		SetCwd: func(dir string) error { gotDir = dir; return nil },
	})
	if err := (native.CdPlugin{}).Run(ctx, []string{"cd", "/sub"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotDir != "/sub" {
		t.Fatalf("dir = %q, want /sub", gotDir)
	}
}

func TestCdPluginTooManyArguments(t *testing.T) {
	ctx := plugins.WithShellContext(context.Background(), plugins.ShellContext{})
	if err := (native.CdPlugin{}).Run(ctx, []string{"cd", "/a", "/b"}); err == nil {
		t.Fatal("expected error for too many arguments")
	}
}

func TestCdIntegratesWithVirtualFilesystem(t *testing.T) {
	// End-to-end sanity check that the shell's own "cd" builtin still moves
	// around the virtual filesystem we configure, independent of CdPlugin.
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/sub", 0o755)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `cd /sub && pwd`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "/sub" {
		t.Fatalf("pwd = %q, want /sub", got)
	}
}
