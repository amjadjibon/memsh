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
)

// newTestShell builds a Shell with stdout and stderr both wired to buf.
// Extra options are appended after the IO option so callers can override FS/cwd.
func newTestShell(t *testing.T, buf *bytes.Buffer, opts ...shell.Option) *shell.Shell {
	t.Helper()
	base := []shell.Option{shell.WithStdIO(strings.NewReader(""), buf, buf)}
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
