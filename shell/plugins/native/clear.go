package native

import (
	"context"
	"fmt"

	"mvdan.cc/sh/v3/interp"
)

type ClearPlugin struct{}

func (ClearPlugin) Name() string                                 { return "clear" }
func (ClearPlugin) Description() string                          { return "clear the terminal screen" }
func (ClearPlugin) Usage() string                                { return "clear" }
func (ClearPlugin) Run(ctx context.Context, args []string) error { return runClear(ctx, args) }

func runClear(ctx context.Context, _ []string) error {
	_, err := fmt.Fprint(interp.HandlerCtx(ctx).Stdout, "\x1b[2J\x1b[H")
	return err
}
