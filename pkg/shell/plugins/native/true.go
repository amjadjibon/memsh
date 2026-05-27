package native

import (
	"context"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// TruePlugin implements `true` — always exits 0.
type TruePlugin struct{}

func (TruePlugin) Name() string                            { return "true" }
func (TruePlugin) Description() string                     { return "do nothing, successfully" }
func (TruePlugin) Usage() string                           { return "true" }
func (TruePlugin) Run(_ context.Context, _ []string) error { return nil }

// FalsePlugin implements `false` — always exits 1.
type FalsePlugin struct{}

func (FalsePlugin) Name() string        { return "false" }
func (FalsePlugin) Description() string { return "do nothing, unsuccessfully" }
func (FalsePlugin) Usage() string       { return "false" }
func (FalsePlugin) Run(_ context.Context, _ []string) error {
	return interp.ExitStatus(1)
}

var _ plugins.PluginInfo = TruePlugin{}
var _ plugins.PluginInfo = FalsePlugin{}
