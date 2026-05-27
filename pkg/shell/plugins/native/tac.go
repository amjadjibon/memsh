package native

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// TacPlugin implements `tac` — concatenate files in reverse line order.
type TacPlugin struct{}

func (TacPlugin) Name() string        { return "tac" }
func (TacPlugin) Description() string { return "concatenate and print files in reverse" }
func (TacPlugin) Usage() string       { return "tac [FILE...]" }

func (TacPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	readAndReverse := func(r io.Reader) error {
		var lines []string
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		for i := len(lines) - 1; i >= 0; i-- {
			fmt.Fprintln(hc.Stdout, lines[i])
		}
		return nil
	}

	files := args[1:]
	if len(files) == 0 {
		return readAndReverse(hc.Stdin)
	}

	for _, name := range files {
		path := sc.ResolvePath(name)
		f, err := sc.FS.Open(path)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "tac: %s: %v\n", name, err)
			continue
		}
		err = readAndReverse(f)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

var _ plugins.PluginInfo = TacPlugin{}
