package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/amjadjibon/memsh/shell"
)

func TestBase64(t *testing.T) {
	ctx := context.Background()

	t.Run("base64 encode from stdin", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo "hello" | base64`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "aGVsbG8K" {
			t.Errorf("expected 'aGVsbG8K', got %q", out)
		}
	})

	t.Run("base64 decode with -d", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo "aGVsbG8K" | base64 -d`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "hello" {
			t.Errorf("expected 'hello', got %q", out)
		}
	})

	t.Run("base64 encode file via stdin", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/file.bin", []byte("test data"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, `cat /file.bin | base64`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "dGVzdCBkYXRh" {
			t.Errorf("expected 'dGVzdCBkYXRh', got %q", out)
		}
	})

	t.Run("base64 encode positional args", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `base64 foo bar`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "Zm9vIGJhcg==" {
			t.Errorf("expected 'Zm9vIGJhcg==', got %q", out)
		}
	})
}
