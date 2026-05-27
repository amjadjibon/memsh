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

// PastePlugin implements `paste` — merge lines of files side by side.
//
//	paste [-d delim] [-s] FILE...
//	paste -      read that column from stdin
type PastePlugin struct{}

func (PastePlugin) Name() string        { return "paste" }
func (PastePlugin) Description() string { return "merge lines of files" }
func (PastePlugin) Usage() string       { return "paste [-d DELIM] [-s] FILE..." }

func (PastePlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	delim := "\t"
	serial := false
	var files []string

	i := 1
	for i < len(args) {
		switch args[i] {
		case "--":
			i++
			goto doneFlags
		case "-d", "--delimiters":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "paste: -d requires an argument")
				return interp.ExitStatus(1)
			}
			delim = args[i+1]
			i += 2
		case "-s", "--serial":
			serial = true
			i++
		default:
			if strings.HasPrefix(args[i], "-") && args[i] != "-" {
				fmt.Fprintf(hc.Stderr, "paste: invalid option -- %q\n", args[i])
				return interp.ExitStatus(1)
			}
			files = append(files, args[i])
			i++
		}
	}
doneFlags:
	files = append(files, args[i:]...)

	if len(files) == 0 {
		files = []string{"-"}
	}

	// Open all readers.
	readers := make([]io.Reader, len(files))
	for i, f := range files {
		if f == "-" {
			readers[i] = hc.Stdin
		} else {
			path := sc.ResolvePath(f)
			fh, err := sc.FS.Open(path)
			if err != nil {
				fmt.Fprintf(hc.Stderr, "paste: %s: %v\n", f, err)
				return interp.ExitStatus(1)
			}
			defer fh.Close()
			readers[i] = fh
		}
	}

	if serial {
		// -s: paste all lines of one file as a single tab-separated line.
		for _, r := range readers {
			scanner := bufio.NewScanner(r)
			var row []string
			for scanner.Scan() {
				row = append(row, scanner.Text())
			}
			fmt.Fprintln(hc.Stdout, strings.Join(row, delim))
		}
		return nil
	}

	// Default: zip lines across all files.
	scanners := make([]*bufio.Scanner, len(readers))
	for i, r := range readers {
		scanners[i] = bufio.NewScanner(r)
	}

	for {
		var cols []string
		alive := false
		for _, s := range scanners {
			if s.Scan() {
				cols = append(cols, s.Text())
				alive = true
			} else {
				cols = append(cols, "")
			}
		}
		if !alive {
			break
		}
		fmt.Fprintln(hc.Stdout, strings.Join(cols, delim))
	}
	return nil
}

var _ plugins.PluginInfo = PastePlugin{}
