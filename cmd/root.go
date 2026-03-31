package cmd

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/amjadjibon/memsh/shell"
)

var (
	execCommand string
	debugMode   bool
)

var rootCmd = &cobra.Command{
	Use:   "memsh [script]",
	Short: "memsh - virtual in-memory shell",
	Long:  "memsh is a virtual in-memory shell. Run with no arguments to start an interactive REPL, or pass a script file to execute it.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		if showVersion, _ := cmd.Flags().GetBool("version"); showVersion {
			fmt.Println(getVersion())
			return nil
		}

		cfg, err := loadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "memsh: config: %v\n", err)
		}

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

		sh, err := shell.New(opts...)
		if err != nil {
			return fmt.Errorf("failed to start: %w", err)
		}

		if execCommand != "" {
			if debugMode {
				fmt.Fprintf(os.Stderr, "memsh: executing: %s\n", execCommand)
			}
			if err := sh.Run(ctx, execCommand); err != nil && !errors.Is(err, shell.ErrExit) {
				return err
			}
			return nil
		}

		if len(args) > 0 {
			src, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			if debugMode {
				fmt.Fprintf(os.Stderr, "memsh: running script: %s\n", args[0])
			}
			if err := sh.Run(ctx, string(src)); err != nil && !errors.Is(err, shell.ErrExit) {
				return err
			}
			return nil
		}

		if !term.IsTerminal(int(os.Stdin.Fd())) {
			if execCommand != "" {
				// When -c is used with piped input, run the command with stdin intact
				if debugMode {
					fmt.Fprintf(os.Stderr, "memsh: executing with piped input: %s\n", execCommand)
				}
				if err := sh.Run(ctx, execCommand); err != nil && !errors.Is(err, shell.ErrExit) {
					return err
				}
				return nil
			}
			return runPiped(ctx, sh, os.Stdin)
		}

		return runInteractive(ctx, sh)
	},
}

func runInteractive(ctx context.Context, sh *shell.Shell) error {
	loadMemshrc(sh, ctx)

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

	var multiline strings.Builder
	inMultiline := false

	for {
		p := prompt(sh)
		if inMultiline {
			p = "> "
		}
		rl.SetPrompt(p)

		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			inMultiline = false
			multiline.Reset()
			continue
		}
		if err == io.EOF {
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if needsContinuation(line) {
			multiline.WriteString(line + "\n")
			inMultiline = true
			continue
		}

		if inMultiline {
			multiline.WriteString(line)
			script := multiline.String()
			multiline.Reset()
			inMultiline = false

			first := strings.Fields(script)[0]
			if first == "exit" || first == "quit" {
				break
			}
			if debugMode {
				fmt.Fprintf(os.Stderr, "memsh: executing multiline script\n")
			}
			if err := runWithSignal(ctx, sh, script); err != nil {
				if errors.Is(err, shell.ErrExit) {
					break
				}
				fmt.Fprintf(os.Stderr, "memsh: %v\n", err)
			}
			continue
		}

		first := strings.Fields(line)[0]
		if first == "exit" || first == "quit" {
			break
		}

		if debugMode {
			fmt.Fprintf(os.Stderr, "memsh: executing: %s\n", line)
		}
		if err := runWithSignal(ctx, sh, line); err != nil {
			if errors.Is(err, shell.ErrExit) {
				break
			}
			fmt.Fprintf(os.Stderr, "memsh: %v\n", err)
		}
	}

	fmt.Println()
	return nil
}

func needsContinuation(line string) bool {
	openCount := strings.Count(line, "{") + strings.Count(line, "(") + strings.Count(line, "[")
	closeCount := strings.Count(line, "}") + strings.Count(line, ")") + strings.Count(line, "]")
	if openCount > closeCount {
		return true
	}

	trimmed := strings.TrimSpace(line)
	if strings.HasSuffix(trimmed, "do") || strings.HasSuffix(trimmed, "then") || strings.HasSuffix(trimmed, "else") || strings.HasSuffix(trimmed, "elif") {
		return true
	}

	words := strings.Fields(trimmed)
	if len(words) > 0 && (words[0] == "for" || words[0] == "while" || words[0] == "until" || words[0] == "if" || words[0] == "case" || words[0] == "function") {
		if !strings.Contains(trimmed, ";") && !strings.Contains(trimmed, "done") && !strings.Contains(trimmed, "fi") && !strings.Contains(trimmed, "esac") {
			return true
		}
	}

	if strings.HasSuffix(trimmed, "\\") {
		return true
	}

	return false
}

func loadMemshrc(sh *shell.Shell, ctx context.Context) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	rcPath := filepath.Join(home, ".memshrc")
	data, err := os.ReadFile(rcPath)
	if err != nil {
		return
	}
	if debugMode {
		fmt.Fprintf(os.Stderr, "memsh: loading %s\n", rcPath)
	}
	if err := sh.Run(ctx, string(data)); err != nil {
		fmt.Fprintf(os.Stderr, "memsh: %s: %v\n", rcPath, err)
	}
}

func runWithSignal(parent context.Context, sh *shell.Shell, script string) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- sh.Run(ctx, script)
	}()

	select {
	case err := <-done:
		return err
	case <-sigCh:
		cancel()
		<-done
		return nil
	}
}

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
	dir := filepath.Join(home, ".memsh", "history")
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		os.Remove(dir)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ""
	}
	ts := fmt.Appendf(nil, "%d", time.Now().UnixNano())
	hash := sha256.Sum256(ts)
	name := fmt.Sprintf("%x", hash)
	return filepath.Join(dir, name)
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.Flags().StringVarP(&execCommand, "command", "c", "", "Execute a command string")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Enable debug output")
	rootCmd.Flags().BoolP("version", "v", false, "Print the version")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "memsh: %v\n", err)
		os.Exit(1)
	}
}

func getVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.tag" && s.Value != "" {
				return "memsh " + s.Value
			}
		}
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return "memsh " + info.Main.Version
		}
	}
	return "memsh dev"
}
