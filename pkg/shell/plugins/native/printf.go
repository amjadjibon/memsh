package native

import (
	"context"
	"fmt"

	"mvdan.cc/sh/v3/interp"
)

type PrintfPlugin struct{}

func (PrintfPlugin) Name() string                                 { return "printf" }
func (PrintfPlugin) Description() string                          { return "format and print data" }
func (PrintfPlugin) Usage() string                                { return "printf <format> [args...]" }
func (PrintfPlugin) Run(ctx context.Context, args []string) error { return runPrintf(ctx, args) }

func runPrintf(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return nil
	}
	_, err := fmt.Fprint(interp.HandlerCtx(ctx).Stdout, expandPrintfFormat(args[1], args[2:]))
	return err
}
