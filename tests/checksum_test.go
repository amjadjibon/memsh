package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/shell"
)

func TestChecksum(t *testing.T) {
	ctx := context.Background()

	// known-good values computed externally
	const (
		data       = "hello world\n"
		md5Hex     = "e59ff97941044f85df5297e1c302d260"
		sha1Hex    = "22596363b3de40b06f981fb85d82312e8c0ed511"
		sha256Hex  = "a948904f2f0f479b8f936133eedbb0e2442df4b5095c67b3e5fe3f3c4c52f3f2" // echo -n doesn't add newline; let's use known val
		sha512Hex  = ""
	)
	// use a deterministic file
	content := []byte("hello")
	// pre-computed: echo -n "hello" | sha256sum
	const helloSHA256 = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	const helloMD5    = "5d41402abc4b2a76b9719d911017c592"
	const helloSHA1   = "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
	const helloSHA512 = "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043"

	t.Run("sha256sum single file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/hello.txt", content, 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "sha256sum /hello.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.HasPrefix(out, helloSHA256) {
			t.Errorf("got %q, want prefix %q", out, helloSHA256)
		}
		if !strings.Contains(out, "/hello.txt") {
			t.Errorf("output missing filename: %q", out)
		}
	})

	t.Run("md5sum single file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/hello.txt", content, 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "md5sum /hello.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.HasPrefix(out, helloMD5) {
			t.Errorf("got %q, want prefix %q", out, helloMD5)
		}
	})

	t.Run("sha1sum single file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/hello.txt", content, 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "sha1sum /hello.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.HasPrefix(out, helloSHA1) {
			t.Errorf("got %q, want prefix %q", out, helloSHA1)
		}
	})

	t.Run("sha512sum single file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/hello.txt", content, 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "sha512sum /hello.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.HasPrefix(out, helloSHA512[:20]) {
			t.Errorf("got %q, unexpected prefix", out)
		}
	})

	t.Run("multiple files", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/a.txt", []byte("hello"), 0644)
		afero.WriteFile(fs, "/b.txt", []byte("world"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "sha256sum /a.txt /b.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d: %q", len(lines), buf.String())
		}
		if !strings.Contains(lines[0], "/a.txt") {
			t.Errorf("line 0 missing /a.txt: %q", lines[0])
		}
		if !strings.Contains(lines[1], "/b.txt") {
			t.Errorf("line 1 missing /b.txt: %q", lines[1])
		}
	})

	t.Run("stdin input", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, `echo -n hello | sha256sum`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.HasPrefix(out, helloSHA256) {
			t.Errorf("got %q, want prefix %q", out, helloSHA256)
		}
	})

	t.Run("check mode passes", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/hello.txt", content, 0644)
		// write a valid checksum file
		sumLine := helloSHA256 + "  /hello.txt\n"
		afero.WriteFile(fs, "/sums.txt", []byte(sumLine), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "sha256sum -c /sums.txt"); err != nil {
			t.Fatalf("check mode failed: %v", err)
		}
		if !strings.Contains(buf.String(), "OK") {
			t.Errorf("expected OK, got %q", buf.String())
		}
	})

	t.Run("check mode fails on mismatch", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/hello.txt", content, 0644)
		// write a wrong checksum
		sumLine := strings.Repeat("0", 64) + "  /hello.txt\n"
		afero.WriteFile(fs, "/badsums.txt", []byte(sumLine), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		err := s.Run(ctx, "sha256sum -c /badsums.txt")
		if err == nil {
			t.Fatal("expected non-zero exit on mismatch")
		}
		if !strings.Contains(buf.String(), "FAILED") {
			t.Errorf("expected FAILED in output, got %q", buf.String())
		}
	})

	t.Run("missing file exits 1", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		err := s.Run(ctx, "sha256sum /nonexistent.txt")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("pipe sha256sum into grep", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/a.txt", []byte("hello"), 0644)
		afero.WriteFile(fs, "/b.txt", []byte("world"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "sha256sum /a.txt /b.txt | grep a.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, helloSHA256) {
			t.Errorf("got %q, want sha256 of 'hello'", out)
		}
	})

	_ = md5Hex
	_ = sha1Hex
	_ = sha256Hex
	_ = sha512Hex
	_ = helloMD5
}
