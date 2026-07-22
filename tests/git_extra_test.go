package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestGitCheckout(t *testing.T) {
	ctx := context.Background()

	t.Run("checkout -b creates and switches branch", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		if err := fs.MkdirAll("/repo", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := afero.WriteFile(fs, "/repo/a.txt", []byte("a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		s := gitShell(t, &buf, fs)
		run(t, s, ctx, `git init /repo`)
		run(t, s, ctx, `git -C /repo add a.txt`)
		run(t, s, ctx, `git -C /repo commit -m "init"`)

		buf.Reset()
		if err := s.Run(ctx, `git -C /repo checkout -b feature`); err != nil {
			t.Fatalf("checkout -b: %v", err)
		}
		if out := buf.String(); !strings.Contains(out, "Switched to a new branch 'feature'") {
			t.Errorf("checkout -b output: %q", out)
		}
	})

	t.Run("checkout switches to an existing branch", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		if err := fs.MkdirAll("/repo", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := afero.WriteFile(fs, "/repo/a.txt", []byte("a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		s := gitShell(t, &buf, fs)
		run(t, s, ctx, `git init /repo`)
		run(t, s, ctx, `git -C /repo add a.txt`)
		run(t, s, ctx, `git -C /repo commit -m "init"`)
		run(t, s, ctx, `git -C /repo branch feature`)

		buf.Reset()
		if err := s.Run(ctx, `git -C /repo checkout feature`); err != nil {
			t.Fatalf("checkout feature: %v", err)
		}
		if out := buf.String(); !strings.Contains(out, "Switched to branch 'feature'") {
			t.Errorf("checkout output: %q", out)
		}
	})

	t.Run("checkout restores a file from HEAD", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		if err := fs.MkdirAll("/repo", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := afero.WriteFile(fs, "/repo/a.txt", []byte("original\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		s := gitShell(t, &buf, fs)
		run(t, s, ctx, `git init /repo`)
		run(t, s, ctx, `git -C /repo add a.txt`)
		run(t, s, ctx, `git -C /repo commit -m "init"`)

		if err := afero.WriteFile(fs, "/repo/a.txt", []byte("modified\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := s.Run(ctx, `git -C /repo checkout a.txt`); err != nil {
			t.Fatalf("checkout a.txt: %v", err)
		}
		data, err := afero.ReadFile(fs, "/repo/a.txt")
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "original\n" {
			t.Errorf("a.txt = %q, want original content restored", data)
		}
	})
}

func TestGitReset(t *testing.T) {
	ctx := context.Background()

	t.Run("reset --hard reverts working tree to target commit", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		if err := fs.MkdirAll("/repo", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		s := gitShell(t, &buf, fs)
		run(t, s, ctx, `git init /repo`)
		run(t, s, ctx, `git -C /repo add a.txt`)
		run(t, s, ctx, `git -C /repo commit -m "v1"`)

		if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, s, ctx, `git -C /repo add a.txt`)
		run(t, s, ctx, `git -C /repo commit -m "v2"`)

		buf.Reset()
		if err := s.Run(ctx, `git -C /repo reset --hard HEAD~1`); err != nil {
			t.Fatalf("reset --hard: %v", err)
		}
		if out := buf.String(); !strings.Contains(out, "HEAD is now at") {
			t.Errorf("reset output: %q", out)
		}
		data, err := afero.ReadFile(fs, "/repo/a.txt")
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "v1\n" {
			t.Errorf("a.txt = %q, want v1 after hard reset", data)
		}
	})

	t.Run("reset with no commits errors", func(t *testing.T) {
		var buf strings.Builder
		fs := afero.NewMemMapFs()
		s := gitShell(t, &buf, fs)
		run(t, s, ctx, `git init /repo`)

		if err := s.Run(ctx, `git -C /repo reset`); err == nil {
			t.Error("expected error resetting with no commits")
		}
	})
}

func TestGitRm(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init"`)

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo rm a.txt`); err != nil {
		t.Fatalf("git rm: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "rm 'a.txt'") {
		t.Errorf("git rm output: %q", out)
	}
	if ok, _ := afero.Exists(fs, "/repo/a.txt"); ok {
		t.Error("a.txt should be removed from worktree")
	}
}

func TestGitStash(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init"`)

	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo stash push`); err != nil {
		t.Fatalf("stash push: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "Saved working directory") {
		t.Errorf("stash push output: %q", out)
	}
	data, _ := afero.ReadFile(fs, "/repo/a.txt")
	if string(data) != "v1\n" {
		t.Errorf("worktree should be clean after stash: %q", data)
	}

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo stash list`); err != nil {
		t.Fatalf("stash list: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "stash@{0}") {
		t.Errorf("stash list output: %q", out)
	}

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo stash apply`); err != nil {
		t.Fatalf("stash apply: %v", err)
	}
	data, _ = afero.ReadFile(fs, "/repo/a.txt")
	if string(data) != "dirty\n" {
		t.Errorf("a.txt after stash apply = %q, want dirty", data)
	}

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo stash drop`); err != nil {
		t.Fatalf("stash drop: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "Dropped stash@{0}") {
		t.Errorf("stash drop output: %q", out)
	}

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo stash list`); err != nil {
		t.Fatalf("stash list after drop: %v", err)
	}
	if out := strings.TrimSpace(buf.String()); out != "" {
		t.Errorf("stash list after drop should be empty, got: %q", out)
	}
}

func TestGitTag(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init"`)

	if err := s.Run(ctx, `git -C /repo tag v1.0`); err != nil {
		t.Fatalf("tag v1.0: %v", err)
	}
	if err := s.Run(ctx, `git -C /repo tag -a v2.0 -m "release 2"`); err != nil {
		t.Fatalf("tag -a v2.0: %v", err)
	}

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo tag`); err != nil {
		t.Fatalf("tag list: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "v1.0") || !strings.Contains(out, "v2.0") {
		t.Errorf("tag list = %q, want both tags", out)
	}

	if err := s.Run(ctx, `git -C /repo tag -d v1.0`); err != nil {
		t.Fatalf("tag -d v1.0: %v", err)
	}
	buf.Reset()
	run(t, s, ctx, `git -C /repo tag`)
	if strings.Contains(buf.String(), "v1.0") {
		t.Errorf("v1.0 should be deleted, tag list: %q", buf.String())
	}
}

func TestGitRemote(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)

	if err := s.Run(ctx, `git -C /repo remote add origin https://example.com/repo.git`); err != nil {
		t.Fatalf("remote add: %v", err)
	}

	buf.Reset()
	run(t, s, ctx, `git -C /repo remote`)
	if out := strings.TrimSpace(buf.String()); out != "origin" {
		t.Errorf("remote list = %q, want origin", out)
	}

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo remote -v`); err != nil {
		t.Fatalf("remote -v: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "https://example.com/repo.git") {
		t.Errorf("remote -v = %q", out)
	}

	if err := s.Run(ctx, `git -C /repo remote set-url origin https://example.com/new.git`); err != nil {
		t.Fatalf("remote set-url: %v", err)
	}
	buf.Reset()
	run(t, s, ctx, `git -C /repo remote get-url origin`)
	if out := strings.TrimSpace(buf.String()); out != "https://example.com/new.git" {
		t.Errorf("remote get-url = %q", out)
	}

	if err := s.Run(ctx, `git -C /repo remote rename origin upstream`); err != nil {
		t.Fatalf("remote rename: %v", err)
	}
	buf.Reset()
	run(t, s, ctx, `git -C /repo remote`)
	if out := strings.TrimSpace(buf.String()); out != "upstream" {
		t.Errorf("remote list after rename = %q", out)
	}

	if err := s.Run(ctx, `git -C /repo remote remove upstream`); err != nil {
		t.Fatalf("remote remove: %v", err)
	}
	buf.Reset()
	run(t, s, ctx, `git -C /repo remote`)
	if out := strings.TrimSpace(buf.String()); out != "" {
		t.Errorf("remote list after remove should be empty, got: %q", out)
	}
}

func TestGitMergeFastForward(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init"`)
	run(t, s, ctx, `git -C /repo branch feature`)
	run(t, s, ctx, `git -C /repo checkout feature`)

	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "feature work"`)

	run(t, s, ctx, `git -C /repo checkout master`)

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo merge feature`); err != nil {
		t.Fatalf("merge feature: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "Fast-forward") {
		t.Errorf("merge output: %q", out)
	}
	data, err := afero.ReadFile(fs, "/repo/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "v2\n" {
		t.Errorf("a.txt after merge = %q, want v2", data)
	}
}

func TestGitCherryPick(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init"`)
	run(t, s, ctx, `git -C /repo branch feature`)
	run(t, s, ctx, `git -C /repo checkout feature`)

	if err := afero.WriteFile(fs, "/repo/b.txt", []byte("feature file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, s, ctx, `git -C /repo add b.txt`)
	run(t, s, ctx, `git -C /repo commit -m "add b"`)

	buf.Reset()
	run(t, s, ctx, `git -C /repo log --oneline`)
	firstLineOut := strings.TrimSpace(strings.SplitN(buf.String(), "\n", 2)[0])
	featureCommitHash := strings.SplitN(firstLineOut, " ", 2)[0]

	run(t, s, ctx, `git -C /repo checkout master`)

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo cherry-pick `+featureCommitHash); err != nil {
		t.Fatalf("cherry-pick: %v", err)
	}
	if ok, _ := afero.Exists(fs, "/repo/b.txt"); !ok {
		t.Error("b.txt should exist on main after cherry-pick")
	}
}

func TestGitRevert(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init"`)

	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "v2"`)

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo revert HEAD`); err != nil {
		t.Fatalf("revert: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "Revert") {
		t.Errorf("revert output: %q", out)
	}
	data, err := afero.ReadFile(fs, "/repo/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "v1\n" {
		t.Errorf("a.txt after revert = %q, want v1", data)
	}
}

func TestGitFormatPatch(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init commit"`)

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo format-patch -1`); err != nil {
		t.Fatalf("format-patch: %v", err)
	}
	filename := strings.TrimSpace(buf.String())
	if !strings.HasSuffix(filename, ".patch") {
		t.Fatalf("format-patch output = %q, want a .patch filename", filename)
	}
	data, err := afero.ReadFile(fs, "/repo/"+filename)
	if err != nil {
		t.Fatalf("reading patch file: %v", err)
	}
	if !strings.Contains(string(data), "Subject: [PATCH 1/1] init commit") {
		t.Errorf("patch content missing subject line: %q", data)
	}
}

func TestGitApply(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init"`)

	patch := "--- a/a.txt\n" +
		"+++ b/a.txt\n" +
		"@@ -1,2 +1,3 @@\n" +
		" line1\n" +
		"+line1.5\n" +
		" line2\n"
	if err := afero.WriteFile(fs, "/repo/change.patch", []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.Run(ctx, `git -C /repo apply change.patch`); err != nil {
		t.Fatalf("apply: %v", err)
	}
	data, err := afero.ReadFile(fs, "/repo/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "line1\nline1.5\nline2\n" {
		t.Errorf("a.txt after apply = %q", data)
	}
}

func TestGitCatFile(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("blob content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init"`)

	buf.Reset()
	run(t, s, ctx, `git -C /repo log --oneline`)
	commitHash := strings.SplitN(strings.TrimSpace(buf.String()), " ", 2)[0]

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo cat-file -t `+commitHash); err != nil {
		t.Fatalf("cat-file -t: %v", err)
	}
	if out := strings.TrimSpace(buf.String()); out != "commit" {
		t.Errorf("cat-file -t = %q, want commit", out)
	}

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo cat-file -p `+commitHash); err != nil {
		t.Fatalf("cat-file -p: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "tree ") || !strings.Contains(out, "author ") {
		t.Errorf("cat-file -p output missing headers: %q", out)
	}
}

func TestGitHashObject(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/data.txt", []byte("hash me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo hash-object data.txt`); err != nil {
		t.Fatalf("hash-object: %v", err)
	}
	hash := strings.TrimSpace(buf.String())
	if len(hash) != 40 {
		t.Fatalf("hash-object output = %q, want a 40-char SHA-1", hash)
	}

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo hash-object -w data.txt`); err != nil {
		t.Fatalf("hash-object -w: %v", err)
	}
	hash2 := strings.TrimSpace(buf.String())
	if hash2 != hash {
		t.Errorf("hash-object -w hash = %q, want %q", hash2, hash)
	}

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo cat-file -p `+hash2); err != nil {
		t.Fatalf("cat-file -p on written object: %v", err)
	}
	if out := buf.String(); out != "hash me\n" {
		t.Errorf("cat-file -p on hash-object output = %q", out)
	}
}

func TestGitBlame(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("first line\nsecond line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init"`)

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo blame a.txt`); err != nil {
		t.Fatalf("blame: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "first line") || !strings.Contains(out, "second line") {
		t.Errorf("blame output missing lines: %q", out)
	}
}

func TestGitLsFiles(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/tracked.txt", []byte("tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add tracked.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init"`)

	if err := afero.WriteFile(fs, "/repo/untracked.txt", []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo ls-files`); err != nil {
		t.Fatalf("ls-files: %v", err)
	}
	if out := strings.TrimSpace(buf.String()); out != "tracked.txt" {
		t.Errorf("ls-files = %q, want tracked.txt", out)
	}

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo ls-files -o`); err != nil {
		t.Fatalf("ls-files -o: %v", err)
	}
	if out := strings.TrimSpace(buf.String()); out != "untracked.txt" {
		t.Errorf("ls-files -o = %q, want untracked.txt", out)
	}
}

func TestGitShortlog(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "first"`)

	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "second"`)

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo shortlog -s`); err != nil {
		t.Fatalf("shortlog -s: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if !strings.Contains(out, "2") {
		t.Errorf("shortlog -s = %q, want a count of 2", out)
	}
}

func TestGitDescribe(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init"`)
	run(t, s, ctx, `git -C /repo tag v1.0`)

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo describe`); err != nil {
		t.Fatalf("describe on tagged commit: %v", err)
	}
	if out := strings.TrimSpace(buf.String()); out != "v1.0" {
		t.Errorf("describe = %q, want v1.0", out)
	}

	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "v2"`)

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo describe`); err != nil {
		t.Fatalf("describe after new commit: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(out, "v1.0-1-g") {
		t.Errorf("describe = %q, want v1.0-1-g<hash>", out)
	}
}

func TestGitSwitch(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init"`)

	buf.Reset()
	if err := s.Run(ctx, `git -C /repo switch -c feature`); err != nil {
		t.Fatalf("switch -c: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "Switched to a new branch 'feature'") {
		t.Errorf("switch -c output: %q", out)
	}

	run(t, s, ctx, `git -C /repo switch master`)
	buf.Reset()
	if err := s.Run(ctx, `git -C /repo switch feature`); err != nil {
		t.Fatalf("switch feature: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "Switched to branch 'feature'") {
		t.Errorf("switch output: %q", out)
	}
}

func TestGitRestore(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := gitShell(t, &buf, fs)
	run(t, s, ctx, `git init /repo`)
	run(t, s, ctx, `git -C /repo add a.txt`)
	run(t, s, ctx, `git -C /repo commit -m "init"`)

	if err := afero.WriteFile(fs, "/repo/a.txt", []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Run(ctx, `git -C /repo restore a.txt`); err != nil {
		t.Fatalf("restore: %v", err)
	}
	data, err := afero.ReadFile(fs, "/repo/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original\n" {
		t.Errorf("a.txt after restore = %q, want original", data)
	}
}
