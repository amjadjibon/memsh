// Package ssh provides SSH server implementation for memsh, allowing
// remote shell access via the SSH protocol. The SSH server integrates
// with the memsh session store for filesystem persistence across connections.
//
// Features:
//   - Password authentication using optional API key
//   - No authentication mode (if no API key configured)
//   - Interactive PTY sessions with full readline support
//   - Non-interactive command execution
//   - Virtual filesystem persistence via session store
//   - Automatic Ed25519 host key generation and persistence
package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/amjadjibon/memsh/internal/session"
	"github.com/amjadjibon/memsh/pkg/shell"
)

// Server wraps the gliderlabs SSH server.
type Server struct {
	*gliderssh.Server
}

// Config holds the SSH server configuration.
type Config struct {
	Addr        string
	APIKey      string
	HostKeyFile string
	Store       *session.Store
	BaseOpts    []shell.Option
	Timeout     time.Duration
}

// New creates a new SSH server with the given configuration.
func New(cfg Config) (*Server, error) {
	signer, err := loadOrGenerateHostKey(cfg.HostKeyFile)
	if err != nil {
		return nil, fmt.Errorf("SSH host key: %w", err)
	}

	srv := &gliderssh.Server{
		Addr:        cfg.Addr,
		HostSigners: []gliderssh.Signer{signer},
		Handler: func(s gliderssh.Session) {
			handleSession(s, cfg.Store, cfg.BaseOpts, cfg.Timeout)
		},
	}

	if cfg.APIKey != "" {
		// Require password == API key.
		srv.PasswordHandler = func(_ gliderssh.Context, password string) bool {
			return subtle.ConstantTimeCompare([]byte(password), []byte(cfg.APIKey)) == 1
		}
	}
	// No API key → leave all auth handlers nil; gliderlabs/ssh will set
	// NoClientAuth = true automatically, so no password prompt is shown.

	return &Server{Server: srv}, nil
}

// loadOrGenerateHostKey loads a persistent Ed25519 host key from keyFile, or
// generates a new one and saves it if the file does not exist.  A stable host
// key prevents "REMOTE HOST IDENTIFICATION HAS CHANGED" warnings on reconnect.
//
// keyFile defaults to ~/.memsh/ssh_host_key when empty.
func loadOrGenerateHostKey(keyFile string) (gliderssh.Signer, error) {
	if keyFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		keyFile = filepath.Join(home, ".memsh", "ssh_host_key")
	}

	// Try to load an existing key.
	if data, err := os.ReadFile(keyFile); err == nil {
		signer, parseErr := gossh.ParsePrivateKey(data)
		if parseErr == nil {
			return signer, nil
		}
		log.Printf("memsh serve: SSH: could not parse host key %s (%v); generating new key", keyFile, parseErr)
	}

	// Generate a fresh Ed25519 key and persist it.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate host key: %w", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("create signer: %w", err)
	}

	pemBlock, err := gossh.MarshalPrivateKey(priv, "memsh ssh host key")
	if err == nil {
		// Get the directory from the keyFile path
		dir := "."
		if lastSlash := strings.LastIndex(keyFile, "/"); lastSlash > 0 {
			dir = keyFile[:lastSlash]
		}
		if mkErr := os.MkdirAll(dir, 0o700); mkErr == nil {
			_ = os.WriteFile(keyFile, pem.EncodeToMemory(pemBlock), 0o600)
			log.Printf("memsh serve: SSH: host key saved to %s", keyFile)
		}
	}

	return signer, nil
}

// handleSession runs a memsh shell for an incoming SSH connection.
//
// The SSH username is used as the session ID so the virtual FS persists across
// reconnects.  If the client supplies a command (non-interactive mode) it is
// run once and the session exits.  Otherwise an interactive REPL is served.
//
// PTY handling: when the SSH client requests a PTY (as the system ssh(1) does
// for interactive sessions), input arrives one raw byte at a time with no line
// buffering.  sshReadLine reads characters, echoes them, and handles backspace /
// Ctrl-C / Ctrl-D so the user gets a proper editing experience.  Command output
// has bare LF translated to CR+LF so it renders correctly in the remote terminal.
func handleSession(s gliderssh.Session, store *session.Store, baseOpts []shell.Option, timeout time.Duration) {
	sessionID := s.User()
	if sessionID == "" || sessionID == "memsh" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		sessionID = fmt.Sprintf("%x", b)
	}

	entry, ok := store.Get(sessionID)
	if !ok {
		fmt.Fprintln(s.Stderr(), "error: maximum number of sessions reached")
		_ = s.Exit(1)
		return
	}

	cmdArgs := s.Command()

	// ── non-interactive: single command ──────────────────────────────────
	if len(cmdArgs) > 0 {
		script := strings.Join(cmdArgs, " ")

		opts := make([]shell.Option, len(baseOpts), len(baseOpts)+3)
		copy(opts, baseOpts)
		opts = append(opts,
			shell.WithFS(entry.Fs),
			shell.WithCwd(entry.Cwd),
			shell.WithStdIO(s, s, s.Stderr()),
		)

		sh, err := shell.New(opts...)
		if err != nil {
			fmt.Fprintf(s.Stderr(), "error: %v\n", err)
			_ = s.Exit(1)
			return
		}
		defer sh.Close()

		ctx, cancel := context.WithTimeout(s.Context(), timeout)
		defer cancel()

		session.RestoreAliases(ctx, sh, entry.Fs)
		rcLoaded := false
		if !entry.RcLoaded {
			_ = sh.LoadMemshrc(ctx)
			rcLoaded = true
		}
		runErr := sh.Run(ctx, script)
		session.SaveAliases(ctx, sh)
		store.Update(sessionID, sh.Cwd(), entry.RcLoaded || rcLoaded)

		if runErr != nil && !isExit(runErr) {
			_ = s.Exit(1)
		} else {
			_ = s.Exit(0)
		}
		return
	}

	// ── interactive REPL ─────────────────────────────────────────────────
	// Load .memshrc once at session start for interactive mode.
	cwd := entry.Cwd
	if !entry.RcLoaded {
		initOpts := make([]shell.Option, len(baseOpts), len(baseOpts)+3)
		copy(initOpts, baseOpts)
		initOpts = append(initOpts,
			shell.WithFS(entry.Fs),
			shell.WithCwd(cwd),
			shell.WithStdIO(strings.NewReader(""), io.Discard, io.Discard),
		)
		if rcSh, rcErr := shell.New(initOpts...); rcErr == nil {
			_ = rcSh.LoadMemshrc(s.Context())
			session.SaveAliases(s.Context(), rcSh)
			rcSh.Close()
		}
		store.Update(sessionID, cwd, true)
	}

	for {
		// Print prompt — use CR+LF so the cursor returns to column 0.
		fmt.Fprintf(s, "memsh:%s$ ", cwd)

		line, err := sshReadLine(s)
		if err != nil {
			// io.EOF → client pressed Ctrl-D on empty line or disconnected.
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			fmt.Fprint(s, "logout\r\n")
			break
		}

		var cmdOut strings.Builder
		opts := make([]shell.Option, len(baseOpts), len(baseOpts)+3)
		copy(opts, baseOpts)
		opts = append(opts,
			shell.WithFS(entry.Fs),
			shell.WithCwd(cwd),
			shell.WithStdIO(strings.NewReader(""), &cmdOut, &cmdOut),
		)

		sh, shErr := shell.New(opts...)
		if shErr != nil {
			fmt.Fprintf(s, "error: %v\r\n", shErr)
			continue
		}

		ctx, cancel := context.WithTimeout(s.Context(), timeout)
		session.RestoreAliases(ctx, sh, entry.Fs)
		runErr := sh.Run(ctx, line)
		session.SaveAliases(ctx, sh)
		cancel()
		newCwd := sh.Cwd()
		sh.Close()

		store.Update(sessionID, newCwd, true)
		cwd = newCwd

		// Translate bare LF → CR+LF for the remote terminal.
		output := strings.ReplaceAll(cmdOut.String(), "\r\n", "\n") // normalise first
		output = strings.ReplaceAll(output, "\n", "\r\n")
		fmt.Fprint(s, output)

		if runErr != nil && !isExit(runErr) {
			fmt.Fprintf(s, "%v\r\n", runErr)
		}
	}
	_ = s.Exit(0)
}

// sshReadLine reads one line of input from an SSH PTY session.
//
// The PTY delivers raw bytes: Enter sends CR (\r), not LF (\n).  Characters
// must be echoed back manually.  Basic editing is supported:
//   - Backspace / DEL (0x7f): erase last character
//   - Ctrl-U (0x15):          kill entire line
//   - Ctrl-C (0x03):          discard line, return empty string (not an error)
//   - Ctrl-D (0x04):          EOF when the line is empty; ignored otherwise
func sshReadLine(r io.Reader) (string, error) {
	var line []byte
	buf := make([]byte, 1)
	for {
		_, err := r.Read(buf)
		if err != nil {
			return string(line), err
		}
		c := buf[0]
		switch {
		case c == '\r' || c == '\n':
			// Echo CR+LF so the cursor moves to the next line before output appears.
			if w, ok := r.(io.Writer); ok {
				_, _ = w.Write([]byte("\r\n"))
			}
			return string(line), nil
		case c == 127 || c == '\b': // DEL / backspace
			if len(line) > 0 {
				line = line[:len(line)-1]
				// Erase character: move back, print space, move back again.
				if w, ok := r.(io.Writer); ok {
					_, _ = w.Write([]byte("\b \b"))
				}
			}
		case c == 21: // Ctrl-U: kill line
			if len(line) > 0 {
				if w, ok := r.(io.Writer); ok {
					_, _ = w.Write([]byte(strings.Repeat("\b", len(line)) +
						strings.Repeat(" ", len(line)) +
						strings.Repeat("\b", len(line))))
				}
				line = line[:0]
			}
		case c == 3: // Ctrl-C: discard line
			if w, ok := r.(io.Writer); ok {
				_, _ = w.Write([]byte("^C\r\n"))
			}
			return "", nil // empty line — caller reprints prompt
		case c == 4: // Ctrl-D: EOF only on empty line
			if len(line) == 0 {
				return "", io.EOF
			}
		case c >= 32 && c < 127: // printable ASCII
			line = append(line, c)
			if w, ok := r.(io.Writer); ok {
				_, _ = w.Write([]byte{c})
			}
		}
	}
}

func isExit(err error) bool {
	return err != nil && err.Error() == "exit"
}
