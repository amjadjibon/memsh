package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

// "pwd" is a recognized builtin name in mvdan.cc/sh's runner, so a literal
// leading "pwd" is always handled by its own implementation, never reaching
// PwdPlugin.Run. Reach it dynamically via "xargs" instead.

func TestPwdPluginViaXargs(t *testing.T) {
	// mvdan.cc/sh's own "cd" builtin changes the interpreter's directory
	// directly and never calls CdPlugin. Plugins still see the new directory
	// because the exec handler takes cwd from the handler context.
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/sub", 0o755)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `cd /sub && echo x | xargs pwd`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "/sub" {
		t.Fatalf("output = %q, want /sub (the directory after cd)", got)
	}
}
