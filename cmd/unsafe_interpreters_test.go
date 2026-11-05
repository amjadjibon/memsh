package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
)

// setUnsafeInterps sets the package-level --unsafe-interpreters flag value for
// the duration of a test and points HOME at an empty dir so no real config is read.
func setUnsafeInterps(t *testing.T, v bool) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	prev := unsafeInterps
	unsafeInterps = v
	t.Cleanup(func() { unsafeInterps = prev })
}

func TestServeRefusesUnsafeInterpretersWithoutAPIKey(t *testing.T) {
	setUnsafeInterps(t, true)

	err := runServe(serveCmd, nil)
	if err == nil {
		t.Fatal("runServe returned nil, want refusal when --unsafe-interpreters is set without --api-key")
	}
	if !strings.Contains(err.Error(), "--unsafe-interpreters requires --api-key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServeRefusesUnsafeInterpretersFromConfigWithoutAPIKey(t *testing.T) {
	setUnsafeInterps(t, false)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".memsh"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[shell]\nunsafe_interpreters = true\n"
	if err := os.WriteFile(filepath.Join(home, ".memsh", "config.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runServe(serveCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--unsafe-interpreters requires --api-key") {
		t.Fatalf("runServe error = %v, want refusal for config-enabled unsafe interpreters", err)
	}
}

func TestBuildShellOptsGatesInterpreters(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flag    bool
		config  bool
		wantLua bool
	}{
		{"default", false, false, false},
		{"flag", true, false, true},
		{"config", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setUnsafeInterps(t, tc.flag)
			cfg := defaultConfig()
			cfg.Shell.WASM = false
			cfg.Shell.UnsafeInterpreters = tc.config

			s, err := shell.New(buildShellOpts(cfg)...)
			if err != nil {
				t.Fatalf("shell.New: %v", err)
			}
			t.Cleanup(func() { s.Close() })

			hasLua := false
			for _, c := range s.Commands() {
				if c == "lua" {
					hasLua = true
				}
			}
			if hasLua != tc.wantLua {
				t.Fatalf("lua registered = %v, want %v", hasLua, tc.wantLua)
			}
		})
	}
}
