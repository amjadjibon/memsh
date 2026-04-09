package native

import (
	"context"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/interp"
)

type YesPlugin struct{}

func (YesPlugin) Name() string                                 { return "yes" }
func (YesPlugin) Description() string                          { return "output a string repeatedly" }
func (YesPlugin) Usage() string                                { return "yes [string]" }
func (YesPlugin) Run(ctx context.Context, args []string) error { return runYes(ctx, args) }

func runYes(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	msg := "y"
	if len(args) > 1 {
		msg = strings.Join(args[1:], " ")
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if _, err := fmt.Fprintln(hc.Stdout, msg); err != nil {
			return nil
		}
	}
}
