package native

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// BasenamePlugin implements `basename`.
// basename NAME [SUFFIX]   — strip leading directory components (and optional suffix).
// basename -a NAME...      — multiple names, one per line.
type BasenamePlugin struct{}

func (BasenamePlugin) Name() string        { return "basename" }
func (BasenamePlugin) Description() string { return "strip directory and suffix from filenames" }
func (BasenamePlugin) Usage() string       { return "basename [-a] [-s SUFFIX] NAME [SUFFIX]" }

func (BasenamePlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	multiple := false
	suffix := ""
	var names []string

	i := 1
	for i < len(args) {
		switch args[i] {
		case "--":
			i++
			goto doneFlags
		case "-a", "--multiple":
			multiple = true
			i++
		case "-s", "--suffix":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "basename: -s requires an argument")
				return interp.ExitStatus(1)
			}
			suffix = args[i+1]
			multiple = true
			i += 2
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(hc.Stderr, "basename: invalid option -- %q\n", args[i])
				return interp.ExitStatus(1)
			}
			goto doneFlags
		}
	}
doneFlags:
	names = args[i:]

	if len(names) == 0 {
		fmt.Fprintln(hc.Stderr, "basename: missing operand")
		return interp.ExitStatus(1)
	}

	// Legacy two-arg form: basename NAME SUFFIX (no flags).
	if !multiple && len(names) == 2 && suffix == "" {
		suffix = names[1]
		names = names[:1]
	}

	if !multiple && len(names) > 1 {
		fmt.Fprintln(hc.Stderr, "basename: extra operand")
		return interp.ExitStatus(1)
	}

	for _, name := range names {
		b := filepath.Base(name)
		if suffix != "" {
			b = strings.TrimSuffix(b, suffix)
		}
		fmt.Fprintln(hc.Stdout, b)
	}
	return nil
}

// DirnamePlugin implements `dirname`.
// dirname NAME...  — print directory component of each path.
type DirnamePlugin struct{}

func (DirnamePlugin) Name() string        { return "dirname" }
func (DirnamePlugin) Description() string { return "strip last component from file names" }
func (DirnamePlugin) Usage() string       { return "dirname NAME..." }

func (DirnamePlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	if len(args) < 2 {
		fmt.Fprintln(hc.Stderr, "dirname: missing operand")
		return interp.ExitStatus(1)
	}

	for _, name := range args[1:] {
		fmt.Fprintln(hc.Stdout, filepath.Dir(name))
	}
	return nil
}

var _ plugins.PluginInfo = BasenamePlugin{}
var _ plugins.PluginInfo = DirnamePlugin{}
