package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/shell"
)

func TestJq(t *testing.T) {
	ctx := context.Background()

	t.Run("jq identity from stdin", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo '{"a":1}' | jq .`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, `"a"`) || !strings.Contains(out, `1`) {
			t.Errorf("unexpected output: %q", out)
		}
	})

	t.Run("jq field selection", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo '{"name":"alice","age":30}' | jq .name`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != `"alice"` {
			t.Errorf("expected '\"alice\"', got %q", out)
		}
	})

	t.Run("jq -r raw string output", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo '{"name":"alice"}' | jq -r .name`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "alice" {
			t.Errorf("expected 'alice', got %q", out)
		}
	})

	t.Run("jq -c compact output", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo '{"a":1,"b":2}' | jq -c .`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if strings.Contains(out, "\n  ") {
			t.Errorf("expected compact output, got indented: %q", out)
		}
		if !strings.Contains(out, `"a":1`) {
			t.Errorf("unexpected compact output: %q", out)
		}
	})

	t.Run("jq array iteration", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo '[1,2,3]' | jq '.[]'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		lines := strings.Split(out, "\n")
		if len(lines) != 3 || lines[0] != "1" || lines[1] != "2" || lines[2] != "3" {
			t.Errorf("expected '1\\n2\\n3', got %q", out)
		}
	})

	t.Run("jq reads from virtual FS file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/data.json", []byte(`{"msg":"hello"}`), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, `jq -r .msg /data.json`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "hello" {
			t.Errorf("expected 'hello', got %q", out)
		}
	})

	t.Run("jq pipe expression", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo '{"items":[1,2,3]}' | jq '.items | length'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "3" {
			t.Errorf("expected '3', got %q", out)
		}
	})

	t.Run("jq -n null input", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `jq -n '{a:1,b:2}'`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, `"a"`) {
			t.Errorf("unexpected output: %q", out)
		}
	})

	t.Run("jq combined flags -rc", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo '{"x":"hello"}' | jq -rc .x`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "hello" {
			t.Errorf("expected 'hello', got %q", out)
		}
	})

	t.Run("jq invalid expression returns error", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		err := s.Run(ctx, `echo '{}' | jq '..invalid..'`)
		if err == nil {
			t.Fatal("expected error for invalid expression, got nil")
		}
	})
}
