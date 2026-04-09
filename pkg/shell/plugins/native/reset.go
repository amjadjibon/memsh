package native

import "context"

type ResetPlugin struct{}

func (ResetPlugin) Name() string                                 { return "reset" }
func (ResetPlugin) Description() string                          { return "clear the terminal screen" }
func (ResetPlugin) Usage() string                                { return "reset" }
func (ResetPlugin) Run(ctx context.Context, args []string) error { return runReset(ctx, args) }

func runReset(ctx context.Context, args []string) error { return ClearPlugin{}.Run(ctx, args) }
