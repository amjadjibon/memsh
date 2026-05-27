package native

import (
	"context"
	"fmt"
	"os/user"

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
	// Fall back to the real OS user so the command always produces output.
	if name == "" {
		if u, err := user.Current(); err == nil {
			name = u.Username
		}
	}
	if name == "" {
		name = "unknown"
	}

	fmt.Fprintln(hc.Stdout, name)
	return nil
}

var _ plugins.PluginInfo = WhoamiPlugin{}
