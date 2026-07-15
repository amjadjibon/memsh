package native_test

import (
	"context"
	"errors"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/amjadjibon/memsh/pkg/shell/plugins/native"
)

// "source" and "." are recognized builtin names in mvdan.cc/sh's runner, so
// they are always handled by its own case "source", "." implementation and
// never reach our exec handler. Exercise the plugins directly instead.

func TestSourcePluginCallsSourceFile(t *testing.T) {
	var gotPath string
	ctx := plugins.WithShellContext(context.Background(), plugins.ShellContext{
		SourceFile: func(_ context.Context, path string) error { gotPath = path; return nil },
	})
	if err := (native.SourcePlugin{}).Run(ctx, []string{"source", "/script.sh"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/script.sh" {
		t.Fatalf("path = %q, want /script.sh", gotPath)
	}
}

func TestSourcePluginMissingArgument(t *testing.T) {
	ctx := plugins.WithShellContext(context.Background(), plugins.ShellContext{})
	if err := (native.SourcePlugin{}).Run(ctx, []string{"source"}); err == nil {
		t.Fatal("expected error for missing file argument")
	}
}

func TestSourcePluginPropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	ctx := plugins.WithShellContext(context.Background(), plugins.ShellContext{
		SourceFile: func(_ context.Context, path string) error { return wantErr },
	})
	if err := (native.SourcePlugin{}).Run(ctx, []string{"source", "/script.sh"}); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestDotPluginDelegatesToSource(t *testing.T) {
	var gotPath string
	ctx := plugins.WithShellContext(context.Background(), plugins.ShellContext{
		SourceFile: func(_ context.Context, path string) error { gotPath = path; return nil },
	})
	if err := (native.DotPlugin{}).Run(ctx, []string{".", "/script.sh"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/script.sh" {
		t.Fatalf("path = %q, want /script.sh", gotPath)
	}
}
