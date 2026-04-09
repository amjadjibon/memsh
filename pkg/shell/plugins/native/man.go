package native

import "context"

type ManPlugin struct{}

func (ManPlugin) Name() string                                 { return "man" }
func (ManPlugin) Description() string                          { return "show help for commands" }
func (ManPlugin) Usage() string                                { return "man [command]" }
func (ManPlugin) Run(ctx context.Context, args []string) error { return runMan(ctx, args) }

func runMan(ctx context.Context, args []string) error { return HelpPlugin{}.Run(ctx, args) }
