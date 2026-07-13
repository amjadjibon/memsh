package ssh

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amjadjibon/memsh/internal/session"
)

func TestNewRequiresStore(t *testing.T) {
	_, err := New(Config{APIKey: "key"})
	if err == nil {
		t.Fatal("expected error when Store is nil")
	}
}

func TestNewRequiresAPIKey(t *testing.T) {
	store := session.New(context.Background(), time.Minute, 10)
	_, err := New(Config{Store: store})
	if err == nil {
		t.Fatal("expected error when APIKey is empty")
	}
}

func TestNewAppliesDefaultTimeouts(t *testing.T) {
	store := session.New(context.Background(), time.Minute, 10)
	dir := t.TempDir()
	srv, err := New(Config{
		Store:       store,
		APIKey:      "secret",
		HostKeyFile: filepath.Join(dir, "host_key"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv == nil || srv.Server == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestSecureAPIKeyEqual(t *testing.T) {
	if !secureAPIKeyEqual("abc", "abc") {
		t.Error("expected equal keys to match")
	}
	if secureAPIKeyEqual("abc", "abd") {
		t.Error("expected different keys to not match")
	}
	if secureAPIKeyEqual("abc", "abcd") {
		t.Error("expected different-length keys to not match")
	}
	if !secureAPIKeyEqual("", "") {
		t.Error("expected equal (empty) keys to match; callers must reject empty API keys separately")
	}
}

type rwPair struct {
	io.Reader
	io.Writer
}

func TestSSHReadLine(t *testing.T) {
	t.Run("reads a simple line terminated by newline", func(t *testing.T) {
		r := rwPair{Reader: strings.NewReader("hello\n"), Writer: io.Discard}
		line, err := sshReadLine(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if line != "hello" {
			t.Fatalf("expected %q, got %q", "hello", line)
		}
	})

	t.Run("handles backspace", func(t *testing.T) {
		r := rwPair{Reader: strings.NewReader("helloo\b\r"), Writer: io.Discard}
		line, err := sshReadLine(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if line != "hello" {
			t.Fatalf("expected %q, got %q", "hello", line)
		}
	})

	t.Run("Ctrl-U kills the line", func(t *testing.T) {
		r := rwPair{Reader: strings.NewReader("hello\x15world\n"), Writer: io.Discard}
		line, err := sshReadLine(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if line != "world" {
			t.Fatalf("expected %q, got %q", "world", line)
		}
	})

	t.Run("Ctrl-C discards the line", func(t *testing.T) {
		r := rwPair{Reader: strings.NewReader("hello\x03"), Writer: io.Discard}
		line, err := sshReadLine(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if line != "" {
			t.Fatalf("expected empty line, got %q", line)
		}
	})

	t.Run("Ctrl-D on empty line returns EOF", func(t *testing.T) {
		r := rwPair{Reader: strings.NewReader("\x04"), Writer: io.Discard}
		_, err := sshReadLine(r)
		if err != io.EOF {
			t.Fatalf("expected io.EOF, got %v", err)
		}
	})

	t.Run("EOF mid-line returns partial line and error", func(t *testing.T) {
		r := rwPair{Reader: strings.NewReader("partial"), Writer: io.Discard}
		line, err := sshReadLine(r)
		if err == nil {
			t.Fatal("expected error at EOF")
		}
		if line != "partial" {
			t.Fatalf("expected %q, got %q", "partial", line)
		}
	})
}

func TestLoadOrGenerateHostKey(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "ssh_host_key")

	signer1, err := loadOrGenerateHostKey(keyFile)
	if err != nil {
		t.Fatalf("unexpected error generating key: %v", err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		t.Fatalf("expected key file to be persisted: %v", err)
	}

	signer2, err := loadOrGenerateHostKey(keyFile)
	if err != nil {
		t.Fatalf("unexpected error loading key: %v", err)
	}
	if string(signer1.PublicKey().Marshal()) != string(signer2.PublicKey().Marshal()) {
		t.Fatal("expected loaded key to match generated key")
	}
}

func TestLoadOrGenerateHostKeyRegeneratesOnCorruptFile(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "ssh_host_key")
	if err := os.WriteFile(keyFile, []byte("not a valid key"), 0o600); err != nil {
		t.Fatalf("failed to seed corrupt key file: %v", err)
	}

	signer, err := loadOrGenerateHostKey(keyFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signer == nil {
		t.Fatal("expected a signer even when existing key file is corrupt")
	}
}
