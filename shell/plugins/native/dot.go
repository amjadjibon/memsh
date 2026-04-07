package native

import "context"

type DotPlugin struct{}

func (DotPlugin) Name() string                                 { return "." }
func (DotPlugin) Description() string                          { return "execute commands from a file" }
func (DotPlugin) Usage() string                                { return ". <file>" }
func (DotPlugin) Run(ctx context.Context, args []string) error { return runDot(ctx, args) }

func runDot(ctx context.Context, args []string) error { return SourcePlugin{}.Run(ctx, args) }
