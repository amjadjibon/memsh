package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
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

		// Non-interactive: pipe input line by line without readline.
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return runPiped(ctx, sh, os.Stdin)
		}

		// Interactive REPL with tab completion and history.
		return runInteractive(ctx, sh)
	},
}

// runInteractive runs the readline-powered REPL.
func runInteractive(ctx context.Context, sh *shell.Shell) error {
	histFile := historyFile()

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          prompt(sh),
		AutoComplete:    &replCompleter{sh: sh},
		HistoryFile:     histFile,
		InterruptPrompt: "^C",
		EOFPrompt:       "",
	})
	if err != nil {
		return fmt.Errorf("readline: %w", err)
	}
	defer rl.Close()

	fmt.Println("memsh - virtual in-memory shell. Type 'exit' or press Ctrl+D to quit.")

	for {
		rl.SetPrompt(prompt(sh))

		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			}
			continue
		}
		if err == io.EOF {
			break
		}

		line = strings.TrimSpace(line)
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

	fmt.Println()
	return nil
}

// runPiped reads commands from r line by line (no prompt, no readline).
func runPiped(ctx context.Context, sh *shell.Shell, r io.Reader) error {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		for {
			idx := strings.Index(string(buf), "\n")
			if idx < 0 {
				break
			}
			line := strings.TrimSpace(string(buf[:idx]))
			buf = buf[idx+1:]
			if line == "" {
				continue
			}
			first := strings.Fields(line)[0]
			if first == "exit" || first == "quit" {
				return nil
			}
			if runErr := sh.Run(ctx, line); runErr != nil {
				if errors.Is(runErr, shell.ErrExit) {
					return nil
				}
				fmt.Fprintf(os.Stderr, "memsh: %v\n", runErr)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func prompt(sh *shell.Shell) string {
	return fmt.Sprintf("memsh:%s$ ", sh.Cwd())
}

func historyFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".memsh")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "history")
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "memsh: %v\n", err)
		os.Exit(1)
	}
}
