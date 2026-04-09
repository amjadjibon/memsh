package native

import (
	"context"
	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

type QuitPlugin struct{}

func (QuitPlugin) Name() string                                 { return "quit" }
func (QuitPlugin) Description() string                          { return "exit the shell" }
func (QuitPlugin) Usage() string                                { return "quit" }
func (QuitPlugin) Run(ctx context.Context, args []string) error { return runQuit(ctx, args) }

func runQuit(ctx context.Context, args []string) error { return plugins.ShellCtx(ctx).Exit() }
