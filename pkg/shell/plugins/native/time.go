package native

import (
	"context"
	"fmt"
	"time"

	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

// TimePlugin implements `time` — measure execution time of a command.
type TimePlugin struct{}

func (TimePlugin) Name() string        { return "time" }
func (TimePlugin) Description() string { return "time a command's execution" }
func (TimePlugin) Usage() string       { return "time COMMAND [ARGS...]" }

func (TimePlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	// Skip leading "--" separator if present.
	i := 1
	if i < len(args) && args[i] == "--" {
		i++
	}

	if i >= len(args) {
		fmt.Fprintln(hc.Stderr, "time: missing command")
		return interp.ExitStatus(1)
	}

	cmdArgs := args[i:]

	start := time.Now()
	err := sc.Exec(ctx, cmdArgs)
	elapsed := time.Since(start)

	real := elapsed
	fmt.Fprintf(hc.Stderr, "\nreal\t%s\nuser\t%s\nsys\t%s\n",
		fmtDuration(real),
		fmtDuration(0),
		fmtDuration(0),
	)

	return err
}

// fmtDuration formats a duration in the style `0m1.234s`.
func fmtDuration(d time.Duration) string {
	m := int(d.Minutes())
	s := d.Seconds() - float64(m)*60
	return fmt.Sprintf("%dm%.3fs", m, s)
}

var _ plugins.PluginInfo = TimePlugin{}
