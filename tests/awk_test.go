package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/amjadjibon/memsh/shell"
)

func TestAwk(t *testing.T) {
	ctx := context.Background()

	t.Run("awk print second field from stdin", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo "hello world" | awk '{print $2}'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "world" {
			t.Errorf("expected 'world', got %q", buf.String())
		}
	})

	t.Run("awk processes virtual FS file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/data.txt", []byte("alice 30\nbob 25\n"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, `awk '{print $1}' /data.txt`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "alice") || !strings.Contains(out, "bob") {
			t.Errorf("unexpected awk output: %q", out)
		}
	})

	t.Run("awk -f reads program from virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/prog.awk", []byte("{print $2}"), 0644)
		afero.WriteFile(fs, "/data.txt", []byte("foo bar\nbaz qux\n"), 0644)

		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, `awk -f /prog.awk /data.txt`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "bar") || !strings.Contains(out, "qux") {
			t.Errorf("unexpected awk output: %q", out)
		}
	})

	t.Run("awk NR counts lines", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo -e "a\nb\nc" | awk '{print NR}'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		if len(out) != 3 || out[0] != "1" || out[1] != "2" || out[2] != "3" {
			t.Errorf("expected line count 1,2,3, got %q", out)
		}
	})
}
