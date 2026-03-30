package cmd

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/amjadjibon/memsh/shell"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage memsh plugins",
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed plugins",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := loadConfig()

		builtins := shell.BuiltinPluginNames()
		sort.Strings(builtins)
		fmt.Println("built-in:")
		for _, name := range builtins {
			fmt.Printf("  %s\n", name)
		}

		if !cfg.Shell.WASM {
			fmt.Println("wasm: disabled (set [shell] wasm = true in ~/.memsh/config.toml to enable)")
			return nil
		}

		dir, err := pluginDir()
		if err != nil {
			return err
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}

		// Build allowlist from config (empty = show all).
		allowlist := make(map[string]bool, len(cfg.Plugins.WASM))
		for _, n := range cfg.Plugins.WASM {
			allowlist[n] = true
		}
		disabled := make(map[string]bool, len(cfg.Plugins.Disable))
		for _, n := range cfg.Plugins.Disable {
			disabled[n] = true
		}

		var installed []string
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".wasm" {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".wasm")
			if disabled[name] {
				continue
			}
			if len(allowlist) > 0 && !allowlist[name] {
				continue
			}
			installed = append(installed, name)
		}
		if len(installed) > 0 {
			fmt.Println("installed:")
			for _, name := range installed {
				fmt.Printf("  %s\n", name)
			}
		}
		return nil
	},
}

var pluginInstallCmd = &cobra.Command{
	Use:   "install <plugin.wasm>",
	Short: "Install a WASM plugin from a local file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		src := args[0]

		dir, err := pluginDir()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}

		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()

		destName := filepath.Base(src)
		dest := filepath.Join(dir, destName)
		out, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer out.Close()

		if _, err := io.Copy(out, in); err != nil {
			return err
		}

		name := strings.TrimSuffix(destName, ".wasm")
		fmt.Printf("installed plugin %q → %s\n", name, dest)
		return nil
	},
}

// plugin runtime installers
var runtimeInstallers = map[string]struct {
	url      string
	name     string
	destFile string
	install  func(string, string) error
}{
	"python": {
		url:      "https://github.com/vmware-labs/webassembly-language-runtimes/releases/download/python%2F3.12.0%2B20231211-040d5a6/python-3.12.0.wasm",
		name:     "Python 3.12",
		destFile: "python.wasm",
		install:  installWasmDirect,
	},
	"ruby": {
		url:      "https://github.com/ruby/ruby.wasm/releases/download/2.9.0/ruby-3.2-wasm32-unknown-wasip1-minimal.tar.gz",
		name:     "Ruby 3.2",
		destFile: "ruby.wasm",
		install:  installRubyTarGz,
	},
}

var pluginInstallPythonCmd = &cobra.Command{
	Use:   "python",
	Short: "Install Python 3.12 WASM runtime",
	RunE: func(cmd *cobra.Command, args []string) error {
		return installRuntime("python")
	},
}

var pluginInstallRubyCmd = &cobra.Command{
	Use:   "ruby",
	Short: "Install Ruby 3.2 WASM runtime",
	RunE: func(cmd *cobra.Command, args []string) error {
		return installRuntime("ruby")
	},
}

func installRuntime(lang string) error {
	runtime, ok := runtimeInstallers[lang]
	if !ok {
		return fmt.Errorf("unknown runtime: %s", lang)
	}

	dir, err := pluginDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	dest := filepath.Join(dir, runtime.destFile)

	// Check if already installed
	if _, err := os.Stat(dest); err == nil {
		fmt.Printf("%s is already installed at %s\n", runtime.name, dest)
		return nil
	}

	fmt.Printf("Downloading %s WASM → %s ...\n", runtime.name, dest)

	if err := runtime.install(runtime.url, dest); err != nil {
		return fmt.Errorf("failed to download %s: %w", runtime.name, err)
	}

	fmt.Printf("Done. Start memsh and run: %s -c \"print('hello')\"\n", lang)
	return nil
}

// installWasmDirect downloads a .wasm file directly
func installWasmDirect(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// installRubyTarGz downloads and extracts ruby.wasm from a tar.gz
func installRubyTarGz(url, dest string) error {
	// Download to temp file
	tmpFile, err := os.CreateTemp("", "ruby-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return err
	}
	tmpFile.Close()

	// Extract using tar command
	tmpDir, err := os.MkdirTemp("", "ruby-extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// Use system tar to extract
	if err := extractTarGz(tmpFile.Name(), tmpDir); err != nil {
		return err
	}

	// Find and copy the ruby binary
	rubyPath := filepath.Join(tmpDir, "ruby-3.2-wasm32-unknown-wasip1-minimal", "usr", "local", "bin", "ruby")
	if _, err := os.Stat(rubyPath); os.IsNotExist(err) {
		return fmt.Errorf("ruby binary not found in extracted archive")
	}

	in, err := os.Open(rubyPath)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// extractTarGz extracts a tar.gz file using Go's archive/tar and compress/gzip
func extractTarGz(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}

			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}

	return nil
}

func init() {
	pluginInstallCmd.AddCommand(pluginInstallPythonCmd, pluginInstallRubyCmd)
	pluginCmd.AddCommand(pluginListCmd, pluginInstallCmd)
	rootCmd.AddCommand(pluginCmd)
}

func pluginDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".memsh", "plugins"), nil
}
