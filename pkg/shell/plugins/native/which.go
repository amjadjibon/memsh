package native

import (
	"context"
	"fmt"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type WhichPlugin struct{}

func (WhichPlugin) Name() string                                 { return "which" }
func (WhichPlugin) Description() string                          { return "locate a command" }
func (WhichPlugin) Usage() string                                { return "which <name>..." }
func (WhichPlugin) Run(ctx context.Context, args []string) error { return runWhich(ctx, args) }

func runWhich(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)
	if len(args) < 2 {
		return nil
	}
	for _, name := range args[1:] {
		if v, ok := sc.AliasLookup(name); ok {
			fmt.Fprintf(hc.Stdout, "%s: aliased to %s\n", name, v)
			continue
		}
		if info, ok := sc.CommandInfo(name); ok {
			fmt.Fprintf(hc.Stdout, "%s: %s\n", name, info.Kind)
			continue
		}
		fmt.Fprintf(hc.Stdout, "%s: not found\n", name)
	}
	return nil
}
