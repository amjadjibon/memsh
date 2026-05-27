package native

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// FoldPlugin implements `fold` — wrap long lines at a specified width.
//
//	fold [-w width] [-s] [-b] [FILE...]
type FoldPlugin struct{}

func (FoldPlugin) Name() string        { return "fold" }
func (FoldPlugin) Description() string { return "wrap each input line to fit in specified width" }
func (FoldPlugin) Usage() string       { return "fold [-w width] [-s] [-b] [FILE...]" }

func (FoldPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	width := 80
	breakSpaces := false // -s: break at spaces
	countBytes := false  // -b: count bytes instead of columns
	var files []string

	i := 1
	for i < len(args) {
		switch args[i] {
		case "--":
			i++
			goto doneFlags
		case "-w", "--width":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "fold: -w requires an argument")
				return interp.ExitStatus(1)
			}
			fmt.Sscanf(args[i+1], "%d", &width)
			if width < 1 {
				fmt.Fprintln(hc.Stderr, "fold: invalid width")
				return interp.ExitStatus(1)
			}
			i += 2
		case "-s", "--spaces":
			breakSpaces = true
			i++
		case "-b", "--bytes":
			countBytes = true
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				// handle combined e.g. -w80
				if len(args[i]) > 2 && args[i][1] == 'w' {
					fmt.Sscanf(args[i][2:], "%d", &width)
					i++
					continue
				}
				fmt.Fprintf(hc.Stderr, "fold: invalid option -- %q\n", args[i])
				return interp.ExitStatus(1)
			}
			files = append(files, args[i])
			i++
		}
	}
doneFlags:
	files = append(files, args[i:]...)

	foldLine := func(line string) {
		if countBytes {
			b := []byte(line)
			for len(b) > width {
				pos := width
				if breakSpaces {
					for pos > 0 && b[pos-1] != ' ' {
						pos--
					}
					if pos == 0 {
						pos = width
					}
				}
				fmt.Fprintf(hc.Stdout, "%s\n", b[:pos])
				b = b[pos:]
			}
			fmt.Fprintf(hc.Stdout, "%s\n", b)
			return
		}

		// Rune-based column folding.
		runes := []rune(line)
		for len(runes) > width {
			pos := width
			if breakSpaces {
				for pos > 0 && runes[pos-1] != ' ' {
					pos--
				}
				if pos == 0 {
					pos = width
				}
			}
			fmt.Fprintf(hc.Stdout, "%s\n", string(runes[:pos]))
			runes = runes[pos:]
		}
		fmt.Fprintf(hc.Stdout, "%s\n", string(runes))
	}

	processReader := func(r io.Reader) error {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			foldLine(scanner.Text())
		}
		return scanner.Err()
	}

	if len(files) == 0 {
		return processReader(hc.Stdin)
	}

	for _, name := range files {
		path := sc.ResolvePath(name)
		f, err := sc.FS.Open(path)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "fold: %s: %v\n", name, err)
			return interp.ExitStatus(1)
		}
		err = processReader(f)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

var _ plugins.PluginInfo = FoldPlugin{}
