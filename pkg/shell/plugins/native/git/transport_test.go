package git

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
	gitclient "github.com/go-git/go-git/v5/plumbing/transport/client"
)

func TestCheckRemoteURL(t *testing.T) {
	dial := func(ctx context.Context, network, address string) (net.Conn, error) { return nil, nil }

	cases := []struct {
		name    string
		dial    dialFunc
		url     string
		wantErr string
	}{
		{"nil dialer", nil, "https://example.com/repo.git", "network dialer not configured"},
		{"empty url", dial, "", "empty remote URL"},
		{"absolute path", dial, "/repo.git", "local remotes are not supported"},
		{"relative dot path", dial, "./repo.git", "local remotes are not supported"},
		{"relative dotdot path", dial, "../repo.git", "local remotes are not supported"},
		{"file scheme prefix", dial, "file:///repo.git", "local remotes are not supported"},
		{"scp-like ssh", dial, "git@github.com:org/repo.git", "ssh remotes are disabled"},
		{"bare relative path", dial, "repo.git", "local remotes are not supported"},
		{"https ok", dial, "https://github.com/org/repo.git", ""},
		{"http ok", dial, "http://github.com/org/repo.git", ""},
		{"https missing host", dial, "https:///repo.git", "missing host"},
		{"ssh scheme", dial, "ssh://git@github.com/org/repo.git", "ssh remotes are disabled"},
		{"git scheme", dial, "git://github.com/org/repo.git", "ssh remotes are disabled"},
		{"file url scheme uppercase", dial, "FILE:///repo.git", "local remotes are not supported"},
		{"unsupported scheme", dial, "ftp://example.com/repo.git", "unsupported git remote scheme"},
		{"invalid url", dial, "https://[::1", "invalid git remote URL"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkRemoteURL(context.Background(), c.dial, c.url)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("expected error containing %q, got %v", c.wantErr, err)
			}
		})
	}
}

func TestWithPolicyHTTPTransport(t *testing.T) {
	t.Run("nil dialer errors", func(t *testing.T) {
		err := withPolicyHTTPTransport(nil, func() error { return nil })
		if err == nil {
			t.Fatal("expected error for nil dialer")
		}
	})

	t.Run("runs fn and restores previous protocols", func(t *testing.T) {
		dial := func(ctx context.Context, network, address string) (net.Conn, error) { return nil, nil }
		called := false
		wantErr := errors.New("boom")
		err := withPolicyHTTPTransport(dial, func() error {
			called = true
			return wantErr
		})
		if !called {
			t.Fatal("expected fn to be called")
		}
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected fn error to propagate, got %v", err)
		}
	})

	t.Run("disables host-backed protocols during fn and restores them", func(t *testing.T) {
		dial := func(ctx context.Context, network, address string) (net.Conn, error) { return nil, nil }
		before := map[string]transport.Transport{}
		for _, p := range sandboxedProtocols {
			before[p] = gitclient.Protocols[p]
		}
		err := withPolicyHTTPTransport(dial, func() error {
			for _, p := range sandboxedProtocols {
				if _, ok := gitclient.Protocols[p].(disabledTransport); !ok {
					t.Errorf("protocol %q not disabled during operation", p)
				}
			}
			ep, _ := transport.NewEndpoint("file:///tmp/x.git")
			if _, err := gitclient.Protocols["file"].NewUploadPackSession(ep, nil); !errors.Is(err, errTransportDisabled) {
				t.Errorf("file upload-pack session err = %v, want errTransportDisabled", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range sandboxedProtocols {
			if gitclient.Protocols[p] != before[p] {
				t.Errorf("protocol %q not restored", p)
			}
		}
	})
}
