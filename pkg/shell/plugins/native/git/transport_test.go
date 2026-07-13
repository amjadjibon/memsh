package git

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
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
		{"absolute path", dial, "/repo.git", ""},
		{"relative dot path", dial, "./repo.git", ""},
		{"relative dotdot path", dial, "../repo.git", ""},
		{"file scheme prefix", dial, "file:///repo.git", ""},
		{"scp-like ssh", dial, "git@github.com:org/repo.git", "ssh remotes are disabled"},
		{"bare relative path", dial, "repo.git", ""},
		{"https ok", dial, "https://github.com/org/repo.git", ""},
		{"http ok", dial, "http://github.com/org/repo.git", ""},
		{"https missing host", dial, "https:///repo.git", "missing host"},
		{"ssh scheme", dial, "ssh://git@github.com/org/repo.git", "ssh remotes are disabled"},
		{"git scheme", dial, "git://github.com/org/repo.git", "ssh remotes are disabled"},
		{"file url scheme", dial, "file:///repo.git", ""},
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
}
