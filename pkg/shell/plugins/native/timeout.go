package native

import (
	"context"
	"fmt"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type TimeoutPlugin struct{}

func (TimeoutPlugin) Name() string                                 { return "timeout" }
func (TimeoutPlugin) Description() string                          { return "run a command with a time limit" }
func (TimeoutPlugin) Usage() string                                { return "timeout DURATION CMD [ARGS...]" }
func (TimeoutPlugin) Run(ctx context.Context, args []string) error { return runTimeout(ctx, args) }

func runTimeout(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)
	i := 1
	for i < len(args) {
		switch args[i] {
		case "--":
			i++
			goto doneFlags
		case "-s", "--signal", "-k", "--kill-after":
			i += 2
		case "--preserve-status", "--foreground":
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(hc.Stderr, "timeout: unknown flag %q\n", args[i])
				return interp.ExitStatus(125)
			}
			goto doneFlags
		}
	}
doneFlags:
	if i >= len(args) {
		fmt.Fprintln(hc.Stderr, "timeout: missing operand")
		return interp.ExitStatus(125)
	}
	dur, err := parseDuration(args[i])
	if err != nil {
		fmt.Fprintf(hc.Stderr, "timeout: invalid time interval %q\n", args[i])
		return interp.ExitStatus(125)
	}
	i++
	if i >= len(args) {
		fmt.Fprintln(hc.Stderr, "timeout: missing command")
		return interp.ExitStatus(125)
	}
	cmdArgs := args[i:]
	if dur <= 0 {
		return sc.Exec(ctx, cmdArgs)
	}
	tctx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()
	err = sc.Exec(tctx, cmdArgs)
	if err != nil && tctx.Err() == context.DeadlineExceeded {
		return interp.ExitStatus(124)
	}
	return err
}
