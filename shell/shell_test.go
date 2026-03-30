package shell_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/shell"
	"github.com/amjadjibon/memsh/shell/plugins/native"
)

// newTestShell builds a Shell with stdout and stderr both wired to buf.
// Extra options are appended after the IO option so callers can override FS/cwd.
func newTestShell(t *testing.T, buf *bytes.Buffer, opts ...shell.Option) *shell.Shell {
	t.Helper()
	base := []shell.Option{
		shell.WithStdIO(strings.NewReader(""), buf, buf),
		shell.WithWASMEnabled(false), // no WASM needed; skips wazero init for speed
	}
	s, err := shell.New(append(base, opts...)...)
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	return s
}

// realTmpDir returns a temporary OS directory and registers cleanup.
// It is needed because interp.Dir validates paths against the real OS.
func realTmpDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "shelltest-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// ──────────────────────────────────────────────
// 1. Built-in commands
// ──────────────────────────────────────────────

func TestBuiltins(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		cwdOpt     string         // if non-empty, pass WithCwd (must be a real OS path)
		setup      func(fs afero.Fs) // optional FS pre-seeding
		script     string
		wantOut    string // substring that must appear in output
		wantErrStr string // non-empty: Run must return error containing this substring
	}{
		// ── echo ──────────────────────────────────────────
		{
			name:    "echo single word",
			script:  "echo hello",
			wantOut: "hello\n",
		},
		{
			name:    "echo multiple words",
			script:  "echo foo bar baz",
			wantOut: "foo bar baz\n",
		},
		{
			name:    "echo empty produces newline",
			script:  "echo",
			wantOut: "\n",
		},
		// ── mkdir ─────────────────────────────────────────
		{
			name:   "mkdir creates directory silently",
			script: "mkdir /testdir",
		},
		{
			name:    "mkdir then ls shows created dir",
			script:  "mkdir /mydir && ls /",
			wantOut: "mydir",
		},
		// ── ls ────────────────────────────────────────────
		{
			name: "ls pre-seeded directory lists files",
			setup: func(fs afero.Fs) {
				fs.MkdirAll("/seeddir", 0755)
				afero.WriteFile(fs, "/seeddir/alpha.txt", []byte("a"), 0644)
				afero.WriteFile(fs, "/seeddir/beta.txt", []byte("b"), 0644)
			},
			script:  "ls /seeddir",
			wantOut: "alpha.txt",
		},
		{
			name: "ls single file prints filename",
			setup: func(fs afero.Fs) {
				afero.WriteFile(fs, "/solo.txt", []byte("x"), 0644)
			},
			script:  "ls /solo.txt",
			wantOut: "solo.txt",
		},
		// ── cat ───────────────────────────────────────────
		{
			name: "cat prints pre-seeded file content",
			setup: func(fs afero.Fs) {
				afero.WriteFile(fs, "/hello.txt", []byte("hello world"), 0644)
			},
			script:  "cat /hello.txt",
			wantOut: "hello world",
		},
		{
			name:       "cat missing file returns error",
			script:     "cat /does_not_exist.txt",
			wantErrStr: "No such file or directory",
		},
		{
			name:       "cat with no arguments returns error",
			script:     "cat",
			wantErrStr: "missing operand",
		},
		// ── touch ─────────────────────────────────────────
		{
			name:   "touch creates a new file",
			script: "touch /newfile.txt",
		},
		{
			name: "touch on existing file does not error",
			setup: func(fs afero.Fs) {
				afero.WriteFile(fs, "/existing.txt", []byte("data"), 0644)
			},
			script: "touch /existing.txt",
		},
		// ── rm ────────────────────────────────────────────
		{
			name: "rm removes a file",
			setup: func(fs afero.Fs) {
				afero.WriteFile(fs, "/todelete.txt", []byte("bye"), 0644)
			},
			script: "rm /todelete.txt",
		},
		{
			name:       "rm with no arguments returns error",
			script:     "rm",
			wantErrStr: "missing operand",
		},
		// ── pwd ───────────────────────────────────────────
		{
			name:    "pwd prints root when cwd is /",
			script:  "pwd",
			wantOut: "/\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			if tc.setup != nil {
				tc.setup(fs)
			}

			var buf bytes.Buffer
			opts := []shell.Option{shell.WithFS(fs)}

			s := newTestShell(t, &buf, opts...)
			err := s.Run(ctx, tc.script)

			if tc.wantErrStr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q; got nil", tc.wantErrStr)
				}
				if !strings.Contains(err.Error(), tc.wantErrStr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErrStr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tc.wantOut != "" && !strings.Contains(buf.String(), tc.wantOut) {
				t.Errorf("output %q does not contain %q", buf.String(), tc.wantOut)
			}
		})
	}
}

// ──────────────────────────────────────────────
// 2. Redirects
// ──────────────────────────────────────────────

func TestRedirects(t *testing.T) {
	ctx := context.Background()

	t.Run("overwrite redirect creates file with content", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "echo foo > /f"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := afero.ReadFile(fs, "/f")
		if err != nil {
			t.Fatalf("ReadFile /f: %v", err)
		}
		if !strings.Contains(string(got), "foo") {
			t.Errorf("file content %q does not contain 'foo'", string(got))
		}
	})

	t.Run("append redirect appends to existing file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f", []byte("foo\n"), 0644)

		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "echo bar >> /f"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := afero.ReadFile(fs, "/f")
		if err != nil {
			t.Fatalf("ReadFile /f: %v", err)
		}
		content := string(got)
		if !strings.Contains(content, "foo") {
			t.Errorf("file content %q should still contain 'foo'", content)
		}
		if !strings.Contains(content, "bar") {
			t.Errorf("file content %q does not contain appended 'bar'", content)
		}
	})

	t.Run("overwrite redirect truncates existing content", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f", []byte("old content\n"), 0644)

		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "echo new > /f"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := afero.ReadFile(fs, "/f")
		if err != nil {
			t.Fatalf("ReadFile /f: %v", err)
		}
		content := string(got)
		if strings.Contains(content, "old content") {
			t.Errorf("file content %q still contains 'old content' after overwrite", content)
		}
		if !strings.Contains(content, "new") {
			t.Errorf("file content %q should contain 'new'", content)
		}
	})

	t.Run("redirect then cat reads back written content", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "echo roundtrip > /rt.txt && cat /rt.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "roundtrip") {
			t.Errorf("output %q does not contain 'roundtrip'", buf.String())
		}
	})
}

// ──────────────────────────────────────────────
// 3. Pipes
//
// Note: the shell's cat builtin requires file arguments — it does not read
// from stdin. Pipes are tested using echo on both sides of the pipe and by
// verifying the pipe mechanism routes output through the connected command.
// ──────────────────────────────────────────────

func TestPipes(t *testing.T) {
	ctx := context.Background()

	t.Run("pipe output reaches right-hand side command", func(t *testing.T) {
		// echo ignores stdin and just prints its own args, but the pipe must
		// not cause an error and the final output must contain the RHS output.
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		if err := s.Run(ctx, "echo left | echo right"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "right") {
			t.Errorf("output %q does not contain 'right'", buf.String())
		}
	})

	t.Run("multiple pipe stages execute without error", func(t *testing.T) {
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		// Three-stage pipe: each echo emits its own args.
		if err := s.Run(ctx, "echo a | echo b | echo c"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "c") {
			t.Errorf("output %q does not contain 'c'", buf.String())
		}
	})

	t.Run("pipe redirected to file captures output", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		// Right-hand echo writes to stdout which is captured by redirect.
		if err := s.Run(ctx, "echo ignored | echo captured > /out.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := afero.ReadFile(fs, "/out.txt")
		if err != nil {
			t.Fatalf("ReadFile /out.txt: %v", err)
		}
		if !strings.Contains(string(got), "captured") {
			t.Errorf("file %q does not contain 'captured'", string(got))
		}
	})
}

// ──────────────────────────────────────────────
// 4. ErrExit / quit
//
// Note: the shell keyword "exit" is handled internally by mvdan.cc/sh/v3 and
// causes the runner to return nil (normal completion). Only the custom "quit"
// command reaches our execHandler and returns shell.ErrExit.
// ──────────────────────────────────────────────

func TestErrExit(t *testing.T) {
	ctx := context.Background()

	t.Run("quit returns ErrExit", func(t *testing.T) {
		var buf bytes.Buffer
		s := newTestShell(t, &buf)

		err := s.Run(ctx, "quit")
		if !errors.Is(err, shell.ErrExit) {
			t.Errorf("expected shell.ErrExit; got %v", err)
		}
	})

	t.Run("exit is handled by interpreter and returns nil", func(t *testing.T) {
		// mvdan.cc/sh intercepts the 'exit' keyword natively before our
		// execHandler and considers it a clean termination (nil error).
		var buf bytes.Buffer
		s := newTestShell(t, &buf)

		err := s.Run(ctx, "exit")
		if err != nil {
			t.Errorf("expected nil for 'exit'; got %v", err)
		}
	})

	t.Run("quit after other commands still returns ErrExit", func(t *testing.T) {
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		err := s.Run(ctx, "echo before; quit")
		if !errors.Is(err, shell.ErrExit) {
			t.Errorf("expected shell.ErrExit after echo+quit; got %v", err)
		}
		if !strings.Contains(buf.String(), "before") {
			t.Errorf("output %q does not contain 'before'", buf.String())
		}
	})
}

// ──────────────────────────────────────────────
// 5. Pre-seeded FS via WithFS + afero.WriteFile
// ──────────────────────────────────────────────

func TestWithFS(t *testing.T) {
	ctx := context.Background()

	t.Run("cat reads pre-seeded file content", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/data.txt", []byte("seeded content"), 0644)

		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "cat /data.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "seeded content") {
			t.Errorf("output %q does not contain 'seeded content'", buf.String())
		}
	})

	t.Run("ls shows pre-seeded files in directory", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/predir", 0755)
		afero.WriteFile(fs, "/predir/fileA", []byte(""), 0644)
		afero.WriteFile(fs, "/predir/fileB", []byte(""), 0644)

		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "ls /predir"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := buf.String()
		for _, want := range []string{"fileA", "fileB"} {
			if !strings.Contains(out, want) {
				t.Errorf("output %q does not contain %q", out, want)
			}
		}
	})

	t.Run("rm removes pre-seeded file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/gone.txt", []byte("bye"), 0644)

		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "rm /gone.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		exists, _ := afero.Exists(fs, "/gone.txt")
		if exists {
			t.Errorf("expected /gone.txt to be removed, but it still exists")
		}
	})

	t.Run("script writes new file visible in afero after Run", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "echo written > /out.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := afero.ReadFile(fs, "/out.txt")
		if err != nil {
			t.Fatalf("ReadFile /out.txt: %v", err)
		}
		if !strings.Contains(string(got), "written") {
			t.Errorf("file content %q does not contain 'written'", string(got))
		}
	})
}

// ──────────────────────────────────────────────
// 6. WithCwd sets initial directory
//
// interp.Dir validates the initial directory against the real OS filesystem.
// Tests therefore use os.MkdirTemp for the cwd value while still routing
// virtual file I/O through afero.
// ──────────────────────────────────────────────

func TestWithCwd(t *testing.T) {
	ctx := context.Background()

	t.Run("pwd reflects WithCwd using real temp dir", func(t *testing.T) {
		dir := realTmpDir(t)

		// Mirror the real dir path in the virtual FS so ls/touch work.
		fs := afero.NewMemMapFs()
		fs.MkdirAll(dir, 0755)

		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs), shell.WithCwd(dir))

		if err := s.Run(ctx, "pwd"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(buf.String(), dir) {
			t.Errorf("output %q does not contain %q", buf.String(), dir)
		}
	})

	t.Run("ls without path uses WithCwd directory", func(t *testing.T) {
		dir := realTmpDir(t)

		fs := afero.NewMemMapFs()
		fs.MkdirAll(dir, 0755)
		afero.WriteFile(fs, dir+"/myfile.txt", []byte("hi"), 0644)

		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs), shell.WithCwd(dir))

		if err := s.Run(ctx, "ls"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(buf.String(), "myfile.txt") {
			t.Errorf("output %q does not contain 'myfile.txt'", buf.String())
		}
	})

	t.Run("touch creates file relative to WithCwd", func(t *testing.T) {
		dir := realTmpDir(t)

		fs := afero.NewMemMapFs()
		fs.MkdirAll(dir, 0755)

		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs), shell.WithCwd(dir))

		if err := s.Run(ctx, "touch relative.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		exists, _ := afero.Exists(fs, dir+"/relative.txt")
		if !exists {
			t.Errorf("expected %s/relative.txt to exist after touch with relative path", dir)
		}
	})

	t.Run("cat with relative path resolves against WithCwd", func(t *testing.T) {
		dir := realTmpDir(t)

		fs := afero.NewMemMapFs()
		fs.MkdirAll(dir, 0755)
		afero.WriteFile(fs, dir+"/greet.txt", []byte("greetings"), 0644)

		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs), shell.WithCwd(dir))

		if err := s.Run(ctx, "cat greet.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "greetings") {
			t.Errorf("output %q does not contain 'greetings'", buf.String())
		}
	})
}

// ──────────────────────────────────────────────
// 7. Unknown command falls through (returns error, not panic)
// ──────────────────────────────────────────────

func TestUnknownCommand(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		script string
	}{
		{"completely unknown binary", "nonexistentbinary123"},
		{"unknown command with arguments", "fakecmd --flag value"},
		{"unknown command starting with underscore", "_notacommand"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			s := newTestShell(t, &buf)

			// Must return a non-nil error; must NOT panic.
			err := s.Run(ctx, tc.script)
			if err == nil {
				t.Errorf("expected an error for unknown command %q; got nil", tc.script)
			}
			// Must not be ErrExit.
			if errors.Is(err, shell.ErrExit) {
				t.Errorf("error for unknown command must not be ErrExit")
			}
		})
	}
}

// ──────────────────────────────────────────────
// 8. Multiple commands in one script
// ──────────────────────────────────────────────

func TestMultipleCommands(t *testing.T) {
	ctx := context.Background()

	t.Run("sequential newline-separated commands share FS state", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		script := "mkdir /multi\ntouch /multi/a.txt\ntouch /multi/b.txt\nls /multi"
		if err := s.Run(ctx, script); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := buf.String()
		for _, want := range []string{"a.txt", "b.txt"} {
			if !strings.Contains(out, want) {
				t.Errorf("output %q does not contain %q", out, want)
			}
		}
	})

	t.Run("semicolon-separated commands all execute", func(t *testing.T) {
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		if err := s.Run(ctx, "echo line1; echo line2; echo line3"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := buf.String()
		for _, want := range []string{"line1", "line2", "line3"} {
			if !strings.Contains(out, want) {
				t.Errorf("output %q does not contain %q", out, want)
			}
		}
	})

	t.Run("and-chained commands with shared FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "mkdir /ch && touch /ch/x.txt && ls /ch"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(buf.String(), "x.txt") {
			t.Errorf("output %q does not contain 'x.txt'", buf.String())
		}
	})

	t.Run("write then read within same Run call", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "echo roundtrip > /rt.txt && cat /rt.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "roundtrip") {
			t.Errorf("output %q does not contain 'roundtrip'", buf.String())
		}
	})

	t.Run("touch then rm removes the file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "touch /tmp_del.txt && rm /tmp_del.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		exists, _ := afero.Exists(fs, "/tmp_del.txt")
		if exists {
			t.Errorf("expected /tmp_del.txt to be removed, but it still exists")
		}
	})

	t.Run("mkdir nested and write file inside", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		script := "mkdir /nested\ntouch /nested/deep.txt"
		if err := s.Run(ctx, script); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		exists, _ := afero.Exists(fs, "/nested/deep.txt")
		if !exists {
			t.Errorf("expected /nested/deep.txt to exist")
		}
	})
}

// ──────────────────────────────────────────────
// 9. cp and mv
// ──────────────────────────────────────────────

func TestCpMv(t *testing.T) {
	ctx := context.Background()

	t.Run("cp copies file content", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/src.txt", []byte("hello"), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "cp /src.txt /dst.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := afero.ReadFile(fs, "/dst.txt")
		if err != nil {
			t.Fatalf("ReadFile /dst.txt: %v", err)
		}
		if string(got) != "hello" {
			t.Errorf("want 'hello', got %q", string(got))
		}
	})

	t.Run("cp into existing directory places file inside", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("data"), 0644)
		fs.MkdirAll("/dir", 0755)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "cp /f.txt /dir"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		exists, _ := afero.Exists(fs, "/dir/f.txt")
		if !exists {
			t.Error("expected /dir/f.txt to exist")
		}
	})

	t.Run("cp -r copies directory recursively", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/a", 0755)
		afero.WriteFile(fs, "/a/x.txt", []byte("x"), 0644)
		afero.WriteFile(fs, "/a/y.txt", []byte("y"), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "cp -r /a /b"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := afero.ReadFile(fs, "/b/x.txt")
		if err != nil {
			t.Fatalf("ReadFile /b/x.txt: %v", err)
		}
		if string(got) != "x" {
			t.Errorf("want 'x', got %q", string(got))
		}
	})

	t.Run("cp directory without -r returns error", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/mydir", 0755)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		err := s.Run(ctx, "cp /mydir /other")
		if err == nil || !strings.Contains(err.Error(), "-r not specified") {
			t.Errorf("expected -r error, got %v", err)
		}
	})

	t.Run("cp missing source returns error", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		err := s.Run(ctx, "cp /nosuchfile /dst")
		if err == nil || !strings.Contains(err.Error(), "No such file or directory") {
			t.Errorf("expected not-found error, got %v", err)
		}
	})

	t.Run("mv renames file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/old.txt", []byte("content"), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "mv /old.txt /new.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		exists, _ := afero.Exists(fs, "/old.txt")
		if exists {
			t.Error("expected /old.txt to be gone after mv")
		}
		got, err := afero.ReadFile(fs, "/new.txt")
		if err != nil {
			t.Fatalf("ReadFile /new.txt: %v", err)
		}
		if string(got) != "content" {
			t.Errorf("want 'content', got %q", string(got))
		}
	})

	t.Run("mv into existing directory moves file inside", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("hi"), 0644)
		fs.MkdirAll("/dir", 0755)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "mv /f.txt /dir"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		exists, _ := afero.Exists(fs, "/dir/f.txt")
		if !exists {
			t.Error("expected /dir/f.txt to exist after mv")
		}
	})
}

// ──────────────────────────────────────────────
// 10. head and tail
// ──────────────────────────────────────────────

func TestHeadTail(t *testing.T) {
	ctx := context.Background()
	content := "line1\nline2\nline3\nline4\nline5\n"

	t.Run("head prints first 10 lines by default", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte(content), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "head /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "line1") {
			t.Errorf("output %q does not contain 'line1'", buf.String())
		}
	})

	t.Run("head -n 2 prints first 2 lines", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte(content), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "head -n 2 /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") {
			t.Errorf("output %q missing line1 or line2", out)
		}
		if strings.Contains(out, "line3") {
			t.Errorf("output %q should not contain line3", out)
		}
	})

	t.Run("tail -n 2 prints last 2 lines", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte(content), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "tail -n 2 /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "line4") || !strings.Contains(out, "line5") {
			t.Errorf("output %q missing line4 or line5", out)
		}
		if strings.Contains(out, "line1") {
			t.Errorf("output %q should not contain line1", out)
		}
	})

	t.Run("tail prints last 10 lines by default", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte(content), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "tail /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "line5") {
			t.Errorf("output %q does not contain 'line5'", buf.String())
		}
	})

	t.Run("head missing file returns error", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		err := s.Run(ctx, "head /nosuchfile.txt")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})
}

// ──────────────────────────────────────────────
// 11. grep and find (native plugins)
// ──────────────────────────────────────────────

func TestGrep(t *testing.T) {
	ctx := context.Background()

	t.Run("grep matches lines in file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("apple\nbanana\napricot\n"), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "grep apple /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "apple") {
			t.Errorf("output %q does not contain 'apple'", out)
		}
		if strings.Contains(out, "banana") {
			t.Errorf("output %q should not contain 'banana'", out)
		}
	})

	t.Run("grep -i does case-insensitive match", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("Hello\nworld\n"), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "grep -i hello /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "Hello") {
			t.Errorf("output %q does not contain 'Hello'", buf.String())
		}
	})

	t.Run("grep -v inverts match", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("keep\nskip\nkeep2\n"), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "grep -v skip /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if strings.Contains(out, "skip") {
			t.Errorf("output %q should not contain 'skip'", out)
		}
		if !strings.Contains(out, "keep") {
			t.Errorf("output %q should contain 'keep'", out)
		}
	})

	t.Run("grep -n shows line numbers", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("foo\nbar\nfoo2\n"), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "grep -n foo /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "1:") {
			t.Errorf("output %q should contain line number '1:'", out)
		}
	})

	t.Run("grep no match returns non-nil error", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("hello\n"), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		err := s.Run(ctx, "grep nomatch /f.txt")
		if err == nil {
			t.Fatal("expected non-nil error for no matches (exit status 1)")
		}
	})

	t.Run("grep stdin via pipe", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, `echo "hello world" | grep hello`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "hello") {
			t.Errorf("output %q does not contain 'hello'", buf.String())
		}
	})
}

func TestFind(t *testing.T) {
	ctx := context.Background()

	t.Run("find lists all entries under path", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/d", 0755)
		afero.WriteFile(fs, "/d/a.txt", []byte(""), 0644)
		afero.WriteFile(fs, "/d/b.go", []byte(""), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "find /d"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "a.txt") {
			t.Errorf("output %q does not contain 'a.txt'", out)
		}
		if !strings.Contains(out, "b.go") {
			t.Errorf("output %q does not contain 'b.go'", out)
		}
	})

	t.Run("find -name filters by glob", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/d", 0755)
		afero.WriteFile(fs, "/d/a.txt", []byte(""), 0644)
		afero.WriteFile(fs, "/d/b.go", []byte(""), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "find /d -name *.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "a.txt") {
			t.Errorf("output %q does not contain 'a.txt'", out)
		}
		if strings.Contains(out, "b.go") {
			t.Errorf("output %q should not contain 'b.go'", out)
		}
	})

	t.Run("find -type f lists only files", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/d/sub", 0755)
		afero.WriteFile(fs, "/d/f.txt", []byte(""), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "find /d -type f"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "f.txt") {
			t.Errorf("output %q does not contain 'f.txt'", out)
		}
		if strings.Contains(out, "sub") {
			t.Errorf("output %q should not contain directory 'sub'", out)
		}
	})

	t.Run("find -type d lists only directories", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/d/sub", 0755)
		afero.WriteFile(fs, "/d/f.txt", []byte(""), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "find /d -type d"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if strings.Contains(out, "f.txt") {
			t.Errorf("output %q should not contain file 'f.txt'", out)
		}
		if !strings.Contains(out, "sub") {
			t.Errorf("output %q should contain directory 'sub'", out)
		}
	})
}

// ──────────────────────────────────────────────
// Plugin interface tests
// ──────────────────────────────────────────────

func TestPluginInterface(t *testing.T) {
	ctx := context.Background()

	t.Run("WithPlugin overrides default plugin", func(t *testing.T) {
		// Re-registering Base64Plugin via WithPlugin should still work.
		var buf bytes.Buffer
		s, err := shell.New(
			shell.WithStdIO(strings.NewReader(""), &buf, &buf),
			shell.WithWASMEnabled(false),
			shell.WithPlugin(native.Base64Plugin{}),
		)
		if err != nil {
			t.Fatalf("shell.New: %v", err)
		}
		if err := s.Run(ctx, "echo hello | base64"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "aGVsbG8K") {
			t.Errorf("expected base64 output, got %q", buf.String())
		}
	})

	t.Run("Register adds plugin after construction", func(t *testing.T) {
		var buf bytes.Buffer
		s, err := shell.New(
			shell.WithStdIO(strings.NewReader(""), &buf, &buf),
			shell.WithWASMEnabled(false),
		)
		if err != nil {
			t.Fatalf("shell.New: %v", err)
		}
		s.Register(native.Base64Plugin{}) // re-register to verify method works
		if err := s.Run(ctx, "echo test | base64"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if buf.Len() == 0 {
			t.Error("expected output from base64 plugin")
		}
	})

	t.Run("native base64 encode roundtrip", func(t *testing.T) {
		var buf bytes.Buffer
		s := newTestShell(t, &buf)
		if err := s.Run(ctx, "echo hello | base64"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := strings.TrimSpace(buf.String()); got != "aGVsbG8K" {
			t.Errorf("want aGVsbG8K, got %q", got)
		}
	})

	t.Run("native wc -l counts lines from virtual FS file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/lines.txt", []byte("a\nb\nc\n"), 0644)
		var buf bytes.Buffer
		s := newTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "wc -l /lines.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := strings.TrimSpace(buf.String()); got != "3" {
			t.Errorf("want 3, got %q", got)
		}
	})

	t.Run("native wc -w counts words from stdin pipe", func(t *testing.T) {
		var buf bytes.Buffer
		s := newTestShell(t, &buf)
		if err := s.Run(ctx, `echo "one two three" | wc -w`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := strings.TrimSpace(buf.String()); got != "3" {
			t.Errorf("want 3, got %q", got)
		}
	})

	t.Run("BuiltinPluginNames includes base64 and wc", func(t *testing.T) {
		names := shell.BuiltinPluginNames()
		nameSet := make(map[string]bool)
		for _, n := range names {
			nameSet[n] = true
		}
		for _, want := range []string{"base64", "wc"} {
			if !nameSet[want] {
				t.Errorf("BuiltinPluginNames: missing %q", want)
			}
		}
	})
}
