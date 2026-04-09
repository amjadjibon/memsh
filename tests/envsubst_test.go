package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestEnvsubst(t *testing.T) {
	ctx := context.Background()

	run := func(t *testing.T, script string, opts ...shell.Option) string {
		t.Helper()
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		opts = append([]shell.Option{shell.WithFS(fs)}, opts...)
		s := NewTestShell(t, &buf, opts...)
		if err := s.Run(ctx, script); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return buf.String()
	}

	t.Run("substitutes $VAR from env", func(t *testing.T) {
		out := run(t, `echo "hello $USER" | envsubst`,
			shell.WithEnv(map[string]string{"USER": "alice"}))
		if strings.TrimSpace(out) != "hello alice" {
			t.Errorf("got %q, want %q", out, "hello alice")
		}
	})

	t.Run("substitutes ${VAR} braced form", func(t *testing.T) {
		// single-quoted so the shell doesn't expand ${MYDIR} before envsubst sees it
		out := run(t, `echo 'dir=${MYDIR}/bin' | envsubst`,
			shell.WithEnv(map[string]string{"MYDIR": "/home/alice"}))
		if strings.TrimSpace(out) != "dir=/home/alice/bin" {
			t.Errorf("got %q", out)
		}
	})

	t.Run("unknown variable becomes empty string", func(t *testing.T) {
		out := run(t, `echo "x=${UNSET_VAR}y" | envsubst`)
		if strings.TrimSpace(out) != "x=y" {
			t.Errorf("got %q, want %q", out, "x=y")
		}
	})

	t.Run("allow-list substitutes only listed vars", func(t *testing.T) {
		// single-quoted template so the shell doesn't expand $FOO/$BAR early
		out := run(t, `echo '$FOO and $BAR' | envsubst '$FOO'`,
			shell.WithEnv(map[string]string{"FOO": "foo", "BAR": "bar"}))
		got := strings.TrimSpace(out)
		if !strings.Contains(got, "foo") {
			t.Errorf("expected FOO substituted: %q", got)
		}
		if !strings.Contains(got, "$BAR") {
			t.Errorf("expected BAR left unchanged: %q", got)
		}
	})

	t.Run("allow-list with multiple vars", func(t *testing.T) {
		out := run(t, `echo '$A $B $C' | envsubst '$A $B'`,
			shell.WithEnv(map[string]string{"A": "one", "B": "two", "C": "three"}))
		got := strings.TrimSpace(out)
		if got != "one two $C" {
			t.Errorf("got %q, want %q", got, "one two $C")
		}
	})

	t.Run("reads from virtual FS file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/tmpl.txt", []byte("Hello, $NAME!\n"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs), shell.WithEnv(map[string]string{"NAME": "World"}))
		if err := s.Run(ctx, "envsubst /tmpl.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "Hello, World!" {
			t.Errorf("got %q", buf.String())
		}
	})

	t.Run("multi-variable template file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/config.tmpl", []byte("host=${HOST}\nport=${PORT}\n"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs),
			shell.WithEnv(map[string]string{"HOST": "localhost", "PORT": "8080"}))
		if err := s.Run(ctx, "envsubst /config.tmpl"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "host=localhost") || !strings.Contains(out, "port=8080") {
			t.Errorf("got %q", out)
		}
	})

	t.Run("pipe output to file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/in.txt", []byte("$GREETING world"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs),
			shell.WithEnv(map[string]string{"GREETING": "hello"}))
		if err := s.Run(ctx, "envsubst /in.txt > /out.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, _ := afero.ReadFile(fs, "/out.txt")
		if strings.TrimSpace(string(data)) != "hello world" {
			t.Errorf("got %q", data)
		}
	})

	t.Run("non-variable dollar signs are preserved", func(t *testing.T) {
		// single-quoted so the shell passes $1 literally to envsubst
		out := run(t, `echo 'price: $1.99 tax' | envsubst`)
		// $1 starts with a digit — not a valid env var name, left unchanged
		got := strings.TrimSpace(out)
		if !strings.Contains(got, "$1.99") {
			t.Errorf("dollar-digit should be preserved: %q", got)
		}
	})

	t.Run("multiple files processed in order", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/a.txt", []byte("$X"), 0o644)
		afero.WriteFile(fs, "/b.txt", []byte("$Y"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs),
			shell.WithEnv(map[string]string{"X": "hello", "Y": "world"}))
		if err := s.Run(ctx, "envsubst /a.txt /b.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if out != "helloworld" {
			t.Errorf("got %q, want %q", out, "helloworld")
		}
	})
}
