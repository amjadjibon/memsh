package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/shell"
)

func TestAlias(t *testing.T) {
	ctx := context.Background()

	t.Run("define and use alias", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "alias hi='echo hello' && hi"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "hello") {
			t.Errorf("expected 'hello', got: %q", buf.String())
		}
	})

	t.Run("alias with extra args", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "alias greet='echo hi' && greet world"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "hi world") {
			t.Errorf("expected 'hi world', got: %q", buf.String())
		}
	})

	t.Run("list all aliases", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "alias ll='ls -la' && alias"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "alias ll=") {
			t.Errorf("expected alias ll= in output, got: %q", buf.String())
		}
	})

	t.Run("print single alias", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "alias ll='ls -la' && alias ll"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "ll") {
			t.Errorf("expected 'll' in output, got: %q", buf.String())
		}
	})

	t.Run("unalias removes alias", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		err := s.Run(ctx, "alias hi='echo hello' && unalias hi && hi")
		if err == nil {
			t.Fatal("expected error after unalias, got nil")
		}
	})

	t.Run("unalias multiple names", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "alias a='echo a' && alias b='echo b' && unalias a b && alias"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(buf.String(), "alias a=") || strings.Contains(buf.String(), "alias b=") {
			t.Errorf("expected empty alias list after unalias a b, got: %q", buf.String())
		}
	})

	t.Run("self-referential alias does not loop", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		// alias ls='ls -la' — ls expands to ls -la, which calls builtinLs (not re-expanded)
		fs := afero.NewMemMapFs()
		_ = afero.WriteFile(fs, "/test.txt", []byte("data"), 0644)
		var buf2 strings.Builder
		s2 := NewTestShell(t, &buf2, shell.WithFS(fs))
		if err := s2.Run(ctx, "alias ls='ls -la' && ls /"); err != nil {
			t.Fatalf("self-referential alias caused error: %v", err)
		}
		if !strings.Contains(buf2.String(), "test.txt") {
			t.Errorf("expected ls output, got: %q", buf2.String())
		}
		_ = s
	})

	t.Run("WithAliases pre-seeds alias table", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithAliases(map[string]string{"greet": "echo seeded"}),
		)
		if err := s.Run(ctx, "greet"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "seeded") {
			t.Errorf("expected 'seeded', got: %q", buf.String())
		}
	})

	t.Run("alias in pipeline", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		_ = afero.WriteFile(fs, "/data.txt", []byte("apple\nbanana\napricot\n"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "alias g='grep a' && g /data.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "apple") {
			t.Errorf("expected 'apple' in output, got: %q", buf.String())
		}
	})

	t.Run("memshrc loaded from virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		_ = afero.WriteFile(fs, "/.memshrc", []byte("alias hi='echo from-rc'\n"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.LoadMemshrc(ctx); err != nil {
			t.Fatalf("LoadMemshrc: %v", err)
		}
		if err := s.Run(ctx, "hi"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "from-rc") {
			t.Errorf("expected 'from-rc', got: %q", buf.String())
		}
	})

	t.Run("type shows alias", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "alias ll='ls -la' && type ll"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "aliased") {
			t.Errorf("expected 'aliased' in type output, got: %q", buf.String())
		}
	})
}
