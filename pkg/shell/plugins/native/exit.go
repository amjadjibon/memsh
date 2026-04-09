package native

import (
	"context"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

type ExitPlugin struct{}

func (ExitPlugin) Name() string                                 { return "exit" }
func (ExitPlugin) Description() string                          { return "exit the shell" }
func (ExitPlugin) Usage() string                                { return "exit" }
func (ExitPlugin) Run(ctx context.Context, args []string) error { return plugins.ShellCtx(ctx).Exit() }
