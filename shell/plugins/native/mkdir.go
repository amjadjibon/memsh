package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/shell/plugins"
	"mvdan.cc/sh/v3/interp"
	"os"
	"strconv"
)

type MkdirPlugin struct{}

func (MkdirPlugin) Name() string                                 { return "mkdir" }
func (MkdirPlugin) Description() string                          { return "create directories" }
func (MkdirPlugin) Usage() string                                { return "mkdir [-p] [-v] [-m mode] <dir>..." }
func (MkdirPlugin) Run(ctx context.Context, args []string) error { return runMkdir(ctx, args) }

func runMkdir(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)
	verbose := false
	var perm os.FileMode = 0755
	var dirs []string
	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			dirs = append(dirs, args[i+1:]...)
			break
		}
		if a == "" || a[0] != '-' {
			dirs = append(dirs, a)
			continue
		}
		if a == "-m" || a == "--mode" {
			if i+1 >= len(args) {
				return fmt.Errorf("mkdir: missing operand for -m")
			}
			i++
			v, err := strconv.ParseUint(args[i], 8, 32)
			if err != nil {
				return fmt.Errorf("mkdir: invalid mode '%s'", args[i])
			}
			perm = os.FileMode(v)
			continue
		}
		if a == "--parents" {
			continue
		}
		if a == "--verbose" {
			verbose = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'p':
			case 'v':
				verbose = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("mkdir: invalid option -- '%s'", unknown)
		}
	}
	if len(dirs) == 0 {
		return fmt.Errorf("mkdir: missing operand")
	}
	for _, dir := range dirs {
		if err := sc.FS.MkdirAll(sc.ResolvePath(dir), perm); err != nil {
			return fmt.Errorf("mkdir: cannot create directory '%s': %w", dir, err)
		}
		if verbose {
			fmt.Fprintf(hc.Stdout, "mkdir: created directory '%s'\n", dir)
		}
	}
	return nil
}
