package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/shell/plugins"
)

type SourcePlugin struct{}

func (SourcePlugin) Name() string                                 { return "source" }
func (SourcePlugin) Description() string                          { return "execute commands from a file" }
func (SourcePlugin) Usage() string                                { return "source <file>" }
func (SourcePlugin) Run(ctx context.Context, args []string) error { return runSource(ctx, args) }

func runSource(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("source: missing file argument")
	}
	return plugins.ShellCtx(ctx).SourceFile(ctx, args[1])
}
