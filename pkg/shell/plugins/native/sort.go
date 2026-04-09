package native

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type SortPlugin struct{}

func (SortPlugin) Name() string                                 { return "sort" }
func (SortPlugin) Description() string                          { return "sort lines of text" }
func (SortPlugin) Usage() string                                { return "sort [-r] [-u] [-n] [file]" }
func (SortPlugin) Run(ctx context.Context, args []string) error { return runSort(ctx, args) }

func runSort(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	reverse, unique, numeric := false, false, false
	var files []string
	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			files = append(files, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		switch a {
		case "--reverse":
			reverse = true
			continue
		case "--unique":
			unique = true
			continue
		case "--numeric-sort":
			numeric = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'r':
				reverse = true
			case 'u':
				unique = true
			case 'n':
				numeric = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("sort: invalid option -- '%s'", unknown)
		}
	}

	lines, err := readLines(ctx, hc, sc, files)
	if err != nil {
		return err
	}

	sort.SliceStable(lines, func(i, j int) bool {
		if numeric {
			ni, _ := strconv.Atoi(strings.TrimSpace(lines[i]))
			nj, _ := strconv.Atoi(strings.TrimSpace(lines[j]))
			if reverse {
				return ni > nj
			}
			return ni < nj
		}
		if reverse {
			return lines[i] > lines[j]
		}
		return lines[i] < lines[j]
	})

	if unique {
		deduped := lines[:0]
		for i, line := range lines {
			if i == 0 || line != lines[i-1] {
				deduped = append(deduped, line)
			}
		}
		lines = deduped
	}

	for _, line := range lines {
		fmt.Fprintln(hc.Stdout, line)
	}
	return nil
}
