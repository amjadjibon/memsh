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

	"github.com/amjadjibon/memsh/internal/session"
	"github.com/amjadjibon/memsh/pkg/shell"
)

var (
	execCommand     string
	debugMode       bool
	localMaxFiles   int
	localMaxBytes   int64
	localMaxRuntime time.Duration
	localSessionID  string
	localStoreDir   string
	localNetFlags   networkFlagConfig
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
		limits := session.Limits{
			MaxFiles:   localMaxFiles,
			MaxBytes:   localMaxBytes,
			MaxRuntime: localMaxRuntime,
		}
		if limits.MaxFiles < 0 {
			return fmt.Errorf("invalid --max-files: must be >= 0")
		}
		if limits.MaxBytes < 0 {
			return fmt.Errorf("invalid --max-bytes: must be >= 0")
		}
		if limits.MaxRuntime < 0 {
			return fmt.Errorf("invalid --max-runtime: must be >= 0")
		}
		if (localStoreDir == "") != (localSessionID == "") {
			return fmt.Errorf("--session-store and --session-id must be provided together")
		}
		netPolicy, err := parseNetworkPolicy(localNetFlags)
		if err != nil {
			return err
		}
		opts = append(opts, shell.WithNetworkPolicy(netPolicy))

		var runtimeUsed time.Duration
		persistedRcLoaded := false
		if localStoreDir != "" && localSessionID != "" {
			fs, cwd, rt, rcLoaded, loadErr := session.LoadShellSession(localStoreDir, localSessionID)
			if loadErr == nil {
				opts = append(opts, shell.WithFS(fs), shell.WithCwd(cwd))
				runtimeUsed = rt
				persistedRcLoaded = rcLoaded
			} else if !os.IsNotExist(loadErr) {
				return fmt.Errorf("failed to load session %q: %w", localSessionID, loadErr)
			}
		}

		sh, err := shell.New(opts...)
		if err != nil {
			return fmt.Errorf("failed to start: %w", err)
		}
		if localStoreDir != "" && localSessionID != "" {
			session.RestoreAliases(ctx, sh, sh.FS())
			defer func() {
				session.SaveAliases(ctx, sh)
				if saveErr := session.SaveShellSession(localStoreDir, localSessionID, sh.FS(), sh.Cwd(), runtimeUsed, true); saveErr != nil {
					fmt.Fprintf(os.Stderr, "memsh: failed to persist session %q: %v\n", localSessionID, saveErr)
				}
			}()
		}

		if execCommand != "" {
			if debugMode {
				fmt.Fprintf(os.Stderr, "memsh: executing: %s\n", execCommand)
			}
			if err := runWithLocalLimits(ctx, sh, execCommand, limits, &runtimeUsed); err != nil && !errors.Is(err, shell.ErrExit) {
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
			if err := runWithLocalLimits(ctx, sh, string(src), limits, &runtimeUsed); err != nil && !errors.Is(err, shell.ErrExit) {
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
				if err := runWithLocalLimits(ctx, sh, execCommand, limits, &runtimeUsed); err != nil && !errors.Is(err, shell.ErrExit) {
					return err
				}
				return nil
			}
			return runPiped(ctx, sh, os.Stdin, limits, &runtimeUsed)
		}

		return runInteractive(ctx, sh, limits, &runtimeUsed, !persistedRcLoaded)
	},
}

func runInteractive(ctx context.Context, sh *shell.Shell, limits session.Limits, runtimeUsed *time.Duration, loadRC bool) error {
	if loadRC {
		loadMemshrc(sh, ctx)
	}

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
			if err := runWithLocalLimits(ctx, sh, script, limits, runtimeUsed); err != nil {
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
		if err := runWithLocalLimits(ctx, sh, line, limits, runtimeUsed); err != nil {
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

func runPiped(ctx context.Context, sh *shell.Shell, r io.Reader, limits session.Limits, runtimeUsed *time.Duration) error {
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
			if runErr := runWithLocalLimits(ctx, sh, line, limits, runtimeUsed); runErr != nil {
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

func runWithLocalLimits(parent context.Context, sh *shell.Shell, script string, limits session.Limits, runtimeUsed *time.Duration) error {
	if err := limits.ValidateRuntime(*runtimeUsed); err != nil {
		return err
	}
	if err := limits.ValidateFS(sh.FS()); err != nil {
		return err
	}

	var runErr error
	startedAt := time.Now()
	if limits.MaxRuntime > 0 {
		remaining := limits.MaxRuntime - *runtimeUsed
		if remaining <= 0 {
			return fmt.Errorf("session runtime limit exceeded: used %s, max %s", (*runtimeUsed).Truncate(time.Millisecond), limits.MaxRuntime)
		}
		runErr = runWithSignalTimeout(parent, sh, script, remaining)
	} else {
		runErr = runWithSignal(parent, sh, script)
	}
	*runtimeUsed += time.Since(startedAt)

	if err := limits.ValidateFS(sh.FS()); err != nil {
		return err
	}
	if err := limits.ValidateRuntime(*runtimeUsed); err != nil {
		return err
	}
	return runErr
}

func runWithSignalTimeout(parent context.Context, sh *shell.Shell, script string, timeout time.Duration) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)

	ctx, cancel := context.WithTimeout(parent, timeout)
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
	case <-ctx.Done():
		<-done
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
		return ctx.Err()
	}
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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
	rootCmd.Flags().IntVar(&localMaxFiles, "max-files", 0, "Maximum files in local shell filesystem (0 = unlimited)")
	rootCmd.Flags().Int64Var(&localMaxBytes, "max-bytes", 0, "Maximum total bytes in local shell filesystem (0 = unlimited)")
	rootCmd.Flags().DurationVar(&localMaxRuntime, "max-runtime", 0, "Maximum cumulative runtime for local shell commands (0 = unlimited)")
	rootCmd.Flags().StringVar(&localStoreDir, "session-store", "", "Directory for local session persistence (requires --session-id)")
	rootCmd.Flags().StringVar(&localSessionID, "session-id", "", "Local persisted session id to load/save (requires --session-store)")
	addNetworkFlags(rootCmd.Flags(), &localNetFlags)
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
