package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the structure of ~/.memsh/config.toml.
type Config struct {
	Shell   ShellConfig   `toml:"shell"`
	Plugins PluginsConfig `toml:"plugins"`
}

// ShellConfig controls core shell behaviour.
type ShellConfig struct {
	// WASM enables or disables the wazero WASM plugin runtime.
	// Set to false to skip all WASM plugin loading (faster startup).
	// Default: true.
	WASM bool `toml:"wasm"`
}

// PluginsConfig controls which plugins are loaded.
type PluginsConfig struct {
	// WASM is an allowlist of WASM plugin names to load from ~/.memsh/plugins/.
	// When empty all discovered .wasm files are loaded.
	// Example: wasm = ["python", "ruby"]
	WASM []string `toml:"wasm"`

	// Disable is a list of plugin names (native or WASM) to exclude.
	// Example: disable = ["wc"]
	Disable []string `toml:"disable"`
}

func defaultConfig() Config {
	return Config{
		Shell: ShellConfig{WASM: true},
	}
}

// loadConfig reads ~/.memsh/config.toml.
// A missing file is not an error — defaults are returned silently.
func loadConfig() (Config, error) {
	cfg := defaultConfig()

	path, err := configFilePath()
	if err != nil {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

func configFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".memsh", "config.toml"), nil
}
