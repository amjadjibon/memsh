// Package ssh provides SSH server implementation for memsh.
package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/pem"
	"errors"
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

const defaultMinTimeout = 5 * time.Second

// ErrServerClosed is returned by ListenAndServe after Close.
var ErrServerClosed = gliderssh.ErrServerClosed

// Server wraps a gliderlabs SSH server.
type Server struct {
	*gliderssh.Server
}

// Config holds SSH server configuration.
type Config struct {
	Addr        string
	APIKey      string
	HostKeyFile string
	Store       *session.Store
	BaseOpts    []shell.Option
	Timeout     time.Duration
	MinTimeout  time.Duration
	Limits      session.Limits
}

// New creates a new SSH server with the given configuration.
func New(cfg Config) (*Server, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("session store is required")
	}
	if cfg.MinTimeout <= 0 {
		cfg.MinTimeout = defaultMinTimeout
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = cfg.MinTimeout
	}

	signer, err := loadOrGenerateHostKey(cfg.HostKeyFile)
	if err != nil {
		return nil, fmt.Errorf("SSH host key: %w", err)
	}

	srv := &gliderssh.Server{
		Addr:        cfg.Addr,
		HostSigners: []gliderssh.Signer{signer},
		Handler: func(s gliderssh.Session) {
			handleSession(s, cfg.Store, cfg.BaseOpts, cfg.Timeout, cfg.MinTimeout, cfg.Limits)
		},
	}

	if cfg.APIKey != "" {
		// Require password == API key.
		srv.PasswordHandler = func(_ gliderssh.Context, password string) bool {
			return subtle.ConstantTimeCompare([]byte(password), []byte(cfg.APIKey)) == 1
		}
	}

	return &Server{Server: srv}, nil
}

// handleSession runs a memsh shell for an incoming SSH connection.
func handleSession(s gliderssh.Session, store *session.Store, baseOpts []shell.Option, timeout, minTimeout time.Duration, limits session.Limits) {
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

	// non-interactive: single command
	if len(cmdArgs) > 0 {
		if err := limits.ValidateRuntime(entry.Runtime); err != nil {
			fmt.Fprintf(s.Stderr(), "error: %v\n", err)
			_ = s.Exit(1)
			return
		}
		if err := limits.ValidateFS(entry.Fs); err != nil {
			fmt.Fprintf(s.Stderr(), "error: %v\n", err)
			_ = s.Exit(1)
			return
		}

		script := strings.Join(cmdArgs, " ")
		execTimeout, err := limits.EffectiveTimeout(timeout, entry.Runtime, minTimeout)
		if err != nil {
			fmt.Fprintf(s.Stderr(), "error: %v\n", err)
			_ = s.Exit(1)
			return
		}
		var preRunSnap *shell.Snapshot
		if limits.HasFSLimits() {
			snap, snapErr := shell.TakeSnapshot(entry.Fs, entry.Cwd)
			if snapErr != nil {
				fmt.Fprintf(s.Stderr(), "error: failed to snapshot session state: %v\n", snapErr)
				_ = s.Exit(1)
				return
			}
			preRunSnap = snap
		}

		opts := make([]shell.Option, len(baseOpts), len(baseOpts)+3)
		copy(opts, baseOpts)
		opts = append(opts,
			shell.WithFS(entry.Fs),
			shell.WithCwd(entry.Cwd),
			shell.WithStdIO(s, s, s.Stderr()),
			shell.WithNetworkUsage(entry.Network),
		)

		sh, err := shell.New(opts...)
		if err != nil {
			fmt.Fprintf(s.Stderr(), "error: %v\n", err)
			_ = s.Exit(1)
			return
		}
		defer sh.Close()

		ctx, cancel := context.WithTimeout(s.Context(), execTimeout)
		defer cancel()

		session.RestoreAliases(ctx, sh, entry.Fs)
		rcLoaded := false
		if !entry.RcLoaded {
			_ = sh.LoadMemshrc(ctx)
			rcLoaded = true
		}
		startedAt := time.Now()
		runErr := sh.Run(ctx, script)
		elapsed := time.Since(startedAt)
		session.SaveAliases(ctx, sh)
		newCwd := sh.Cwd()
		store.UpdateWithRuntimeAndNetwork(sessionID, newCwd, entry.RcLoaded || rcLoaded, elapsed, sh.NetworkUsage())
		if fsErr := limits.ValidateFS(entry.Fs); fsErr != nil {
			if preRunSnap != nil {
				if restoredFS, restoredCwd, restoreErr := shell.RestoreSnapshot(preRunSnap); restoreErr == nil {
					store.Replace(sessionID, restoredFS, restoredCwd)
				}
			}
			fmt.Fprintf(s.Stderr(), "error: %v\n", fsErr)
			_ = s.Exit(1)
			return
		}
		if rtErr := limits.ValidateRuntime(entry.Runtime); rtErr != nil {
			fmt.Fprintf(s.Stderr(), "error: %v\n", rtErr)
			_ = s.Exit(1)
			return
		}

		if runErr != nil && !errors.Is(runErr, shell.ErrExit) {
			_ = s.Exit(1)
		} else {
			_ = s.Exit(0)
		}
		return
	}

	// interactive REPL
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
		store.UpdateWithRuntime(sessionID, cwd, true, 0)
	}

	for {
		fmt.Fprintf(s, "memsh:%s$ ", cwd)

		line, err := sshReadLine(s)
		if err != nil {
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
		if err := limits.ValidateRuntime(entry.Runtime); err != nil {
			fmt.Fprintf(s, "error: %v\r\n", err)
			continue
		}
		if err := limits.ValidateFS(entry.Fs); err != nil {
			fmt.Fprintf(s, "error: %v\r\n", err)
			continue
		}
		execTimeout, err := limits.EffectiveTimeout(timeout, entry.Runtime, minTimeout)
		if err != nil {
			fmt.Fprintf(s, "error: %v\r\n", err)
			continue
		}
		var preRunSnap *shell.Snapshot
		if limits.HasFSLimits() {
			snap, snapErr := shell.TakeSnapshot(entry.Fs, cwd)
			if snapErr != nil {
				fmt.Fprintf(s, "error: failed to snapshot session state: %v\r\n", snapErr)
				continue
			}
			preRunSnap = snap
		}

		var cmdOut strings.Builder
		opts := make([]shell.Option, len(baseOpts), len(baseOpts)+3)
		copy(opts, baseOpts)
		opts = append(opts,
			shell.WithFS(entry.Fs),
			shell.WithCwd(cwd),
			shell.WithStdIO(strings.NewReader(""), &cmdOut, &cmdOut),
			shell.WithNetworkUsage(entry.Network),
		)

		sh, shErr := shell.New(opts...)
		if shErr != nil {
			fmt.Fprintf(s, "error: %v\r\n", shErr)
			continue
		}

		ctx, cancel := context.WithTimeout(s.Context(), execTimeout)
		session.RestoreAliases(ctx, sh, entry.Fs)
		startedAt := time.Now()
		runErr := sh.Run(ctx, line)
		elapsed := time.Since(startedAt)
		session.SaveAliases(ctx, sh)
		cancel()
		newCwd := sh.Cwd()
		sh.Close()

		store.UpdateWithRuntimeAndNetwork(sessionID, newCwd, true, elapsed, sh.NetworkUsage())
		if fsErr := limits.ValidateFS(entry.Fs); fsErr != nil {
			if preRunSnap != nil {
				if restoredFS, restoredCwd, restoreErr := shell.RestoreSnapshot(preRunSnap); restoreErr == nil {
					store.Replace(sessionID, restoredFS, restoredCwd)
					newCwd = restoredCwd
				}
			}
			fmt.Fprintf(s, "error: %v\r\n", fsErr)
		}
		if rtErr := limits.ValidateRuntime(entry.Runtime); rtErr != nil {
			fmt.Fprintf(s, "error: %v\r\n", rtErr)
		}
		cwd = newCwd

		// Translate bare LF -> CR+LF for the remote terminal.
		output := strings.ReplaceAll(cmdOut.String(), "\r\n", "\n")
		output = strings.ReplaceAll(output, "\n", "\r\n")
		fmt.Fprint(s, output)

		if runErr != nil && !errors.Is(runErr, shell.ErrExit) {
			fmt.Fprintf(s, "%v\r\n", runErr)
		}
	}
	_ = s.Exit(0)
}

// sshReadLine reads one line of input from an SSH PTY session.
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
			if w, ok := r.(io.Writer); ok {
				_, _ = w.Write([]byte("\r\n"))
			}
			return string(line), nil
		case c == 127 || c == '\b': // DEL / backspace
			if len(line) > 0 {
				line = line[:len(line)-1]
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
			return "", nil
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

// loadOrGenerateHostKey loads a persistent Ed25519 host key from keyFile, or
// generates a new one and saves it if the file does not exist.
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
		if mkErr := os.MkdirAll(filepath.Dir(keyFile), 0o700); mkErr == nil {
			_ = os.WriteFile(keyFile, pem.EncodeToMemory(pemBlock), 0o600)
			log.Printf("memsh serve: SSH: host key saved to %s", keyFile)
		}
	}

	return signer, nil
}
