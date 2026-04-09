package native

import (
	"context"
	"fmt"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type XargsPlugin struct{}

func (XargsPlugin) Name() string                                 { return "xargs" }
func (XargsPlugin) Description() string                          { return "build and execute command lines" }
func (XargsPlugin) Usage() string                                { return "xargs <command> [args...]" }
func (XargsPlugin) Run(ctx context.Context, args []string) error { return runXargs(ctx, args) }

func runXargs(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)
	if len(args) < 2 {
		return fmt.Errorf("xargs: missing command")
	}
	scanner := scanWithContext(ctx, hc.Stdin)
	var items []string
	for scanner.Scan() {
		items = append(items, strings.Fields(scanner.Text())...)
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	if len(items) == 0 {
		return nil
	}
	return sc.Exec(ctx, append(append([]string{args[1]}, args[2:]...), items...))
}
