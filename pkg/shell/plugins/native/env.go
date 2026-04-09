package native

import (
	"context"
	"fmt"
	"sort"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type EnvPlugin struct{}

func (EnvPlugin) Name() string                                 { return "env" }
func (EnvPlugin) Description() string                          { return "display environment variables" }
func (EnvPlugin) Usage() string                                { return "env" }
func (EnvPlugin) Run(ctx context.Context, args []string) error { return runEnv(ctx, args) }

func runEnv(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	env := plugins.ShellCtx(ctx).EnvAll()
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(hc.Stdout, "%s=%s\n", k, env[k])
	}
	return nil
}
