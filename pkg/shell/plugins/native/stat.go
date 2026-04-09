package native

import (
	"context"
	"fmt"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type StatPlugin struct{}

func (StatPlugin) Name() string                                 { return "stat" }
func (StatPlugin) Description() string                          { return "show file status" }
func (StatPlugin) Usage() string                                { return "stat <file>..." }
func (StatPlugin) Run(ctx context.Context, args []string) error { return runStat(ctx, args) }

func runStat(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	if len(args) < 2 {
		return fmt.Errorf("stat: missing operand")
	}
	for _, target := range args[1:] {
		info, err := sc.FS.Stat(sc.ResolvePath(target))
		if err != nil {
			return fmt.Errorf("stat: cannot stat '%s': %w", target, err)
		}
		fmt.Fprintf(hc.Stdout, "  File: %s\n", target)
		fmt.Fprintf(hc.Stdout, "  Size: %d\n", info.Size())
		fmt.Fprintf(hc.Stdout, "  Mode: %s\n", info.Mode())
		fmt.Fprintf(hc.Stdout, " IsDir: %v\n", info.IsDir())
		fmt.Fprintf(hc.Stdout, "ModTime: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))
	}
	return nil
}
