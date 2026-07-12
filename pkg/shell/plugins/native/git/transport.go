package git

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitclient "github.com/go-git/go-git/v5/plumbing/transport/client"
)

// dialFunc is the shell's policy-enforced network dialer.
type dialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// gitTransportMu serialises temporary InstallProtocol swaps so concurrent
// shells with different policies do not race on the global protocol table.
var gitTransportMu sync.Mutex

// withPolicyHTTPTransport installs a go-git HTTP client that dials through
// the shell network dialer for the duration of fn.
func withPolicyHTTPTransport(dial dialFunc, fn func() error) error {
	if dial == nil {
		return fmt.Errorf("network dialer not configured")
	}
	gitTransportMu.Lock()
	defer gitTransportMu.Unlock()

	prevHTTP := gitclient.Protocols["http"]
	prevHTTPS := gitclient.Protocols["https"]

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
	}()

	return fn()
}

// checkRemoteURL validates a git remote before network I/O.
// Local paths are allowed. SSH/scp remotes are rejected (go-git's SSH
// transport cannot be scoped to NetworkDialContext without a global install).
// HTTP(S) remotes are enforced by withPolicyHTTPTransport — no preflight dial.
func checkRemoteURL(_ context.Context, dial dialFunc, rawURL string) error {
	if dial == nil {
		return fmt.Errorf("network dialer not configured")
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("empty remote URL")
	}
	// Local paths / file:// stay inside the virtual FS; no host dial.
	if strings.HasPrefix(rawURL, "/") || strings.HasPrefix(rawURL, "./") || strings.HasPrefix(rawURL, "../") ||
		strings.HasPrefix(rawURL, "file://") {
		return nil
	}

	// scp-like: git@host:path/repo.git
	if !strings.Contains(rawURL, "://") {
		if strings.Contains(rawURL, "@") && strings.Contains(rawURL, ":") {
			return fmt.Errorf("ssh remotes are disabled; use https:// instead")
		}
		// Bare relative path into virtual FS.
		return nil
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
		return nil
	default:
		return fmt.Errorf("unsupported git remote scheme %q; use https://", u.Scheme)
	}
}
