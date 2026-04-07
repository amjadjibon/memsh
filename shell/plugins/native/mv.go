package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/shell/plugins"
	"path/filepath"
)

type MvPlugin struct{}

func (MvPlugin) Name() string                                 { return "mv" }
func (MvPlugin) Description() string                          { return "move or rename files" }
func (MvPlugin) Usage() string                                { return "mv <src> <dst>" }
func (MvPlugin) Run(ctx context.Context, args []string) error { return runMv(ctx, args) }

func runMv(ctx context.Context, args []string) error {
	sc := plugins.ShellCtx(ctx)
	if len(args) < 3 {
		return fmt.Errorf("mv: missing destination file operand")
	}
	dst := sc.ResolvePath(args[len(args)-1])
	sources := args[1 : len(args)-1]
	for _, src := range sources {
		absSrc := sc.ResolvePath(src)
		target := dst
		dstInfo, _ := sc.FS.Stat(dst)
		if dstInfo != nil && dstInfo.IsDir() {
			target = filepath.Join(dst, filepath.Base(absSrc))
		}
		if err := sc.FS.Rename(absSrc, target); err != nil {
			return fmt.Errorf("mv: cannot move '%s' to '%s': %w", src, args[len(args)-1], err)
		}
	}
	return nil
}
