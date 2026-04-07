package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/shell/plugins"
	"mvdan.cc/sh/v3/interp"
	"path/filepath"
	"sort"
	"strings"
)

type LsPlugin struct{}

func (LsPlugin) Name() string                                 { return "ls" }
func (LsPlugin) Description() string                          { return "list directory contents" }
func (LsPlugin) Usage() string                                { return "ls [-l] [-a] [-R] [path]" }
func (LsPlugin) Run(ctx context.Context, args []string) error { return runLs(ctx, args) }

func runLs(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)
	longFormat, showAll, recursive := false, false, false
	var targets []string
	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			targets = append(targets, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		switch a {
		case "--format=long":
			longFormat = true
			continue
		case "--all":
			showAll = true
			continue
		case "--recursive":
			recursive = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'l':
				longFormat = true
			case 'a':
				showAll = true
			case 'R':
				recursive = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("ls: invalid option -- '%s'", unknown)
		}
	}
	if len(targets) == 0 {
		targets = []string{sc.Cwd}
	}
	var listOne func(string) error
	listOne = func(target string) error {
		absPath := sc.ResolvePath(target)
		f, err := sc.FS.Open(absPath)
		if err != nil {
			return fmt.Errorf("ls: cannot access '%s': %w", target, err)
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if longFormat {
				fmt.Fprintf(hc.Stdout, "%s  %8d  %s  %s\n", info.Mode(), info.Size(), info.ModTime().Format("Jan 02 15:04"), filepath.Base(target))
			} else {
				fmt.Fprintln(hc.Stdout, filepath.Base(target))
			}
			return nil
		}
		if recursive {
			fmt.Fprintf(hc.Stdout, "%s:\n", target)
		}
		names, err := f.Readdirnames(-1)
		if err != nil {
			return err
		}
		sort.Strings(names)
		for _, name := range names {
			if !showAll && strings.HasPrefix(name, ".") {
				continue
			}
			if longFormat {
				childPath := filepath.Join(absPath, name)
				ci, err := sc.FS.Stat(childPath)
				if err != nil {
					ci = nil
				}
				if ci != nil {
					prefix := "-"
					if ci.IsDir() {
						prefix = "d"
					}
					fmt.Fprintf(hc.Stdout, "%s%s  %8d  %s  %s\n", prefix, ci.Mode().Perm(), ci.Size(), ci.ModTime().Format("Jan 02 15:04"), name)
				} else {
					fmt.Fprintln(hc.Stdout, name)
				}
			} else {
				fmt.Fprintln(hc.Stdout, name)
			}
		}
		if recursive {
			for _, name := range names {
				if !showAll && strings.HasPrefix(name, ".") {
					continue
				}
				childPath := filepath.Join(absPath, name)
				ci, err := sc.FS.Stat(childPath)
				if err == nil && ci.IsDir() {
					fmt.Fprintln(hc.Stdout)
					_ = listOne(filepath.Join(target, name))
				}
			}
		}
		return nil
	}
	for _, target := range targets {
		if err := listOne(target); err != nil {
			return err
		}
	}
	return nil
}
