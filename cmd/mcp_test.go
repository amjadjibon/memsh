package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestMCPHandlerRequiresAPIKey(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := mcpHandler(inner, "secret")

	for _, tc := range []struct {
		auth string
		want int
	}{
		{"", http.StatusUnauthorized},
		{"Bearer wrong", http.StatusForbidden},
		{"Bearer secret", http.StatusNoContent},
	} {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		if tc.auth != "" {
			req.Header.Set("Authorization", tc.auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("auth %q: status %d, want %d", tc.auth, rec.Code, tc.want)
		}
	}

	if mcpHandler(inner, "") == nil {
		t.Fatal("mcpHandler without key returned nil")
	}
}

func TestMCPShellOptsDenyPrivateNetworks(t *testing.T) {
	setUnsafeInterps(t, false)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("internal-only"))
	}))
	defer target.Close()

	opts, err := mcpShellOpts(mcpConfig{transport: "http"})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	sh, err := shell.New(append(opts, shell.WithStdIO(strings.NewReader(""), &buf, &buf))...)
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()

	_ = sh.Run(context.Background(), "curl -s "+target.URL)
	if strings.Contains(buf.String(), "internal-only") {
		t.Fatalf("MCP shell reached a loopback service: %q", buf.String())
	}
}

func TestMCPUnsafeInterpretersRequireAPIKeyOverHTTP(t *testing.T) {
	setUnsafeInterps(t, true)

	for _, transport := range []string{"http", "sse"} {
		if _, err := mcpShellOpts(mcpConfig{transport: transport}); err == nil ||
			!strings.Contains(err.Error(), "requires --api-key") {
			t.Errorf("%s: err = %v, want --api-key requirement", transport, err)
		}
	}
	if _, err := mcpShellOpts(mcpConfig{transport: "http", apiKey: "k"}); err != nil {
		t.Errorf("http with api key: %v", err)
	}
	if _, err := mcpShellOpts(mcpConfig{transport: "stdio"}); err != nil {
		t.Errorf("stdio: %v", err)
	}
}

func TestMCPDefaultAddrIsLoopback(t *testing.T) {
	addr, _ := mcpCmd.Flags().GetString("addr")
	if !isLoopbackAddr(addr) {
		t.Fatalf("default --addr %q is not loopback", addr)
	}
}
