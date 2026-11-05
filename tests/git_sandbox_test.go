package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

// TestGitCannotReachHostRepositories guards against go-git's local/file
// transport, which runs host git-upload-pack against real OS paths instead of
// the virtual filesystem.
func TestGitCannotReachHostRepositories(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("host git not installed; the escape needs host git-upload-pack")
	}
	hostRepo := filepath.Join(t.TempDir(), "secret.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", hostRepo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	scripts := map[string]string{
		"clone path":       "git clone " + hostRepo + " /c",
		"clone file url":   "git clone file://" + hostRepo + " /c",
		"ls-remote path":   "git ls-remote " + hostRepo,
		"ls-remote file":   "git ls-remote file://" + hostRepo,
		"fetch via remote": "mkdir /r && cd /r && git init -q && git remote add origin file://" + hostRepo + " && git fetch origin",
		"fetch via insteadOf": "mkdir /r && cd /r && git init -q && git remote add origin https://example.invalid/x.git && " +
			`printf '[url "file://` + hostRepo + `"]\n\tinsteadOf = https://example.invalid/x.git\n' >> /r/.git/config && git fetch origin`,
	}
	for name, script := range scripts {
		t.Run(name, func(t *testing.T) {
			var buf strings.Builder
			s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
			err := s.Run(context.Background(), script)
			out := buf.String()
			if err != nil {
				out += err.Error()
			}
			if err == nil {
				t.Fatalf("expected failure, got success: %q", out)
			}
			if strings.Contains(out, "upload-pack") || strings.Contains(out, "Cloning into") && !strings.Contains(out, "not supported") {
				t.Fatalf("host transport reached: %q", out)
			}
		})
	}

	if entries, _ := os.ReadDir(hostRepo); len(entries) == 0 {
		t.Fatal("host repo unexpectedly empty")
	}
}
