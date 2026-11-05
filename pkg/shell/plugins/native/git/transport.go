package git

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5/plumbing/transport"
	gitclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// errTransportDisabled is returned by the stub transports installed for the
// non-HTTP protocols while a sandboxed git operation runs.
var errTransportDisabled = fmt.Errorf("only http:// and https:// git remotes are supported")

// disabledTransport refuses every session. go-git's stock file, ssh and git
// transports reach the host (file:// spawns host git-upload-pack/receive-pack,
// ssh reads host keys/agent, git:// dials without the policy dialer), so they
// are replaced for the duration of each operation.
type disabledTransport struct{}

func (disabledTransport) NewUploadPackSession(*transport.Endpoint, transport.AuthMethod) (transport.UploadPackSession, error) {
	return nil, errTransportDisabled
}

func (disabledTransport) NewReceivePackSession(*transport.Endpoint, transport.AuthMethod) (transport.ReceivePackSession, error) {
	return nil, errTransportDisabled
}

// sandboxedProtocols are the go-git protocols that must never run with their
// default host-backed implementation inside memsh.
var sandboxedProtocols = []string{"file", "ssh", "git"}

// dialFunc is the shell's policy-enforced network dialer.
type dialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// gitTransportMu serialises temporary InstallProtocol swaps so concurrent
// shells with different policies do not race on the global protocol table.
var gitTransportMu sync.Mutex

// withPolicyHTTPTransport installs a go-git HTTP client that dials through
// the shell network dialer for the duration of fn, and disables every other
// protocol so no code path can fall back to a host-backed transport.
func withPolicyHTTPTransport(dial dialFunc, fn func() error) error {
	if dial == nil {
		return fmt.Errorf("network dialer not configured")
	}
	gitTransportMu.Lock()
	defer gitTransportMu.Unlock()

	prevHTTP := gitclient.Protocols["http"]
	prevHTTPS := gitclient.Protocols["https"]
	prevOther := make(map[string]transport.Transport, len(sandboxedProtocols))
	for _, proto := range sandboxedProtocols {
		prevOther[proto] = gitclient.Protocols[proto]
		gitclient.InstallProtocol(proto, disabledTransport{})
	}

	transport := &http.Transport{
		DialContext: dial,
	}
	httpClient := &http.Client{Transport: transport}
	custom := githttp.NewClient(httpClient)
	gitclient.InstallProtocol("http", custom)
	gitclient.InstallProtocol("https", custom)
	defer func() {
		if prevHTTP != nil {
			gitclient.InstallProtocol("http", prevHTTP)
		}
		if prevHTTPS != nil {
			gitclient.InstallProtocol("https", prevHTTPS)
		}
		for proto, prev := range prevOther {
			if prev != nil {
				gitclient.InstallProtocol(proto, prev)
			} else {
				delete(gitclient.Protocols, proto)
			}
		}
	}()

	return fn()
}

// checkRemoteURL validates a git remote before network I/O. Only http(s)
// remotes are allowed: go-git's local/file transport operates on real host
// paths (it spawns host git-upload-pack), not on the virtual FS, and SSH/git
// transports cannot be routed through the policy dialer. HTTP(S) remotes are
// enforced by withPolicyHTTPTransport — no preflight dial.
func checkRemoteURL(_ context.Context, dial dialFunc, rawURL string) error {
	if dial == nil {
		return fmt.Errorf("network dialer not configured")
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("empty remote URL")
	}

	if !strings.Contains(rawURL, "://") {
		// scp-like: git@host:path/repo.git
		if strings.Contains(rawURL, "@") && strings.Contains(rawURL, ":") {
			return fmt.Errorf("ssh remotes are disabled; use https:// instead")
		}
		return fmt.Errorf("local remotes are not supported (%q); use https://", rawURL)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid git remote URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Hostname() == "" {
			return fmt.Errorf("git remote URL missing host: %q", rawURL)
		}
		return nil
	case "ssh", "git":
		return fmt.Errorf("ssh remotes are disabled; use https:// instead")
	case "file":
		return fmt.Errorf("local remotes are not supported (%q); use https://", rawURL)
	default:
		return fmt.Errorf("unsupported git remote scheme %q; use https://", u.Scheme)
	}
}
