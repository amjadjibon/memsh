package native

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/afero"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

// SSHPlugin connects to a remote memsh SSH server (started with memsh serve --ssh).
//
// Usage:
//
//	ssh [-p password] [-P port] [user@]host[:port] [-- command args...]
//
// Without "--": opens an interactive remote REPL (reads from hc.Stdin).
// With "--":    runs a single command on the remote and returns its output.
//
// The SSH username is used as the remote session ID, so reconnecting with the
// same username resumes the same virtual filesystem.  Use a random username
// for an isolated session (the server auto-generates one if you use "memsh").
type SSHPlugin struct{}

func (SSHPlugin) Name() string        { return "ssh" }
func (SSHPlugin) Description() string { return "connect to a remote memsh serve --ssh instance" }
func (SSHPlugin) Usage() string {
	return "ssh [-p password] [-P port] [user@]host[:port] [-- command...]  (default port 22; use -P 2222 for memsh serve --ssh)"
}

var _ plugins.PluginInfo = SSHPlugin{}

func (SSHPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	// ── flag parsing ──────────────────────────────────────────────────────
	password := ""
	portOverride := ""
	var positional []string
	var cmdArgs []string

	endFlags := false
	dashdash := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if dashdash {
			cmdArgs = append(cmdArgs, a)
			continue
		}
		if a == "--" {
			dashdash = true
			continue
		}
		if endFlags || a == "" || a[0] != '-' {
			positional = append(positional, a)
			endFlags = true
			continue
		}
		switch {
		case a == "-p" || a == "--password":
			if i+1 < len(args) {
				password = args[i+1]
				i++
			}
		case len(a) > 2 && a[:2] == "-p":
			password = a[2:]
		case a == "-P" || a == "--port":
			if i+1 < len(args) {
				portOverride = args[i+1]
				i++
			}
		case len(a) > 2 && a[:2] == "-P":
			portOverride = a[2:]
			// ignore other flags for compatibility (-o, -i, etc.)
		}
	}

	if len(positional) == 0 {
		fmt.Fprintln(hc.Stderr, "usage: "+SSHPlugin{}.Usage())
		return interp.ExitStatus(1)
	}

	// ── parse [user@]host[:port] ──────────────────────────────────────────
	target := positional[0]
	user := "memsh"
	hostport := target

	if idx := strings.LastIndex(target, "@"); idx >= 0 {
		user = target[:idx]
		hostport = target[idx+1:]
	}

	host := hostport
	port := "22"
	if portOverride != "" {
		port = portOverride
	}
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		host = h
		if portOverride == "" {
			port = p
		}
	}
	addr := net.JoinHostPort(host, port)

	// ── SSH client config ─────────────────────────────────────────────────
	sc := plugins.ShellCtx(ctx)
	cfg := &gossh.ClientConfig{
		User: user,
		Auth: []gossh.AuthMethod{
			gossh.Password(password),
		},
		HostKeyCallback: sshTOFUCallback(hc, sc),
		Timeout:         15 * time.Second,
	}

	client, err := gossh.Dial("tcp", addr, cfg)
	if err != nil {
		fmt.Fprintf(hc.Stderr, "ssh: %s: %v\n", addr, err)
		return interp.ExitStatus(255)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		fmt.Fprintf(hc.Stderr, "ssh: new session: %v\n", err)
		return interp.ExitStatus(255)
	}
	defer sess.Close()

	sess.Stdin = hc.Stdin
	sess.Stdout = hc.Stdout
	sess.Stderr = hc.Stderr

	// ── single command mode ───────────────────────────────────────────────
	if len(cmdArgs) > 0 {
		cmd := strings.Join(cmdArgs, " ")
		if runErr := sess.Run(cmd); runErr != nil {
			var exitErr *gossh.ExitError
			if errors.As(runErr, &exitErr) {
				code := exitErr.ExitStatus()
				if code > 255 {
					code = 1
				}
				return interp.ExitStatus(uint8(code))
			}
			fmt.Fprintf(hc.Stderr, "ssh: %v\n", runErr)
			return interp.ExitStatus(1)
		}
		return nil
	}

	// ── interactive mode: request a PTY ──────────────────────────────────
	width, height := 80, 24
	if f, ok := hc.Stdout.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		if w, h, sizeErr := term.GetSize(int(f.Fd())); sizeErr == nil {
			width, height = w, h
		}
	}

	modes := gossh.TerminalModes{
		gossh.ECHO:          1,
		gossh.TTY_OP_ISPEED: 38400,
		gossh.TTY_OP_OSPEED: 38400,
	}
	if ptyErr := sess.RequestPty("xterm-256color", height, width, modes); ptyErr != nil {
		fmt.Fprintf(hc.Stderr, "ssh: request pty: %v\n", ptyErr)
		return interp.ExitStatus(1)
	}

	if shellErr := sess.Shell(); shellErr != nil {
		fmt.Fprintf(hc.Stderr, "ssh: start shell: %v\n", shellErr)
		return interp.ExitStatus(1)
	}

	// Block until the remote shell exits or the context is cancelled.
	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()

	select {
	case waitErr := <-done:
		if waitErr != nil {
			var exitErr *gossh.ExitError
			if errors.As(waitErr, &exitErr) {
				code := exitErr.ExitStatus()
				if code > 255 {
					code = 1
				}
				return interp.ExitStatus(uint8(code))
			}
			// EOF on a normal disconnect is not an error.
		}
	case <-ctx.Done():
		_ = sess.Close()
		<-done
	}
	return nil
}

// sshTOFUCallback returns a HostKeyCallback implementing Trust-On-First-Use:
// known keys are stored in /.memsh/known_hosts in the virtual FS and verified
// on subsequent connections; unknown keys prompt the user for confirmation.
func sshTOFUCallback(hc interp.HandlerContext, sc plugins.ShellContext) gossh.HostKeyCallback {
	const knownHostsPath = "/.memsh/known_hosts"
	return func(hostname string, _ net.Addr, key gossh.PublicKey) error {
		vfs := sc.FS
		fingerprint := gossh.FingerprintSHA256(key)

		// Check existing entries in the virtual known_hosts file.
		if vfs != nil {
			if data, err := afero.ReadFile(vfs, knownHostsPath); err == nil && len(data) > 0 {
				rest := data
				for len(rest) > 0 {
					_, hosts, storedKey, _, remainder, parseErr := gossh.ParseKnownHosts(rest)
					rest = remainder
					if parseErr != nil {
						break
					}
					for _, h := range hosts {
						if h != hostname {
							continue
						}
						if storedKey.Type() == key.Type() && bytes.Equal(storedKey.Marshal(), key.Marshal()) {
							return nil // key matches stored entry
						}
						if storedKey.Type() == key.Type() {
							// Same key type but different key — possible MITM.
							fmt.Fprintf(hc.Stderr, "ssh: WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!\n")
							fmt.Fprintf(hc.Stderr, "ssh: Host key for %s has changed (expected %s)\n",
								hostname, gossh.FingerprintSHA256(storedKey))
							return fmt.Errorf("ssh: host key verification failed for %s", hostname)
						}
					}
				}
			}
		}

		// Host not seen before — TOFU prompt.
		fmt.Fprintf(hc.Stderr, "The authenticity of host '%s' can't be established.\n", hostname)
		fmt.Fprintf(hc.Stderr, "%s key fingerprint is %s.\n", key.Type(), fingerprint)
		fmt.Fprintf(hc.Stderr, "Are you sure you want to continue connecting (yes/no)? ")

		line, _ := bufio.NewReader(hc.Stdin).ReadString('\n')
		if strings.ToLower(strings.TrimSpace(line)) != "yes" {
			return fmt.Errorf("ssh: host key verification rejected by user")
		}

		// Persist to known_hosts in the virtual FS.
		if vfs != nil {
			_ = vfs.MkdirAll("/.memsh", 0o700)
			entry := []byte(hostname + " " + string(gossh.MarshalAuthorizedKey(key)))
			existing, _ := afero.ReadFile(vfs, knownHostsPath)
			_ = afero.WriteFile(vfs, knownHostsPath, append(existing, entry...), 0o600)
		}
		fmt.Fprintf(hc.Stderr, "Warning: Permanently added '%s' (%s) to the list of known hosts.\n",
			hostname, key.Type())
		return nil
	}
}
