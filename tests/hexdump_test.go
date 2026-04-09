package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestXxd(t *testing.T) {
	ctx := context.Background()

	// "Hello, World!" = 48 65 6c 6c 6f 2c 20 57 6f 72 6c 64 21
	content := []byte("Hello, World!")

	t.Run("default dump has offset hex and ASCII", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.bin", content, 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "xxd /f.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "00000000") {
			t.Errorf("missing offset: %q", out)
		}
		if !strings.Contains(out, "48 65 6c 6c") {
			t.Errorf("missing hex bytes: %q", out)
		}
		if !strings.Contains(out, "Hello, World!") {
			t.Errorf("missing ASCII sidebar: %q", out)
		}
	})

	t.Run("-p plain hex output", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.bin", content, 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "xxd -p /f.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		// should be pure hex, no spaces or addresses
		if strings.Contains(out, " ") {
			t.Errorf("plain output should have no spaces: %q", out)
		}
		if !strings.HasPrefix(strings.ToLower(out), "48656c6c6f") {
			t.Errorf("unexpected hex: %q", out)
		}
	})

	t.Run("-u uppercase hex", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.bin", content, 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "xxd -u /f.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "48 65 6C 6C") {
			t.Errorf("expected uppercase hex: %q", out)
		}
	})

	t.Run("-l limits bytes", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.bin", content, 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "xxd -l 4 /f.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		// only first 4 bytes: "Hell"
		if strings.Contains(out, "World") {
			t.Errorf("should be limited to 4 bytes but got World: %q", out)
		}
		if !strings.Contains(out, "Hell") {
			t.Errorf("expected 'Hell' in output: %q", out)
		}
	})

	t.Run("-s skips bytes", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.bin", content, 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "xxd -s 7 /f.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		// skip "Hello, " (7 bytes), remainder is "World!"
		if !strings.Contains(out, "World!") {
			t.Errorf("expected 'World!' after skip: %q", out)
		}
	})

	t.Run("-c sets columns", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.bin", content, 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "xxd -c 4 /f.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		// 13 bytes / 4 cols = 4 lines (3 full + 1 partial)
		if len(lines) != 4 {
			t.Errorf("expected 4 lines with -c 4, got %d: %q", len(lines), buf.String())
		}
	})

	t.Run("-r reverse hex to binary", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		// create a plain hex file and reverse it
		afero.WriteFile(fs, "/hex.txt", []byte("48656c6c6f"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "xxd -r -p /hex.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if buf.String() != "Hello" {
			t.Errorf("got %q, want %q", buf.String(), "Hello")
		}
	})

	t.Run("round-trip: xxd -p then xxd -r -p", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/orig.bin", content, 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "xxd -p /orig.bin > /hex.txt && xxd -r -p /hex.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if buf.String() != string(content) {
			t.Errorf("round-trip mismatch: got %q, want %q", buf.String(), content)
		}
	})

	t.Run("stdin input", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/in.txt", []byte("hi"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "cat /in.txt | xxd -p"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(strings.ToLower(buf.String()), "6869") {
			t.Errorf("expected hex of 'hi' (6869): %q", buf.String())
		}
	})

	t.Run("-b binary bit dump", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.bin", []byte{0xFF, 0x00}, 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "xxd -b /f.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "11111111") {
			t.Errorf("expected 11111111 for 0xFF: %q", out)
		}
		if !strings.Contains(out, "00000000") {
			t.Errorf("expected 00000000 for 0x00: %q", out)
		}
	})
}

func TestHexdump(t *testing.T) {
	ctx := context.Background()

	content := []byte("Hello, World!")

	t.Run("-C canonical output has offset hex and ASCII", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.bin", content, 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "hexdump -C /f.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "00000000") {
			t.Errorf("missing offset: %q", out)
		}
		if !strings.Contains(out, "48 65 6c 6c") {
			t.Errorf("missing hex bytes: %q", out)
		}
		if !strings.Contains(out, "|Hello, World!|") {
			t.Errorf("missing ASCII sidebar: %q", out)
		}
	})

	t.Run("default two-word output", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.bin", content, 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "hexdump /f.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		// ends with final offset
		if !strings.Contains(out, "000000d") {
			t.Errorf("missing final offset (13 bytes = 0xd): %q", out)
		}
	})

	t.Run("-n limits bytes", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.bin", content, 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "hexdump -C -n 5 /f.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "|Hello|") {
			t.Errorf("expected |Hello| for first 5 bytes: %q", out)
		}
		if strings.Contains(out, "World") {
			t.Errorf("should be limited to 5 bytes: %q", out)
		}
	})

	t.Run("-s skips bytes", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.bin", content, 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "hexdump -C -s 7 /f.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "|World!|") {
			t.Errorf("expected |World!| after skip: %q", out)
		}
	})

	t.Run("duplicate rows collapsed with *", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		// 48 bytes of zeros → multiple identical 16-byte rows
		afero.WriteFile(fs, "/zeros.bin", make([]byte, 48), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "hexdump -C /zeros.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "*") {
			t.Errorf("expected * for collapsed duplicate rows: %q", out)
		}
	})

	t.Run("-v disables collapse", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/zeros.bin", make([]byte, 48), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "hexdump -C -v /zeros.bin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		// 48 bytes / 16 cols = 3 data rows + 1 final offset line
		if len(lines) != 4 {
			t.Errorf("expected 4 lines with -v, got %d", len(lines))
		}
	})

	t.Run("stdin input", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/in.txt", []byte("AB"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "cat /in.txt | hexdump -C"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "41 42") {
			t.Errorf("expected hex 41 42 for 'AB': %q", out)
		}
	})
}
