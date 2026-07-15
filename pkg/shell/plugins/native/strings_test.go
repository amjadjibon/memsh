package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/tests"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestStrings(t *testing.T) {
	ctx := context.Background()

	t.Run("extracts printable strings from binary stdin", func(t *testing.T) {
		// Build input: null bytes interspersed with printable text.
		fs := afero.NewMemMapFs()
		// Write a binary file with embedded strings.
		data := []byte{0x00, 0x00, 'h', 'e', 'l', 'l', 'o', 0x00, 0x00,
			'w', 'o', 'r', 'l', 'd', 0x00}
		afero.WriteFile(fs, "/bin.dat", data, 0o644)

		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "strings /bin.dat"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "hello") {
			t.Errorf("expected 'hello' in output, got %q", out)
		}
		if !strings.Contains(out, "world") {
			t.Errorf("expected 'world' in output, got %q", out)
		}
	})

	t.Run("-n raises minimum length threshold", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		// "hi" is 2 chars, "hello" is 5 — only "hello" should appear with -n 4
		data := []byte{0x00, 'h', 'i', 0x00, 'h', 'e', 'l', 'l', 'o', 0x00}
		afero.WriteFile(fs, "/bin.dat", data, 0o644)

		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "strings -n 4 /bin.dat"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if strings.Contains(out, "hi") {
			t.Errorf("'hi' should be filtered by -n 4, got %q", out)
		}
		if !strings.Contains(out, "hello") {
			t.Errorf("expected 'hello' with -n 4, got %q", out)
		}
	})

	t.Run("-t d shows decimal offsets", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		data := []byte{0x00, 0x00, 0x00, 'h', 'e', 'l', 'l', 'o', 0x00}
		afero.WriteFile(fs, "/bin.dat", data, 0o644)

		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "strings -t d /bin.dat"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "3") {
			t.Errorf("expected offset '3' in output, got %q", out)
		}
		if !strings.Contains(out, "hello") {
			t.Errorf("expected 'hello' in output, got %q", out)
		}
	})

	t.Run("-t x shows hex offsets", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		data := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // 16 null bytes
			'h', 'e', 'l', 'l', 'o', 0x00}
		afero.WriteFile(fs, "/bin.dat", data, 0o644)

		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "strings -t x /bin.dat"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "10") { // 16 decimal = 10 hex
			t.Errorf("expected hex offset '10' in output, got %q", out)
		}
	})

	t.Run("reads from stdin when no file given", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		data := []byte{0x00, 'w', 'o', 'r', 'l', 'd', 0x00}
		afero.WriteFile(fs, "/input.bin", data, 0o644)

		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "strings < /input.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "world") {
			t.Errorf("expected 'world' from stdin, got %q", buf.String())
		}
	})

	t.Run("pure ASCII file returns all text", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/text.txt", []byte("hello world\nfoo bar\n"), 0o644)

		var buf strings.Builder
		s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "strings /text.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "hello world") {
			t.Errorf("expected 'hello world', got %q", out)
		}
	})
}
