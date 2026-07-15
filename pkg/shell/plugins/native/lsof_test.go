package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestLsof(t *testing.T) {
	newFS := func() afero.Fs {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/etc/hosts", []byte("127.0.0.1 localhost\n"), 0644)
		afero.WriteFile(fs, "/tmp/small.txt", []byte("hi"), 0644)
		afero.WriteFile(fs, "/tmp/big.txt", make([]byte, 2048), 0644)
		return fs
	}

	t.Run("lists regular files with header", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(newFS()))
		if err := s.Run(context.Background(), "lsof"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "/etc/hosts") {
			t.Errorf("expected /etc/hosts in output, got %q", out)
		}
		if !strings.Contains(out, "REG") {
			t.Errorf("expected REG type in output, got %q", out)
		}
	})

	t.Run("-d includes directories", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(newFS()))
		if err := s.Run(context.Background(), "lsof -d"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "DIR") {
			t.Errorf("expected DIR entries with -d, got %q", out)
		}
	})

	t.Run("path filter restricts to subtree", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(newFS()))
		if err := s.Run(context.Background(), "lsof /tmp"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "small.txt") {
			t.Errorf("expected small.txt, got %q", out)
		}
		if strings.Contains(out, "/etc/hosts") {
			t.Errorf("expected /etc/hosts to be excluded, got %q", out)
		}
	})

	t.Run("-s filters by minimum size", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(newFS()))
		if err := s.Run(context.Background(), "lsof -s 1K"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "big.txt") {
			t.Errorf("expected big.txt (2K) in output, got %q", out)
		}
		if strings.Contains(out, "small.txt") {
			t.Errorf("expected small.txt to be filtered out, got %q", out)
		}
	})

	t.Run("count line appears in output", func(t *testing.T) {
		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(newFS()))
		if err := s.Run(context.Background(), "lsof"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "file(s)") {
			t.Errorf("expected count summary, got %q", buf.String())
		}
	})
}
