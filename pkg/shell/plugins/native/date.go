package native

import (
	"context"
	"fmt"
	"time"

	"mvdan.cc/sh/v3/interp"
)

type DatePlugin struct{}

func (DatePlugin) Name() string                                 { return "date" }
func (DatePlugin) Description() string                          { return "print the current date and time" }
func (DatePlugin) Usage() string                                { return "date [+format]" }
func (DatePlugin) Run(ctx context.Context, args []string) error { return runDate(ctx, args) }

func runDate(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	format := ""
	utc := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-u", "--utc", "--universal":
			utc = true
		case "+%s", "+%F", "+%T", "+%Y-%m-%d", "+%H:%M:%S":
			format = args[i][1:]
		}
	}

	now := time.Now()
	if utc {
		now = now.UTC()
	}

	if format == "" {
		fmt.Fprintln(hc.Stdout, now.Format("Mon Jan 2 15:04:05 MST 2006"))
		return nil
	}
	fmt.Fprintln(hc.Stdout, now.Format(format))
	return nil
}
