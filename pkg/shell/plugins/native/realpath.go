package native

import (
	"context"
	"fmt"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// RealpathPlugin implements `realpath`.
// Resolves each path argument to an absolute virtual-FS path using the shell's
// ResolvePath helper (which applies the current working directory).
type RealpathPlugin struct{}

func (RealpathPlugin) Name() string        { return "realpath" }
func (RealpathPlugin) Description() string { return "resolve absolute path names" }
func (RealpathPlugin) Usage() string       { return "realpath PATH..." }

func (RealpathPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	var paths []string
	for _, a := range args[1:] {
		if strings.HasPrefix(a, "-") {
			// Silently ignore flags (--relative-to, --no-symlinks, etc.) that
			// don't apply to an in-memory FS; just skip them and their value.
			continue
		}
		paths = append(paths, a)
	}

	if len(paths) == 0 {
		fmt.Fprintln(hc.Stderr, "realpath: missing operand")
		return interp.ExitStatus(1)
	}

	for _, p := range paths {
		fmt.Fprintln(hc.Stdout, sc.ResolvePath(p))
	}
	return nil
}

var _ plugins.PluginInfo = RealpathPlugin{}
