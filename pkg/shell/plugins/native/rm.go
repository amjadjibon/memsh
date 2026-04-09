package native

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type RmPlugin struct{}

func (RmPlugin) Name() string                                 { return "rm" }
func (RmPlugin) Description() string                          { return "remove files or directories" }
func (RmPlugin) Usage() string                                { return "rm [-f] [-r] [-R] [-v] [-d] [-i] [-I] [--] <path>..." }
func (RmPlugin) Run(ctx context.Context, args []string) error { return runRm(ctx, args) }

func runRm(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)
	force, recursive, verbose, dirOnly, interactive, endOfFlags := false, false, false, false, false, false
	var targets []string
	for _, a := range args[1:] {
		if endOfFlags || a == "" || a[0] != '-' {
			targets = append(targets, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		switch a {
		case "--force":
			force = true
			continue
		case "--recursive":
			recursive = true
			continue
		case "--verbose":
			verbose = true
			continue
		case "--dir":
			dirOnly = true
			continue
		case "--interactive":
			interactive = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'f':
				force = true
			case 'r', 'R':
				recursive = true
			case 'v':
				verbose = true
			case 'd':
				dirOnly = true
			case 'i', 'I':
				interactive = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("rm: invalid option -- '%s'", unknown)
		}
	}
	if len(targets) == 0 {
		if force {
			return nil
		}
		return fmt.Errorf("rm: missing operand")
	}
	for _, target := range targets {
		absPath := sc.ResolvePath(target)
		info, err := sc.FS.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) && force {
				continue
			}
			return fmt.Errorf("rm: cannot remove '%s': %w", target, err)
		}
		if info.IsDir() {
			if !recursive && !dirOnly {
				return fmt.Errorf("rm: cannot remove '%s': Is a directory", target)
			}
			if interactive && recursive {
				fmt.Fprintf(hc.Stdout, "rm: descend into directory '%s'? ", target)
				var resp string
				fmt.Fscan(hc.Stdin, &resp)
				resp = strings.ToLower(strings.TrimSpace(resp))
				if resp != "y" && resp != "yes" {
					continue
				}
			}
		} else if interactive {
			fmt.Fprintf(hc.Stdout, "rm: remove regular file '%s'? ", target)
			var resp string
			fmt.Fscan(hc.Stdin, &resp)
			resp = strings.ToLower(strings.TrimSpace(resp))
			if resp != "y" && resp != "yes" {
				continue
			}
		}
		if err := sc.FS.RemoveAll(absPath); err != nil {
			return fmt.Errorf("rm: cannot remove '%s': %w", target, err)
		}
		if verbose {
			if info.IsDir() {
				fmt.Fprintf(hc.Stdout, "removed directory '%s'\n", target)
			} else {
				fmt.Fprintf(hc.Stdout, "removed '%s'\n", target)
			}
		}
	}
	return nil
}
