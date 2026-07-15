package native_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/amjadjibon/memsh/pkg/shell/plugins/native"
	"github.com/amjadjibon/memsh/tests"
)

func TestExitPluginCallsShellCtxExit(t *testing.T) {
	wantErr := errors.New("boom")
	ctx := plugins.WithShellContext(context.Background(), plugins.ShellContext{
		Exit: func() error { return wantErr },
	})
	if err := (native.ExitPlugin{}).Run(ctx, []string{"exit"}); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestQuitStopsScriptExecution(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	_ = s.Run(ctx, "echo before; quit; echo after")
	out := buf.String()
	if !strings.Contains(out, "before") {
		t.Fatalf("output = %q, want to contain before", out)
	}
	if strings.Contains(out, "after") {
		t.Fatalf("output = %q, want quit to stop further commands", out)
	}
}
