package native

import (
	"context"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/interp"
)

type TputPlugin struct{}

func (TputPlugin) Name() string                                 { return "tput" }
func (TputPlugin) Description() string                          { return "terminal capability stub" }
func (TputPlugin) Usage() string                                { return "tput <cap> [args...]" }
func (TputPlugin) Run(ctx context.Context, args []string) error { return runTput(ctx, args) }

// virtual terminal defaults
const (
	termCols   = 80
	termLines  = 24
	termColors = 256
)

// ANSI escape sequences emitted by tput.
const (
	ansiReset = "\033[0m"
	ansiBold  = "\033[1m"
	ansiDim   = "\033[2m"
	ansiRev   = "\033[7m"
	ansiSmul  = "\033[4m"  // underline on
	ansiRmul  = "\033[24m" // underline off
	ansiClear = "\033[H\033[2J"
	ansiEl    = "\033[K" // erase to end of line
	ansiEl1   = "\033[1K"
	ansiEd    = "\033[J"
)

func runTput(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	if len(args) < 2 {
		fmt.Fprintln(hc.Stderr, "tput: missing operand")
		return interp.ExitStatus(1)
	}

	cap := args[1]
	capArgs := args[2:]

	switch strings.ToLower(cap) {
	// numeric capabilities
	case "cols", "columns":
		fmt.Fprintln(hc.Stdout, termCols)
	case "lines", "rows":
		fmt.Fprintln(hc.Stdout, termLines)
	case "colors", "colours":
		fmt.Fprintln(hc.Stdout, termColors)
	case "longname":
		fmt.Fprintln(hc.Stdout, "virtual terminal (memsh)")

	// boolean capabilities → exit 0 = true
	case "am", "bce", "ccc", "xon":
		// supported

	// text attributes
	case "bold":
		fmt.Fprint(hc.Stdout, ansiBold)
	case "dim":
		fmt.Fprint(hc.Stdout, ansiDim)
	case "rev", "mr":
		fmt.Fprint(hc.Stdout, ansiRev)
	case "smul":
		fmt.Fprint(hc.Stdout, ansiSmul)
	case "rmul":
		fmt.Fprint(hc.Stdout, ansiRmul)
	case "sgr0", "me", "op":
		fmt.Fprint(hc.Stdout, ansiReset)

	// foreground / background colour
	case "setaf":
		n := 0
		if len(capArgs) > 0 {
			fmt.Sscanf(capArgs[0], "%d", &n)
		}
		fmt.Fprintf(hc.Stdout, "\033[38;5;%dm", n)
	case "setab":
		n := 0
		if len(capArgs) > 0 {
			fmt.Sscanf(capArgs[0], "%d", &n)
		}
		fmt.Fprintf(hc.Stdout, "\033[48;5;%dm", n)
	case "setf": // legacy 8-colour fg
		n := 0
		if len(capArgs) > 0 {
			fmt.Sscanf(capArgs[0], "%d", &n)
		}
		fmt.Fprintf(hc.Stdout, "\033[3%dm", n)
	case "setb": // legacy 8-colour bg
		n := 0
		if len(capArgs) > 0 {
			fmt.Sscanf(capArgs[0], "%d", &n)
		}
		fmt.Fprintf(hc.Stdout, "\033[4%dm", n)

	// cursor movement
	case "cup":
		row, col := 0, 0
		if len(capArgs) >= 2 {
			fmt.Sscanf(capArgs[0], "%d", &row)
			fmt.Sscanf(capArgs[1], "%d", &col)
		}
		fmt.Fprintf(hc.Stdout, "\033[%d;%dH", row+1, col+1)
	case "cuu1", "up":
		fmt.Fprint(hc.Stdout, "\033[A")
	case "cud1", "do":
		fmt.Fprint(hc.Stdout, "\033[B")
	case "cuf1", "nd":
		fmt.Fprint(hc.Stdout, "\033[C")
	case "cub1", "le":
		fmt.Fprint(hc.Stdout, "\033[D")
	case "cr":
		fmt.Fprint(hc.Stdout, "\r")
	case "nel", "nw":
		fmt.Fprint(hc.Stdout, "\r\n")

	// erase
	case "clear":
		fmt.Fprint(hc.Stdout, ansiClear)
	case "el", "ce":
		fmt.Fprint(hc.Stdout, ansiEl)
	case "el1":
		fmt.Fprint(hc.Stdout, ansiEl1)
	case "ed", "cd":
		fmt.Fprint(hc.Stdout, ansiEd)

	// init / reset — emit reset sequence
	case "init", "reset":
		fmt.Fprint(hc.Stdout, ansiReset)

	// enter/exit alternate screen
	case "smcup":
		fmt.Fprint(hc.Stdout, "\033[?1049h")
	case "rmcup":
		fmt.Fprint(hc.Stdout, "\033[?1049l")

	// hide/show cursor
	case "civis":
		fmt.Fprint(hc.Stdout, "\033[?25l")
	case "cnorm", "cvvis":
		fmt.Fprint(hc.Stdout, "\033[?25h")

	default:
		// unknown capability — exit 1 (no output), like real tput
		return interp.ExitStatus(1)
	}
	return nil
}
