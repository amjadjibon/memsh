package native

import "context"

type PrintenvPlugin struct{}

func (PrintenvPlugin) Name() string                                 { return "printenv" }
func (PrintenvPlugin) Description() string                          { return "display environment variables" }
func (PrintenvPlugin) Usage() string                                { return "printenv" }
func (PrintenvPlugin) Run(ctx context.Context, args []string) error { return runPrintenv(ctx, args) }

func runPrintenv(ctx context.Context, args []string) error {
	return EnvPlugin{}.Run(ctx, args)
}
