package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type UniqPlugin struct{}

func (UniqPlugin) Name() string                                 { return "uniq" }
func (UniqPlugin) Description() string                          { return "filter adjacent duplicate lines" }
func (UniqPlugin) Usage() string                                { return "uniq [-c] [-d] [-u] [file]" }
func (UniqPlugin) Run(ctx context.Context, args []string) error { return runUniq(ctx, args) }

func runUniq(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	count, onlyDups, onlyUniq := false, false, false
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
		case "--count":
			count = true
			continue
		case "--repeated":
			onlyDups = true
			continue
		case "--unique":
			onlyUniq = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'c':
				count = true
			case 'd':
				onlyDups = true
			case 'u':
				onlyUniq = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("uniq: invalid option -- '%s'", unknown)
		}
	}

	lines, err := readLines(ctx, hc, sc, files)
	if err != nil {
		return err
	}

	type run struct {
		line string
		n    int
	}
	var runs []run
	for _, line := range lines {
		if len(runs) > 0 && runs[len(runs)-1].line == line {
			runs[len(runs)-1].n++
		} else {
			runs = append(runs, run{line: line, n: 1})
		}
	}

	for _, run := range runs {
		isDup := run.n > 1
		if onlyDups && !isDup {
			continue
		}
		if onlyUniq && isDup {
			continue
		}
		if count {
			fmt.Fprintf(hc.Stdout, "%7d %s\n", run.n, run.line)
		} else {
			fmt.Fprintln(hc.Stdout, run.line)
		}
	}
	return nil
}
