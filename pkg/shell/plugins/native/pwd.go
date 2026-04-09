package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type PwdPlugin struct{}

func (PwdPlugin) Name() string                                 { return "pwd" }
func (PwdPlugin) Description() string                          { return "print working directory" }
func (PwdPlugin) Usage() string                                { return "pwd" }
func (PwdPlugin) Run(ctx context.Context, args []string) error { return runPwd(ctx, args) }

func runPwd(ctx context.Context, args []string) error {
	_, err := fmt.Fprintln(interp.HandlerCtx(ctx).Stdout, plugins.ShellCtx(ctx).Cwd)
	return err
}
