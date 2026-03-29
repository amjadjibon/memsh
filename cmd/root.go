package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/amjadjibon/memsh/shell"
)

var rootCmd = &cobra.Command{
	Use:   "memsh [script]",
	Short: "memsh - virtual in-memory shell",
	Long:  "memsh is a virtual in-memory shell. Run with no arguments to start an interactive REPL, or pass a script file to execute it.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		sh, err := shell.New()
		if err != nil {
			return fmt.Errorf("failed to start: %w", err)
		}

		// Script file mode: memsh ./hello.sh
		if len(args) > 0 {
			src, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			if err := sh.Run(ctx, string(src)); err != nil && !errors.Is(err, shell.ErrExit) {
				return err
			}
			return nil
		}

		// Interactive REPL mode
		isTTY := term.IsTerminal(int(os.Stdin.Fd()))
		if isTTY {
			fmt.Println("memsh - virtual in-memory shell. Type 'exit' or press Ctrl+D to quit.")
		}

		scanner := bufio.NewScanner(os.Stdin)
		for {
			if isTTY {
				fmt.Fprintf(os.Stderr, "memsh:%s$ ", sh.Cwd())
			}

			if !scanner.Scan() {
				break
			}

			line := scanner.Text()
			if line == "" {
				continue
			}

			first := strings.Fields(line)[0]
			if first == "exit" || first == "quit" {
				break
			}

			if err := sh.Run(ctx, line); err != nil {
				if errors.Is(err, shell.ErrExit) {
					break
				}
				fmt.Fprintf(os.Stderr, "memsh: %v\n", err)
			}
		}

		if isTTY {
			fmt.Println()
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "memsh: %v\n", err)
		os.Exit(1)
	}
}
