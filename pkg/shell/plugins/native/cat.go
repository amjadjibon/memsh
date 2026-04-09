package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"io"
	"mvdan.cc/sh/v3/interp"
)

type CatPlugin struct{}

func (CatPlugin) Name() string                                 { return "cat" }
func (CatPlugin) Description() string                          { return "concatenate and print files" }
func (CatPlugin) Usage() string                                { return "cat [file]..." }
func (CatPlugin) Run(ctx context.Context, args []string) error { return runCat(ctx, args) }

func runCat(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	if len(args) < 2 {
		_, err := copyWithContext(ctx, hc.Stdout, hc.Stdin)
		if ctx.Err() != nil {
			return nil
		}
		return err
	}

	for _, target := range args[1:] {
		if target == "-" {
			_, err := copyWithContext(ctx, hc.Stdout, hc.Stdin)
			if err != nil && ctx.Err() == nil {
				return err
			}
			continue
		}
		f, err := sc.FS.Open(sc.ResolvePath(target))
		if err != nil {
			return fmt.Errorf("cat: %s: No such file or directory", target)
		}
		_, copyErr := io.Copy(hc.Stdout, f)
		f.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}
