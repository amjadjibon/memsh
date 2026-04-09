package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

type CdPlugin struct{}

func (CdPlugin) Name() string                                 { return "cd" }
func (CdPlugin) Description() string                          { return "change working directory" }
func (CdPlugin) Usage() string                                { return "cd [dir]" }
func (CdPlugin) Run(ctx context.Context, args []string) error { return runCd(ctx, args) }

func runCd(ctx context.Context, args []string) error {
	sc := plugins.ShellCtx(ctx)
	if len(args) > 2 {
		return fmt.Errorf("cd: too many arguments")
	}
	dir := "/"
	if len(args) == 2 {
		dir = args[1]
	}
	return sc.SetCwd(dir)
}
