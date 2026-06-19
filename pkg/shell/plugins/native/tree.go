package native

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// TreePlugin renders directory contents as an ASCII tree, operating on the virtual filesystem.
// Supports depth limiting (-L), hidden entries (-a), directories only (-d), and full paths (-f).
type TreePlugin struct{}

func (TreePlugin) Name() string        { return "tree" }
func (TreePlugin) Description() string { return "list directory contents in a tree-like format" }
func (TreePlugin) Usage() string       { return "tree [-a] [-d] [-f] [-L depth] [path]" }

func (TreePlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	showHidden := false
	dirsOnly := false
	fullPath := false
	maxDepth := -1
	rootPath := sc.Cwd

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			rootPath = sc.ResolvePath(a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		if a == "-L" {
			if i+1 >= len(args) {
				return fmt.Errorf("tree: missing argument to '-L'")
			}
			i++
			n := 0
			for _, c := range args[i] {
				if c < '0' || c > '9' {
					return fmt.Errorf("tree: invalid argument '%s' to '-L'", args[i])
				}
				n = n*10 + int(c-'0')
			}
			if n <= 0 {
				return fmt.Errorf("tree: invalid argument '%s' to '-L'", args[i])
			}
			maxDepth = n
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'a':
				showHidden = true
			case 'd':
				dirsOnly = true
			case 'f':
				fullPath = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("tree: invalid option -- '%s'", unknown)
		}
	}

	// Verify root exists and is a directory.
	info, err := sc.FS.Stat(rootPath)
	if err != nil {
		return fmt.Errorf("tree: '%s': No such file or directory", rootPath)
	}
	if !info.IsDir() {
		return fmt.Errorf("tree: '%s': Not a directory", rootPath)
	}

	fmt.Fprintln(hc.Stdout, rootPath)

	var dirs, files int
	var render func(dir, prefix string, depth int)
	render = func(dir, prefix string, depth int) {
		if maxDepth >= 0 && depth > maxDepth {
			return
		}

		f, err := sc.FS.Open(dir)
		if err != nil {
			return
		}
		names, err := f.Readdirnames(-1)
		f.Close()
		if err != nil {
			return
		}
		sort.Strings(names)

		// Filter entries.
		filtered := names[:0]
		for _, name := range names {
			if !showHidden && strings.HasPrefix(name, ".") {
				continue
			}
			childPath := filepath.Join(dir, name)
			ci, err := sc.FS.Stat(childPath)
			if err != nil {
				continue
			}
			if dirsOnly && !ci.IsDir() {
				continue
			}
			filtered = append(filtered, name)
		}

		for i, name := range filtered {
			childPath := filepath.Join(dir, name)
			ci, _ := sc.FS.Stat(childPath)

			connector := "├── "
			childPrefix := prefix + "│   "
			if i == len(filtered)-1 {
				connector = "└── "
				childPrefix = prefix + "    "
			}

			label := name
			if fullPath {
				label = childPath
			}
			fmt.Fprintf(hc.Stdout, "%s%s%s\n", prefix, connector, label)

			if ci != nil && ci.IsDir() {
				dirs++
				render(childPath, childPrefix, depth+1)
			} else {
				files++
			}
		}
	}

	render(rootPath, "", 1)

	fmt.Fprintf(hc.Stdout, "\n%d directories, %d files\n", dirs, files)
	return nil
}

var _ plugins.PluginInfo = TreePlugin{}
