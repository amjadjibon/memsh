package native

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// FindPlugin searches the virtual filesystem for files matching criteria.
//
//	find [path] [-name <glob>] [-type f|d]
type FindPlugin struct{}

func (FindPlugin) Name() string        { return "find" }
func (FindPlugin) Description() string { return "search the virtual filesystem" }
func (FindPlugin) Usage() string       { return "find [path] [-name <glob>] [-type f|d]" }

func (FindPlugin) Run(ctx context.Context, args []string) error {
	sc := plugins.ShellCtx(ctx)
	hc := interp.HandlerCtx(ctx)

	startPath := sc.Cwd
	namePattern := ""
	typeFilter := "" // "f" for files, "d" for dirs
	maxDepth := -1
	pathSet := false

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-name":
			if i+1 >= len(args) {
				return fmt.Errorf("find: missing argument to '-name'")
			}
			i++
			namePattern = args[i]
		case "-type":
			if i+1 >= len(args) {
				return fmt.Errorf("find: missing argument to '-type'")
			}
			i++
			typeFilter = args[i]
			if typeFilter != "f" && typeFilter != "d" {
				return fmt.Errorf("find: unknown argument to -type: %s", typeFilter)
			}
		case "-maxdepth":
			if i+1 >= len(args) {
				return fmt.Errorf("find: missing argument to '-maxdepth'")
			}
			i++
			depth, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("find: invalid argument '%s' to '-maxdepth'", args[i])
			}
			maxDepth = depth
		default:
			if !pathSet {
				startPath = sc.ResolvePath(args[i])
				pathSet = true
			}
		}
	}

	return afero.Walk(sc.FS, startPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		relPath := path
		if filepath.IsAbs(startPath) {
			relPath, _ = filepath.Rel(startPath, path)
		}
		depth := strings.Count(relPath, string(filepath.Separator))
		if relPath == "." {
			depth = 0
		}
		if maxDepth >= 0 && depth > maxDepth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if typeFilter == "f" && info.IsDir() {
			return nil
		}
		if typeFilter == "d" && !info.IsDir() {
			return nil
		}
		if namePattern != "" {
			matched, matchErr := filepath.Match(namePattern, filepath.Base(path))
			if matchErr != nil || !matched {
				return nil
			}
		}
		fmt.Fprintln(hc.Stdout, path)
		return nil
	})
}

// ensure FindPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = FindPlugin{}
