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

// checkRemoteURL ensures a git remote URL is allowed by the network dialer
// before go-git opens its own transport (covers non-HTTP schemes and preflight).
func checkRemoteURL(ctx context.Context, dial dialFunc, rawURL string) error {
	if dial == nil {
		return fmt.Errorf("network dialer not configured")
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("empty remote URL")
	}
	// Local paths / file:// stay inside the virtual FS path of go-git; no host dial.
	if strings.HasPrefix(rawURL, "/") || strings.HasPrefix(rawURL, "./") || strings.HasPrefix(rawURL, "../") ||
		strings.HasPrefix(rawURL, "file://") {
		return nil
	}

	host, port, err := remoteHostPort(rawURL)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(host, port)
	conn, err := dial(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("network policy: %w", err)
	}
	_ = conn.Close()
	return nil
}

func remoteHostPort(rawURL string) (host, port string, err error) {
	// scp-like: git@host:path/repo.git
	if !strings.Contains(rawURL, "://") {
		if at := strings.Index(rawURL, "@"); at >= 0 {
			rest := rawURL[at+1:]
			if colon := strings.Index(rest, ":"); colon >= 0 {
				return rest[:colon], "22", nil
			}
		}
		return "", "", fmt.Errorf("unsupported git remote URL %q", rawURL)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid git remote URL: %w", err)
	}
	host = u.Hostname()
	if host == "" {
		return "", "", fmt.Errorf("git remote URL missing host: %q", rawURL)
	}
	port = u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		case "ssh", "git":
			port = "22"
		default:
			port = "443"
		}
	}
	return host, port, nil
}
