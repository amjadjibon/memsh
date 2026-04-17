package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/amjadjibon/memsh/internal/session"
	"github.com/amjadjibon/memsh/pkg/shell"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Save and load memsh session snapshots",
}

var snapshotSaveCmd = &cobra.Command{
	Use:   "save <file>",
	Short: "Run a script and save the resulting virtual FS to a JSON snapshot",
	Long: `Run an optional script, then serialize the entire virtual filesystem
to a JSON file.  The snapshot can later be restored with 'memsh snapshot load'.

Example:
    memsh snapshot save my.json -c 'echo hello > /greeting.txt'
	memsh snapshot save my.json    # saves an empty root filesystem`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outFile := args[0]
		script, _ := cmd.Flags().GetString("command")

		fs := afero.NewMemMapFs()
		sh, err := shell.New(
			shell.WithFS(fs),
			shell.WithStdIO(os.Stdin, os.Stdout, os.Stderr),
		)
		if err != nil {
			return err
		}
		defer sh.Close()

		if script != "" {
			if runErr := sh.Run(context.Background(), script); runErr != nil {
				return fmt.Errorf("snapshot save: script failed: %w", runErr)
			}
		}

		snap, err := shell.TakeSnapshot(fs, sh.Cwd())
		if err != nil {
			return fmt.Errorf("snapshot save: %w", err)
		}
		data, err := shell.MarshalSnapshot(snap)
		if err != nil {
			return err
		}
		if err := os.WriteFile(outFile, data, 0o644); err != nil {
			return fmt.Errorf("snapshot save: write %s: %w", outFile, err)
		}
		fmt.Fprintf(os.Stderr, "memsh: snapshot saved to %s (%d files, cwd=%s)\n",
			outFile, len(snap.Files), snap.Cwd)
		return nil
	},
}

var snapshotLoadCmd = &cobra.Command{
	Use:   "load <file>",
	Short: "Restore a snapshot and start an interactive shell (or run a script)",
	Long: `Restore a virtual filesystem from a JSON snapshot file, then start
an interactive REPL or run an optional script with -c.

Example:
  memsh snapshot load my.json
  memsh snapshot load my.json -c 'ls /'`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		inFile := args[0]
		script, _ := cmd.Flags().GetString("command")

		data, err := os.ReadFile(inFile)
		if err != nil {
			return fmt.Errorf("snapshot load: read %s: %w", inFile, err)
		}
		snap, err := shell.UnmarshalSnapshot(data)
		if err != nil {
			return err
		}
		fs, cwd, err := shell.RestoreSnapshot(snap)
		if err != nil {
			return fmt.Errorf("snapshot load: restore: %w", err)
		}
		fmt.Fprintf(os.Stderr, "memsh: snapshot loaded from %s (%d files, cwd=%s)\n",
			inFile, len(snap.Files), cwd)

		sh, err := shell.New(
			shell.WithFS(fs),
			shell.WithCwd(cwd),
			shell.WithStdIO(os.Stdin, os.Stdout, os.Stderr),
		)
		if err != nil {
			return err
		}
		defer sh.Close()

		ctx := context.Background()
		if script != "" {
			if runErr := sh.Run(ctx, script); runErr != nil {
				return runErr
			}
			return nil
		}

		// Interactive REPL with the restored filesystem.
		limits := session.Limits{}
		var runtimeUsed time.Duration
		if !isInteractiveTerm() {
			return runPiped(ctx, sh, os.Stdin, limits, &runtimeUsed)
		}
		return runInteractive(ctx, sh, limits, &runtimeUsed)
	},
}

var snapshotInspectCmd = &cobra.Command{
	Use:   "inspect <file>",
	Short: "Show the contents of a snapshot (file list + cwd)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("snapshot inspect: %w", err)
		}
		snap, err := shell.UnmarshalSnapshot(data)
		if err != nil {
			return err
		}
		fmt.Printf("version: %d\n", snap.Version)
		fmt.Printf("cwd:     %s\n", snap.Cwd)
		fmt.Printf("files:   %d\n\n", len(snap.Files))
		for _, f := range snap.Files {
			kind := "file"
			if f.IsDir {
				kind = "dir "
			}
			size := ""
			if !f.IsDir {
				size = fmt.Sprintf("  (%d bytes)", len(f.Content))
			}
			fmt.Printf("  %s  %s  %s%s\n", kind, f.Mode, f.Path, size)
		}
		return nil
	},
}

func isInteractiveTerm() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func init() {
	snapshotSaveCmd.Flags().StringP("command", "c", "", "Script to run before saving")
	snapshotLoadCmd.Flags().StringP("command", "c", "", "Script to run after loading (non-interactive)")

	snapshotCmd.AddCommand(snapshotSaveCmd, snapshotLoadCmd, snapshotInspectCmd)
	rootCmd.AddCommand(snapshotCmd)
}
