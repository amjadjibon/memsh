package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/shell/plugins"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
	"os"
)

type DuPlugin struct{}

func (DuPlugin) Name() string                                 { return "du" }
func (DuPlugin) Description() string                          { return "estimate file space usage" }
func (DuPlugin) Usage() string                                { return "du [-h] [-s] [path]..." }
func (DuPlugin) Run(ctx context.Context, args []string) error { return runDu(ctx, args) }

func runDu(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	humanReadable := false
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
		if a == "--human-readable" || a == "--summary" {
			if a == "--human-readable" {
				humanReadable = true
			}
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'h':
				humanReadable = true
			case 's':
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("du: invalid option -- '%s'", unknown)
		}
	}

	if len(targets) == 0 {
		targets = []string{sc.Cwd}
	}

	formatSize := func(size int64) string {
		if humanReadable {
			const (
				KB = 1024
				MB = KB * 1024
				GB = MB * 1024
			)
			switch {
			case size >= GB:
				return fmt.Sprintf("%.1fG", float64(size)/float64(GB))
			case size >= MB:
				return fmt.Sprintf("%.1fM", float64(size)/float64(MB))
			case size >= KB:
				return fmt.Sprintf("%.1fK", float64(size)/float64(KB))
			}
		}
		return fmt.Sprintf("%d", size)
	}

	for _, target := range targets {
		absPath := sc.ResolvePath(target)
		var total int64
		_ = afero.Walk(sc.FS, absPath, func(_ string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				total += info.Size()
			}
			return nil
		})
		fmt.Fprintf(hc.Stdout, "%s\t%s\n", formatSize(total), target)
	}
	return nil
}
