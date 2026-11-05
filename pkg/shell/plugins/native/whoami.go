package native

import (
	"context"
	"fmt"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// WhoamiPlugin implements the `whoami` command.
// It reports the current user name from the USER environment variable,
// falling back to the real OS user when the variable is unset.
type WhoamiPlugin struct{}

func (WhoamiPlugin) Name() string        { return "whoami" }
func (WhoamiPlugin) Description() string { return "print the current user name" }
func (WhoamiPlugin) Usage() string       { return "whoami" }

func (WhoamiPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	// Prefer the USER variable set in the virtual shell environment.
	name := sc.Env("USER")
	if name == "" {
		name = sc.Env("LOGNAME")
	}
	// Never fall back to the host user: in server mode that would leak the
	// account memsh runs as. Use the fixed virtual identity instead.
	if name == "" {
		name = virtualUser
	}

	fmt.Fprintln(hc.Stdout, name)
	return nil
}

var _ plugins.PluginInfo = WhoamiPlugin{}
