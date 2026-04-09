// Package paths provides utilities for resolving standard memsh filesystem paths.
// All paths are relative to the user's home directory (~/.memsh or ~/.memshrc).
// Functions that create directories will create them with mode 0755 if they don't exist.
package paths

import (
	"os"
	"path/filepath"
)

// MemshDir returns the ~/.memsh directory path, creating it if needed.
// Returns an error if the home directory cannot be determined or if directory creation fails.
func MemshDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".memsh")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// ConfigFile returns the path to ~/.memsh/config.toml.
// The directory will NOT be created. Returns an error if the home directory cannot be determined.
func ConfigFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".memsh", "config.toml"), nil
}

// MemshrcFile returns the path to ~/.memshrc.
// The file will NOT be created. Returns an error if the home directory cannot be determined.
func MemshrcFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".memshrc"), nil
}

// HistoryDir returns the path to ~/.memsh/history, creating it if needed.
// Returns an error if the home directory cannot be determined or if directory creation fails.
func HistoryDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".memsh", "history")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// PluginDir returns the path to ~/.memsh/plugins, creating it if needed.
// Returns an error if the home directory cannot be determined or if directory creation fails.
func PluginDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".memsh", "plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// SSHHostKeyFile returns the path to ~/.memsh/ssh_host_key.
// The file will NOT be created. Returns an error if the home directory cannot be determined.
func SSHHostKeyFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".memsh", "ssh_host_key"), nil
}
