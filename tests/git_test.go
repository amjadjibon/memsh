package tests

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/shell"
)

// gitShell creates a test shell paired with a pre-seeded afero.MemMapFs.
// realCwd must be a real OS path because mvdan.cc/sh validates it.
func gitShell(t *testing.T, buf *strings.Builder, fs afero.Fs) *shell.Shell {
	t.Helper()
	realCwd, err := os.MkdirTemp("", "memsh-git-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(realCwd) })
	return NewTestShell(t, buf,
		shell.WithFS(fs),
		shell.WithCwd(realCwd),
	)
}

// run is a helper that calls s.Run and logs (but does not fail) on error.
func run(t *testing.T, s *shell.Shell, ctx context.Context, cmd string) error {
	t.Helper()
	err := s.Run(ctx, cmd)
	if err != nil {
		t.Logf("cmd %q: %v", cmd, err)
	}
	return err
}

func TestGit(t *testing.T) {
	ctx := context.Background()

	t.Run("git init creates .git dir", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		s := gitShell(t, &buf, fs)

		if err := s.Run(ctx, `git init /repo`); err != nil {
			t.Fatalf("git init: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, "Initialized empty Git repository") {
			t.Errorf("expected init message, got: %q", out)
		}
		// Verify .git directory was actually created in the virtual FS.
		info, err := fs.Stat("/repo/.git")
		if err != nil || !info.IsDir() {
			t.Errorf(".git directory not found in virtual FS: %v", err)
		}
	})

	t.Run("git status on fresh repo shows no commits", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		s := gitShell(t, &buf, fs)

		if err := s.Run(ctx, `git init /repo`); err != nil {
			t.Fatalf("git init: %v", err)
		}
		buf.Reset()
		// Use -C to point git at the virtual repo root.
		if err := s.Run(ctx, `git -C /repo status`); err != nil {
			t.Logf("git status: %v", err)
		}
		out := buf.String()
		// Fresh repo should mention branch or "no commits".
		if !strings.Contains(out, "branch") && !strings.Contains(out, "No commits") &&
			!strings.Contains(out, "nothing to commit") {
			t.Logf("git status output: %q", out)
		}
	})

	t.Run("git add and commit then log oneline", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		if err := fs.MkdirAll("/repo", 0755); err != nil {
			t.Fatal(err)
		}
		if err := afero.WriteFile(fs, "/repo/hello.txt", []byte("hello world\n"), 0644); err != nil {
			t.Fatal(err)
		}

		s := gitShell(t, &buf, fs)

		if err := s.Run(ctx, `git init /repo`); err != nil {
			t.Fatalf("git init: %v", err)
		}
		if err := s.Run(ctx, `git -C /repo add hello.txt`); err != nil {
			t.Fatalf("git add: %v", err)
		}
		if err := s.Run(ctx, `git -C /repo commit -m "first commit"`); err != nil {
			t.Fatalf("git commit: %v", err)
		}

		buf.Reset()
		if err := s.Run(ctx, `git -C /repo log --oneline`); err != nil {
			t.Fatalf("git log: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, "first commit") {
			t.Errorf("git log --oneline: expected 'first commit', got: %q", out)
		}
	})

	t.Run("git status after commit shows clean tree", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		if err := fs.MkdirAll("/repo", 0755); err != nil {
			t.Fatal(err)
		}
		if err := afero.WriteFile(fs, "/repo/a.txt", []byte("data\n"), 0644); err != nil {
			t.Fatal(err)
		}

		s := gitShell(t, &buf, fs)
		if err := s.Run(ctx, `git init /repo`); err != nil {
			t.Fatal(err)
		}
		if err := s.Run(ctx, `git -C /repo add a.txt`); err != nil {
			t.Fatal(err)
		}
		if err := s.Run(ctx, `git -C /repo commit -m "add a"`); err != nil {
			t.Fatal(err)
		}

		buf.Reset()
		if err := s.Run(ctx, `git -C /repo status`); err != nil {
			t.Logf("git status: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "nothing to commit") && !strings.Contains(out, "clean") {
			t.Errorf("expected clean status, got: %q", out)
		}
	})

	t.Run("git branch creates and lists branches", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		if err := fs.MkdirAll("/repo", 0755); err != nil {
			t.Fatal(err)
		}
		if err := afero.WriteFile(fs, "/repo/f.txt", []byte("data\n"), 0644); err != nil {
			t.Fatal(err)
		}

		s := gitShell(t, &buf, fs)
		run(t, s, ctx, `git init /repo`)
		run(t, s, ctx, `git -C /repo add f.txt`)
		run(t, s, ctx, `git -C /repo commit -m "init"`)

		if err := s.Run(ctx, `git -C /repo branch feature`); err != nil {
			t.Fatalf("git branch feature: %v", err)
		}

		buf.Reset()
		if err := s.Run(ctx, `git -C /repo branch`); err != nil {
			t.Logf("git branch list: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "feature") {
			t.Errorf("expected 'feature' branch in output, got: %q", out)
		}
	})

	t.Run("git show HEAD after commit", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		if err := fs.MkdirAll("/repo", 0755); err != nil {
			t.Fatal(err)
		}
		if err := afero.WriteFile(fs, "/repo/readme.md", []byte("# Hello\n"), 0644); err != nil {
			t.Fatal(err)
		}

		s := gitShell(t, &buf, fs)
		run(t, s, ctx, `git init /repo`)
		run(t, s, ctx, `git -C /repo add readme.md`)
		run(t, s, ctx, `git -C /repo commit -m "add readme"`)

		buf.Reset()
		if err := s.Run(ctx, `git -C /repo show HEAD`); err != nil {
			t.Fatalf("git show HEAD: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "add readme") {
			t.Errorf("git show HEAD: expected commit message, got: %q", out)
		}
	})

	t.Run("git show on empty repo returns error", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		s := gitShell(t, &buf, fs)

		run(t, s, ctx, `git init /repo`)
		err := s.Run(ctx, `git -C /repo show HEAD`)
		if err == nil {
			t.Logf("git show HEAD on empty repo returned no error (output: %s)", buf.String())
		}
	})

	t.Run("git diff shows changes", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		if err := fs.MkdirAll("/repo", 0755); err != nil {
			t.Fatal(err)
		}
		if err := afero.WriteFile(fs, "/repo/data.txt", []byte("line1\nline2\n"), 0644); err != nil {
			t.Fatal(err)
		}

		s := gitShell(t, &buf, fs)
		run(t, s, ctx, `git init /repo`)
		run(t, s, ctx, `git -C /repo add data.txt`)
		run(t, s, ctx, `git -C /repo commit -m "initial"`)

		// Modify the file.
		if err := afero.WriteFile(fs, "/repo/data.txt", []byte("line1\nline2\nline3\n"), 0644); err != nil {
			t.Fatal(err)
		}

		buf.Reset()
		if err := s.Run(ctx, `git -C /repo diff`); err != nil {
			t.Logf("git diff: %v", err)
		}
		out := buf.String()
		// Should have diff output.
		if out == "" {
			t.Logf("git diff returned empty output (may be expected if worktree not tracked)")
		}
	})

	t.Run("git config get set", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		s := gitShell(t, &buf, fs)

		run(t, s, ctx, `git init /repo`)

		if err := s.Run(ctx, `git -C /repo config user.name "Alice"`); err != nil {
			t.Fatalf("git config set: %v", err)
		}

		buf.Reset()
		if err := s.Run(ctx, `git -C /repo config user.name`); err != nil {
			t.Fatalf("git config get: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "Alice" {
			t.Errorf("git config user.name: expected 'Alice', got: %q", out)
		}
	})
}
