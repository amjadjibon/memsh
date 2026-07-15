package config_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/internal/config"
	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestLoadReturnsDefaultsWhenConfigMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !cfg.Shell.WASM {
		t.Fatal("Shell.WASM = false, want default true")
	}
	if len(cfg.Plugins.WASM) != 0 {
		t.Fatalf("Plugins.WASM = %v, want empty", cfg.Plugins.WASM)
	}
	if len(cfg.Plugins.Disable) != 0 {
		t.Fatalf("Plugins.Disable = %v, want empty", cfg.Plugins.Disable)
	}
}

func TestLoadReadsConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, `
[shell]
wasm = false

[plugins]
wasm = ["python", "ruby"]
disable = ["wc", "curl"]
`)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Shell.WASM {
		t.Fatal("Shell.WASM = true, want false")
	}
	if strings.Join(cfg.Plugins.WASM, ",") != "python,ruby" {
		t.Fatalf("Plugins.WASM = %v, want [python ruby]", cfg.Plugins.WASM)
	}
	if strings.Join(cfg.Plugins.Disable, ",") != "wc,curl" {
		t.Fatalf("Plugins.Disable = %v, want [wc curl]", cfg.Plugins.Disable)
	}
}

func TestLoadReturnsContextForInvalidConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[shell\n")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load returned nil error for invalid TOML")
	}
	if !strings.Contains(err.Error(), "config ") {
		t.Fatalf("error = %q, want config path context", err)
	}
}

func TestBuildShellOptsDisablesConfiguredPlugins(t *testing.T) {
	cfg := config.Config{
		Shell: config.ShellConfig{WASM: false},
		Plugins: config.PluginsConfig{
			WASM:    []string{"python"},
			Disable: []string{"wc"},
		},
	}
	var out bytes.Buffer
	opts := append([]shell.Option{shell.WithStdIO(strings.NewReader(""), &out, &out)}, config.BuildShellOpts(cfg)...)
	s, err := shell.New(opts...)
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	commands := s.Commands()
	if containsCommand(commands, "wc") {
		t.Fatal("wc command is registered, want disabled")
	}
	if !containsCommand(commands, "cat") {
		t.Fatal("cat command missing, want unrelated builtins preserved")
	}

	err = s.Run(context.Background(), "wc")
	if err == nil {
		t.Fatal("Run(wc) returned nil error, want command not found")
	}
	if !strings.Contains(err.Error(), "command not found") {
		t.Fatalf("Run(wc) error = %q, want command not found", err)
	}
}

func writeConfig(t *testing.T, home, data string) {
	t.Helper()
	dir := filepath.Join(home, ".memsh")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func containsCommand(commands []string, want string) bool {
	for _, command := range commands {
		if command == want {
			return true
		}
	}
	return false
}
