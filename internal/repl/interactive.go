package repl

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
	"golang.org/x/term"

	"github.com/amjadjibon/memsh/internal/paths"
	"github.com/amjadjibon/memsh/pkg/shell"
)

// RunInteractive starts an interactive REPL session.
func RunInteractive(ctx context.Context, sh *shell.Shell) error {
	loadMemshrc(sh, ctx)

	histFile, err := historyFile()
	if err != nil && os.Getenv("DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "memsh: history file: %v\n", err)
	}

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          prompt(sh),
		AutoComplete:    &REPLCompleter{sh: sh},
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
			if os.Getenv("DEBUG") != "" {
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

		if os.Getenv("DEBUG") != "" {
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

// RunPiped executes commands from piped input (non-interactive mode).
func RunPiped(ctx context.Context, sh *shell.Shell, r io.Reader) error {
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

// IsInteractiveTerminal returns true if stdin is an interactive terminal.
func IsInteractiveTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// ShouldRunInteractive returns true if the shell should run in interactive mode.
func ShouldRunInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// GetVersion returns the version string from build info.
func GetVersion() string {
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

func prompt(sh *shell.Shell) string {
	return fmt.Sprintf("memsh:%s$ ", sh.Cwd())
}

func historyFile() (string, error) {
	dir, err := paths.HistoryDir()
	if err != nil {
		return "", err
	}

	// Remove the file if it exists and is not a directory
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		os.Remove(dir)
	}

	ts := fmt.Appendf(nil, "%d", time.Now().UnixNano())
	hash := sha256.Sum256(ts)
	name := fmt.Sprintf("%x", hash)
	return filepath.Join(dir, name), nil
}

func loadMemshrc(sh *shell.Shell, ctx context.Context) {
	rcPath, err := paths.MemshrcFile()
	if err != nil {
		return
	}
	data, err := os.ReadFile(rcPath)
	if err != nil {
		return
	}
	if os.Getenv("DEBUG") != "" {
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
