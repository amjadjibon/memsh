package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/amjadjibon/memsh/pkg/shell"
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
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}

		destName := filepath.Base(src)
		if filepath.Ext(destName) != ".wasm" {
			return fmt.Errorf("plugin file must have a .wasm extension: %s", src)
		}
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()

		dest := filepath.Join(dir, destName)
		if _, err := writeWasmAtomically(in, dest, "", maxPluginSize); err != nil {
			return err
		}

		name := strings.TrimSuffix(destName, ".wasm")
		fmt.Printf("installed plugin %q → %s\n", name, dest)
		return nil
	},
}

// maxPluginSize caps a downloaded or installed .wasm file.
const maxPluginSize = 256 << 20

// pluginDownloadClient bounds how long a runtime download may take.
var pluginDownloadClient = &http.Client{Timeout: 15 * time.Minute}

// plugin runtime installers. sha256 pins each release asset; a download that
// does not match is discarded.
var runtimeInstallers = map[string]struct {
	url      string
	name     string
	destFile string
	sha256   string
}{
	"python": {
		url:      "https://github.com/vmware-labs/webassembly-language-runtimes/releases/download/python%2F3.12.0%2B20231211-040d5a6/python-3.12.0.wasm",
		name:     "Python 3.12",
		destFile: "python.wasm",
		sha256:   "e5dc5a398b07b54ea8fdb503bf68fb583d533f10ec3f930963e02b9505f7a763",
	},
	"ruby": {
		url:      "https://github.com/vmware-labs/webassembly-language-runtimes/releases/download/ruby%2F3.2.2%2B20230714-11be424/ruby-3.2.2-slim.wasm",
		name:     "Ruby 3.2.2 (slim)",
		destFile: "ruby.wasm",
		sha256:   "de598f394e398763d2b147e3e51a6eeadf048128598ac4a3f992a97204c192b0",
	},
	"php": {
		url:      "https://github.com/vmware-labs/webassembly-language-runtimes/releases/download/php%2F8.2.6%2B20230714-11be424/php-cgi-8.2.6-slim.wasm",
		name:     "PHP 8.2.6 (slim)",
		destFile: "php.wasm",
		sha256:   "4fd2e8c42ae529ba72f88a0f1e46de1fc69a4b4f01e01fedd65ca966b8ffe6fa",
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

var pluginInstallPhpCmd = &cobra.Command{
	Use:   "php",
	Short: "Install PHP 8.2.6 (slim) WASM runtime",
	RunE: func(cmd *cobra.Command, args []string) error {
		return installRuntime("php")
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	dest := filepath.Join(dir, runtime.destFile)

	// Already installed and intact? A partial or tampered file is replaced.
	if sum, err := fileSHA256(dest); err == nil {
		if sum == runtime.sha256 {
			fmt.Printf("%s is already installed at %s\n", runtime.name, dest)
			return nil
		}
		fmt.Printf("%s at %s does not match the pinned checksum; reinstalling\n", runtime.name, dest)
	}

	fmt.Printf("Downloading %s WASM → %s ...\n", runtime.name, dest)

	if err := installWasmDirect(pluginDownloadClient, runtime.url, dest, runtime.sha256); err != nil {
		return fmt.Errorf("failed to download %s: %w", runtime.name, err)
	}

	fmt.Printf("Done. Start memsh and run: %s -c \"print('hello')\"\n", lang)
	return nil
}

// installWasmDirect downloads a .wasm file, verifies its SHA-256 against
// wantSHA256, and only then moves it into place.
func installWasmDirect(client *http.Client, url, dest, wantSHA256 string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	_, err = writeWasmAtomically(resp.Body, dest, wantSHA256, maxPluginSize)
	return err
}

// writeWasmAtomically streams r into a temp file next to dest, checks the
// size cap, the WASM magic number and (if wantSHA256 is set) the checksum,
// then renames it over dest. On any failure dest is left untouched.
func writeWasmAtomically(r io.Reader, dest, wantSHA256 string, maxSize int64) (string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(dest), "."+filepath.Base(dest)+".*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, maxSize+1))
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if n > maxSize {
		return "", fmt.Errorf("plugin exceeds %d bytes", maxSize)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if wantSHA256 != "" && !strings.EqualFold(sum, wantSHA256) {
		return "", fmt.Errorf("checksum mismatch: got sha256 %s, want %s", sum, wantSHA256)
	}
	if err := checkWasmMagic(tmpName); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", err
	}
	return sum, nil
}

func checkWasmMagic(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil || string(magic) != "\x00asm" {
		return fmt.Errorf("not a WebAssembly module")
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func init() {
	pluginInstallCmd.AddCommand(pluginInstallPythonCmd, pluginInstallRubyCmd, pluginInstallPhpCmd)
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
