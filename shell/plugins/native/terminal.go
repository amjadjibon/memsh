package native

import (
	"context"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/shell/plugins"
)

// ── tput ─────────────────────────────────────────────────────────────────────

// TputPlugin provides terminal capability stubs for script compatibility.
// In the virtual shell there is no real terminal, so sensible defaults are used.
//
//	tput cols      → 80
//	tput lines     → 24
//	tput colors    → 256
//	tput bold      → ANSI bold sequence
//	tput sgr0      → ANSI reset sequence
//	tput setaf N   → ANSI foreground colour sequence
//	tput setab N   → ANSI background colour sequence
//	tput cup R C   → ANSI cursor-position sequence
//	tput clear     → ANSI clear-screen sequence
//	tput init / tput reset / tput smul / tput rmul / tput rev → ANSI sequences
type TputPlugin struct{}

func (TputPlugin) Name() string        { return "tput" }
func (TputPlugin) Description() string { return "terminal capability stub" }
func (TputPlugin) Usage() string       { return "tput <cap> [args...]" }

// virtual terminal defaults
const (
	termCols   = 80
	termLines  = 24
	termColors = 256
)

// ANSI escape sequences emitted by tput.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRev    = "\033[7m"
	ansiSmul   = "\033[4m"  // underline on
	ansiRmul   = "\033[24m" // underline off
	ansiClear  = "\033[H\033[2J"
	ansiEl     = "\033[K" // erase to end of line
	ansiEl1    = "\033[1K"
	ansiEd     = "\033[J"
)

func (TputPlugin) Run(ctx context.Context, args []string) error {
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

// ensure TputPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = TputPlugin{}

// ── stty ─────────────────────────────────────────────────────────────────────

// SttyPlugin provides terminal settings stubs for script compatibility.
// No real terminal is present; sensible defaults are returned.
//
//	stty size         → "24 80"
//	stty cols         → 80
//	stty rows         → 24
//	stty -a / --all   → fake settings dump
//	stty <setting>    → no-op (silently accepted)
type SttyPlugin struct{}

func (SttyPlugin) Name() string        { return "stty" }
func (SttyPlugin) Description() string { return "terminal settings stub" }
func (SttyPlugin) Usage() string       { return "stty [size | cols | rows | -a | setting...]" }

func (SttyPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	_ = plugins.ShellCtx(ctx)

	if len(args) < 2 {
		// bare stty → print speed and settings (minimal)
		fmt.Fprintf(hc.Stdout, "speed 38400 baud; rows %d; columns %d;\n", termLines, termCols)
		return nil
	}

	switch args[1] {
	case "size":
		fmt.Fprintf(hc.Stdout, "%d %d\n", termLines, termCols)
	case "cols", "columns":
		fmt.Fprintln(hc.Stdout, termCols)
	case "rows":
		fmt.Fprintln(hc.Stdout, termLines)
	case "-a", "--all":
		fmt.Fprintf(hc.Stdout, sttyAll)
	case "-g", "--save":
		// print a restorable settings string (fake)
		fmt.Fprintf(hc.Stdout, "1:0:bf:8a3b:3:1c:7f:15:4:0:1:0:11:13:1a:0:12:f:17:16:0:0:0:0:0:0:0:0:0:0:0:0:0:0:0:0\n")
	default:
		// all other settings (echo, raw, -echo, speed, etc.) → no-op
	}
	return nil
}

const sttyAll = `speed 38400 baud; rows 24; columns 80; line = 0;
intr = ^C; quit = ^\; erase = ^?; kill = ^U; eof = ^D; eol = <undef>;
eol2 = <undef>; swtch = <undef>; start = ^Q; stop = ^S; susp = ^Z; rprnt = ^R;
werase = ^W; lnext = ^V; discard = ^O; min = 1; time = 0;
-parenb -parodd -cmspar cs8 -hupcl -cstopb cread -clocal -crtscts
-ignbrk -brkint -ignpar -parmrk -inpck -istrip -inlcr -igncr icrnl ixon -ixoff
-iuclc -ixany -imaxbel -iutf8
opost -olcuc -ocrnl onlcr -onocr -onlret -ofill -ofdel nl0 cr0 tab0 bs0 vt0 ff0
isig icanon iexten echo echoe echok -echonl -noflsh -xcase -tostop -echoprt
echoctl echoke -flusho -extproc
`

// ensure SttyPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = SttyPlugin{}
