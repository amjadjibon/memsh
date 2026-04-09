package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
	"sort"
)

type HelpPlugin struct{}

func (HelpPlugin) Name() string                                 { return "help" }
func (HelpPlugin) Description() string                          { return "show help for commands" }
func (HelpPlugin) Usage() string                                { return "help [command]" }
func (HelpPlugin) Run(ctx context.Context, args []string) error { return runHelp(ctx, args) }

func runHelp(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)
	if len(args) >= 2 {
		cmd := args[1]
		if info, ok := sc.CommandInfo(cmd); ok {
			desc := info.Description
			if desc == "" {
				desc = info.Kind
			}
			fmt.Fprintf(hc.Stdout, "%s - %s\n", cmd, desc)
			if info.Usage != "" {
				fmt.Fprintf(hc.Stdout, "Usage: %s\n", info.Usage)
			}
		} else {
			fmt.Fprintf(hc.Stdout, "help: no help entry for '%s'\n", cmd)
		}
		return nil
	}
	names := sc.CommandNames()
	sort.Strings(names)
	fmt.Fprintln(hc.Stdout, "Available commands:")
	for _, name := range names {
		info, _ := sc.CommandInfo(name)
		desc := info.Description
		if desc == "" {
			desc = info.Kind
		}
		fmt.Fprintf(hc.Stdout, "  %-10s  %s\n", name, desc)
	}
	return nil
}
