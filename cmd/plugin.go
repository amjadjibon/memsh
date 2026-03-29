package cmd

import (
	"fmt"
	"io"
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
		builtins := shell.BuiltinPluginNames()
		sort.Strings(builtins)
		fmt.Println("built-in:")
		for _, name := range builtins {
			fmt.Printf("  %s\n", name)
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

		var installed []string
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".wasm" {
				installed = append(installed, strings.TrimSuffix(e.Name(), ".wasm"))
			}
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
	Short: "Install a WASM plugin",
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

func init() {
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
