package native

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

// LsofPlugin implements `lsof` — list files in the virtual filesystem.
//
// Because memsh uses an in-memory FS with no kernel fd table, "open files"
// is interpreted as all files (and optionally dirs) present in the virtual FS.
//
//	lsof [-d] [-p path] [-s min_size]
type LsofPlugin struct{}

func (LsofPlugin) Name() string        { return "lsof" }
func (LsofPlugin) Description() string { return "list files in the virtual filesystem" }
func (LsofPlugin) Usage() string {
	return "lsof [-a] [-d] [-s size] [path]"
}

func (LsofPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	showDirs := false  // -d: include directories
	allFiles := false  // -a: include hidden files (dot-files)
	minSize := int64(-1) // -s: minimum size filter
	root := "/"        // default: scan everything

	i := 1
	for i < len(args) {
		switch args[i] {
		case "--":
			i++
			goto doneFlags
		case "-d":
			showDirs = true
			i++
		case "-a":
			allFiles = true
			i++
		case "-s", "--size":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "lsof: -s requires an argument")
				return interp.ExitStatus(1)
			}
			n, err := parseSize(args[i+1])
			if err != nil {
				fmt.Fprintf(hc.Stderr, "lsof: invalid size %q\n", args[i+1])
				return interp.ExitStatus(1)
			}
			minSize = n
			i += 2
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(hc.Stderr, "lsof: invalid option -- %q\n", args[i])
				return interp.ExitStatus(1)
			}
			root = sc.ResolvePath(args[i])
			i++
		}
	}
doneFlags:
	if i < len(args) {
		root = sc.ResolvePath(args[i])
	}

	// Print header
	fmt.Fprintf(hc.Stdout, "%-6s %-10s %s\n", "TYPE", "SIZE", "NAME")
	fmt.Fprintln(hc.Stdout, strings.Repeat("-", 50))

	count := 0
	err := afero.Walk(sc.FS, root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		// Filter hidden files unless -a
		base := lastSegment(path)
		if !allFiles && strings.HasPrefix(base, ".") && path != "/" {
			if info.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			if !showDirs {
				return nil
			}
			fmt.Fprintf(hc.Stdout, "%-6s %-10s %s\n", "DIR", "-", path)
			count++
			return nil
		}

		if minSize >= 0 && info.Size() < minSize {
			return nil
		}

		fmt.Fprintf(hc.Stdout, "%-6s %-10s %s\n", "REG", formatSize(info.Size()), path)
		count++
		return nil
	})
	if err != nil {
		fmt.Fprintf(hc.Stderr, "lsof: %v\n", err)
		return interp.ExitStatus(1)
	}

	fmt.Fprintf(hc.Stdout, "\n%d file(s)\n", count)
	return nil
}

// lastSegment returns the last path component without importing filepath.
func lastSegment(p string) string {
	p = strings.TrimRight(p, "/")
	idx := strings.LastIndex(p, "/")
	if idx < 0 {
		return p
	}
	return p[idx+1:]
}

// formatSize formats bytes into a human-readable string.
func formatSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// parseSize parses a size string like "1024", "4K", "2M".
func parseSize(s string) (int64, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty")
	}
	mult := int64(1)
	digits := s
	switch s[len(s)-1] {
	case 'K', 'k':
		mult = 1 << 10
		digits = s[:len(s)-1]
	case 'M', 'm':
		mult = 1 << 20
		digits = s[:len(s)-1]
	case 'G', 'g':
		mult = 1 << 30
		digits = s[:len(s)-1]
	}
	var n int64
	if _, err := fmt.Sscanf(digits, "%d", &n); err != nil || n < 0 {
		return 0, fmt.Errorf("invalid")
	}
	return n * mult, nil
}

var _ plugins.PluginInfo = LsofPlugin{}
