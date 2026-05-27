package native

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// RevPlugin implements `rev` — reverse characters in each line.
type RevPlugin struct{}

func (RevPlugin) Name() string        { return "rev" }
func (RevPlugin) Description() string { return "reverse characters in each line" }
func (RevPlugin) Usage() string       { return "rev [FILE...]" }

func (RevPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	reverseLine := func(r io.Reader) error {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			runes := []rune(scanner.Text())
			for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
				runes[i], runes[j] = runes[j], runes[i]
			}
			fmt.Fprintln(hc.Stdout, string(runes))
		}
		return scanner.Err()
	}

	files := args[1:]
	if len(files) == 0 {
		return reverseLine(hc.Stdin)
	}

	for _, name := range files {
		path := sc.ResolvePath(name)
		f, err := sc.FS.Open(path)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "rev: %s: %v\n", name, err)
			continue
		}
		err = reverseLine(f)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

var _ plugins.PluginInfo = RevPlugin{}
