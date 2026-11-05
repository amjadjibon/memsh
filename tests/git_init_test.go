package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestGitInitFlags(t *testing.T) {
	fs := afero.NewMemMapFs()
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(context.Background(), `git init -q /r && git init --initial-branch=main /s`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out := buf.String(); strings.Contains(out, "/r") {
		t.Errorf("-q should suppress output, got %q", out)
	}
	if ok, _ := afero.DirExists(fs, "/r/.git"); !ok {
		t.Error("/r/.git not created")
	}
	if ok, _ := afero.DirExists(fs, "/-q"); ok {
		t.Error("-q was treated as a directory")
	}
	head, _ := afero.ReadFile(fs, "/s/.git/HEAD")
	if got := strings.TrimSpace(string(head)); got != "ref: refs/heads/main" {
		t.Errorf("HEAD = %q", got)
	}
	if err := s.Run(context.Background(), `git init --bogus /t`); err == nil {
		t.Error("unknown option should fail")
	}
}
