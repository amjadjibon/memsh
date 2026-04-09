package native

import (
	"context"
	"fmt"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type RmdirPlugin struct{}

func (RmdirPlugin) Name() string                                 { return "rmdir" }
func (RmdirPlugin) Description() string                          { return "remove empty directories" }
func (RmdirPlugin) Usage() string                                { return "rmdir <dir>..." }
func (RmdirPlugin) Run(ctx context.Context, args []string) error { return runRmdir(ctx, args) }

func runRmdir(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)
	if len(args) < 2 {
		return fmt.Errorf("rmdir: missing operand")
	}
	for _, target := range args[1:] {
		absPath := sc.ResolvePath(target)
		info, err := sc.FS.Stat(absPath)
		if err != nil {
			return fmt.Errorf("rmdir: cannot remove '%s': %w", target, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("rmdir: cannot remove '%s': Not a directory", target)
		}
		f, err := sc.FS.Open(absPath)
		if err != nil {
			return err
		}
		names, _ := f.Readdirnames(-1)
		f.Close()
		if len(names) > 0 {
			return fmt.Errorf("rmdir: cannot remove '%s': Directory not empty", target)
		}
		if err := sc.FS.Remove(absPath); err != nil {
			return fmt.Errorf("rmdir: cannot remove '%s': %w", target, err)
		}
		if len(args) > 2 {
			fmt.Fprintf(hc.Stdout, "rmdir: removing directory, '%s'\n", target)
		}
	}
	return nil
}
