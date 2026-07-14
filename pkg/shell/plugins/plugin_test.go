package plugins_test

import (
	"context"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

func TestShellCtxReturnsZeroValueWhenMissing(t *testing.T) {
	sc := plugins.ShellCtx(context.Background())
	if sc.FS != nil {
		t.Fatalf("FS = %#v, want nil", sc.FS)
	}
	if sc.Cwd != "" {
		t.Fatalf("Cwd = %q, want empty", sc.Cwd)
	}
	if sc.AllowHostListen {
		t.Fatal("AllowHostListen = true, want false")
	}
}

func TestWithShellContextInjectsContext(t *testing.T) {
	fs := afero.NewMemMapFs()
	want := plugins.ShellContext{
		FS:              fs,
		Cwd:             "/work",
		AllowHostListen: true,
		Env: func(key string) string {
			if key == "USER" {
				return "memsh"
			}
			return ""
		},
		ResolvePath: func(path string) string { return "/work/" + path },
	}

	ctx := plugins.WithShellContext(context.Background(), want)
	got := plugins.ShellCtx(ctx)

	if got.FS != fs {
		t.Fatal("ShellCtx returned different filesystem")
	}
	if got.Cwd != "/work" {
		t.Fatalf("Cwd = %q, want /work", got.Cwd)
	}
	if !got.AllowHostListen {
		t.Fatal("AllowHostListen = false, want true")
	}
	if got.Env("USER") != "memsh" {
		t.Fatalf("Env(USER) = %q, want memsh", got.Env("USER"))
	}
	if got.ResolvePath("file.txt") != "/work/file.txt" {
		t.Fatalf("ResolvePath = %q, want /work/file.txt", got.ResolvePath("file.txt"))
	}
}
