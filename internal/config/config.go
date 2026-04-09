// Package config handles loading and parsing the memsh configuration file
// (~/.memsh/config.toml) and converting the configuration into shell.Option
// slices for use with shell.New().
//
// A missing config file is not an error - sensible defaults are returned.
package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/amjadjibon/memsh/internal/paths"
	"github.com/amjadjibon/memsh/pkg/shell"
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

// Load reads ~/.memsh/config.toml.
// A missing file is not an error — defaults are returned silently.
func Load() (Config, error) {
	cfg := defaultConfig()

	path, err := paths.ConfigFile()
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

// BuildShellOpts converts the loaded config into shell options.
func BuildShellOpts(cfg Config) []shell.Option {
	var opts []shell.Option
	if !cfg.Shell.WASM {
		opts = append(opts, shell.WithWASMEnabled(false))
	}
	if len(cfg.Plugins.WASM) > 0 {
		opts = append(opts, shell.WithPluginFilter(cfg.Plugins.WASM))
	}
	if len(cfg.Plugins.Disable) > 0 {
		opts = append(opts, shell.WithDisabledPlugins(cfg.Plugins.Disable...))
	}
	return opts
}
