package native

import (
	"context"
	"fmt"

	"mvdan.cc/sh/v3/interp"
)

type DfPlugin struct{}

func (DfPlugin) Name() string                                 { return "df" }
func (DfPlugin) Description() string                          { return "show filesystem space" }
func (DfPlugin) Usage() string                                { return "df" }
func (DfPlugin) Run(ctx context.Context, args []string) error { return runDf(ctx, args) }

func runDf(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	fmt.Fprintln(hc.Stdout, "Filesystem     1K-blocks    Used Available Use% Mounted on")
	fmt.Fprintln(hc.Stdout, "memsh               0       0         0    - /")
	return nil
}
