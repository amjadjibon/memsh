package tests

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	"github.com/spf13/afero"
	gossh "golang.org/x/crypto/ssh"

	"github.com/amjadjibon/memsh/pkg/shell"
)

// startTestSSHServer starts an in-process memsh SSH server on a random port.
// It returns the address ("127.0.0.1:PORT") and a cancel func.
func startTestSSHServer(t *testing.T, fs afero.Fs, apiKey string) string {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	// Find a free port.
	ln := newLoopbackListener(t)
	addr := ln.Addr().String()
	ln.Close()

	srv := &gliderssh.Server{
		Addr:        addr,
		HostSigners: []gliderssh.Signer{signer},
		Handler: func(s gliderssh.Session) {
			cwd := "/"
			cmdArgs := s.Command()
			timeout := 10 * time.Second

			if len(cmdArgs) > 0 {
				script := strings.Join(cmdArgs, " ")
				var out strings.Builder
				sh, newErr := shell.New(
					shell.WithFS(fs),
					shell.WithCwd(cwd),
					shell.WithStdIO(s, &out, &out),
					shell.WithWASMEnabled(false),
				)
				if newErr != nil {
					fmt.Fprintf(s.Stderr(), "error: %v\n", newErr)
					_ = s.Exit(1)
					return
				}
				defer sh.Close()

				ctx, cancel := context.WithTimeout(s.Context(), timeout)
				defer cancel()

				runErr := sh.Run(ctx, script)
				// Write buffered output to session stdout.
				_, _ = s.Write([]byte(out.String()))
				if runErr != nil {
					_ = s.Exit(1)
				} else {
					_ = s.Exit(0)
				}
				return
			}

			// Interactive mode.
			scanner := bufio.NewScanner(s)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				if line == "exit" || line == "quit" {
					break
				}
				var out strings.Builder
				sh, newErr := shell.New(
					shell.WithFS(fs),
					shell.WithCwd(cwd),
					shell.WithStdIO(strings.NewReader(""), &out, &out),
					shell.WithWASMEnabled(false),
				)
				if newErr != nil {
					fmt.Fprintf(s, "error: %v\n", newErr)
					continue
				}
				ctx, cancel := context.WithTimeout(s.Context(), timeout)
				runErr := sh.Run(ctx, line)
				cancel()
				cwd = sh.Cwd()
				sh.Close()
				_, _ = s.Write([]byte(out.String()))
				if runErr != nil {
					fmt.Fprintf(s, "error: %v\n", runErr)
				}
			}
			_ = s.Exit(0)
		},
	}

	if apiKey != "" {
		srv.PasswordHandler = func(_ gliderssh.Context, password string) bool {
			return password == apiKey
		}
	} else {
		srv.PasswordHandler = func(_ gliderssh.Context, _ string) bool { return true }
	}

	go func() { _ = srv.ListenAndServe() }()
	t.Cleanup(func() { _ = srv.Close() })

	// Wait until the server is ready.
	for i := 0; i < 20; i++ {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			c.Close()
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("SSH server at %s did not start", addr)
	return ""
}

func TestSSH(t *testing.T) {
	ctx := context.Background()

	t.Run("single command returns output", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		addr := startTestSSHServer(t, fs, "")
		host, port, _ := net.SplitHostPort(addr)

		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		err := s.Run(ctx, fmt.Sprintf("ssh -P %s memsh@%s -- echo hello", port, host))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "hello") {
			t.Errorf("expected 'hello' in output, got: %q", buf.String())
		}
	})

	t.Run("single command with password auth", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		addr := startTestSSHServer(t, fs, "secret")
		host, port, _ := net.SplitHostPort(addr)

		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		err := s.Run(ctx, fmt.Sprintf("ssh -p secret -P %s memsh@%s -- echo auth_ok", port, host))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "auth_ok") {
			t.Errorf("expected 'auth_ok', got: %q", buf.String())
		}
	})

	t.Run("wrong password fails", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		addr := startTestSSHServer(t, fs, "secret")
		host, port, _ := net.SplitHostPort(addr)

		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		err := s.Run(ctx, fmt.Sprintf("ssh -p wrong -P %s memsh@%s -- echo hi", port, host))
		if err == nil {
			t.Fatal("expected error for wrong password, got nil")
		}
	})

	t.Run("unreachable host exits 255", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		err := s.Run(ctx, "ssh -P 19999 127.0.0.1 -- echo hi")
		if exitCode(err) != 255 {
			t.Errorf("expected exit 255 for unreachable host, got: %v", err)
		}
	})

	t.Run("single command file creation persists in remote FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		addr := startTestSSHServer(t, fs, "")
		host, port, _ := net.SplitHostPort(addr)

		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		// Create a file on the remote.
		if err := s.Run(ctx, fmt.Sprintf("ssh -P %s %s -- 'echo remote > /remotefile.txt'", port, host)); err != nil {
			t.Fatalf("write command: %v", err)
		}

		// Verify it exists in the remote FS.
		if ok, _ := afero.Exists(fs, "/remotefile.txt"); !ok {
			t.Error("expected /remotefile.txt to exist in remote FS after command")
		}
	})

	t.Run("missing host argument exits 1", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		err := s.Run(ctx, "ssh")
		if exitCode(err) != 1 {
			t.Errorf("expected exit 1 for missing host, got: %v", err)
		}
	})
}
