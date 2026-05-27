package native

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// WatchPlugin implements the `watch` command.
// It repeatedly runs a command at a fixed interval, printing a header and the
// output each time, until the context is cancelled (e.g. Ctrl-C).
type WatchPlugin struct{}

func (WatchPlugin) Name() string        { return "watch" }
func (WatchPlugin) Description() string { return "re-run a command every N seconds" }
func (WatchPlugin) Usage() string       { return "watch [-n seconds] [-t] CMD [ARGS...]" }

func (WatchPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	interval := 2 * time.Second
	noHeader := false

	i := 1
	for i < len(args) {
		switch args[i] {
		case "--":
			i++
			goto doneFlags
		case "-n", "--interval":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "watch: option requires an argument -- 'n'")
				return interp.ExitStatus(1)
			}
			secs, err := strconv.ParseFloat(args[i+1], 64)
			if err != nil || secs <= 0 {
				fmt.Fprintf(hc.Stderr, "watch: invalid interval %q\n", args[i+1])
				return interp.ExitStatus(1)
			}
			interval = time.Duration(secs * float64(time.Second))
			i += 2
		case "-t", "--no-title":
			noHeader = true
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(hc.Stderr, "watch: invalid option -- %q\n", args[i])
				return interp.ExitStatus(1)
			}
			goto doneFlags
		}
	}
doneFlags:
	if i >= len(args) {
		fmt.Fprintln(hc.Stderr, "watch: missing command")
		return interp.ExitStatus(1)
	}
	cmdArgs := args[i:]
	cmdLine := strings.Join(cmdArgs, " ")

	runOnce := func() {
		if !noHeader {
			now := time.Now().Format("Mon Jan  2 15:04:05 2006")
			fmt.Fprintf(hc.Stdout, "Every %.1fs: %-40s  %s\n\n",
				interval.Seconds(), cmdLine, now)
		}
		_ = sc.Exec(ctx, cmdArgs)
		fmt.Fprintln(hc.Stdout)
	}

	// First run immediately.
	runOnce()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if !noHeader {
				fmt.Fprintf(hc.Stdout, "%s\n", strings.Repeat("-", 60))
			}
			runOnce()
		}
	}
}

var _ plugins.PluginInfo = WatchPlugin{}
