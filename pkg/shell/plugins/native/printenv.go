package native

import (
	"context"
	"fmt"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type PrintenvPlugin struct{}

func (PrintenvPlugin) Name() string                                 { return "printenv" }
func (PrintenvPlugin) Description() string                          { return "display environment variables" }
func (PrintenvPlugin) Usage() string                                { return "printenv [name...]" }
func (PrintenvPlugin) Run(ctx context.Context, args []string) error { return runPrintenv(ctx, args) }

// runPrintenv prints the whole environment, or the value of each named
// variable. Like coreutils, it exits 1 if any named variable is unset.
func runPrintenv(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return EnvPlugin{}.Run(ctx, args[:1])
	}
	hc := interp.HandlerCtx(ctx)
	env := plugins.ShellCtx(ctx).EnvAll()
	missing := false
	for _, name := range args[1:] {
		v, ok := env[name]
		if !ok {
			missing = true
			continue
		}
		fmt.Fprintln(hc.Stdout, v)
	}
	if missing {
		return interp.ExitStatus(1)
	}
	return nil
}
