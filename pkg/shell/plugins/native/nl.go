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

// NlPlugin implements `nl` — number lines of files.
//
//	nl [-b style] [-n format] [-w width] [-v start] [-s sep] [FILE...]
type NlPlugin struct{}

func (NlPlugin) Name() string        { return "nl" }
func (NlPlugin) Description() string { return "number lines of files" }
func (NlPlugin) Usage() string       { return "nl [-b a|t|n] [-n ln|rn|rz] [-w width] [-v start] [-s sep] [FILE...]" }

func (NlPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	bodyStyle := "t"  // t=non-empty, a=all, n=none
	numFormat := "rn" // rn=right no-zero, ln=left, rz=right zero-padded
	width := 6
	startNum := 1
	sep := "\t"
	var files []string

	i := 1
	for i < len(args) {
		switch args[i] {
		case "--":
			i++
			goto doneFlags
		case "-b":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "nl: -b requires an argument")
				return interp.ExitStatus(1)
			}
			bodyStyle = args[i+1]
			i += 2
		case "-n":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "nl: -n requires an argument")
				return interp.ExitStatus(1)
			}
			numFormat = args[i+1]
			i += 2
		case "-w":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "nl: -w requires an argument")
				return interp.ExitStatus(1)
			}
			fmt.Sscanf(args[i+1], "%d", &width)
			i += 2
		case "-v":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "nl: -v requires an argument")
				return interp.ExitStatus(1)
			}
			fmt.Sscanf(args[i+1], "%d", &startNum)
			i += 2
		case "-s":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "nl: -s requires an argument")
				return interp.ExitStatus(1)
			}
			sep = args[i+1]
			i += 2
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(hc.Stderr, "nl: invalid option -- %q\n", args[i])
				return interp.ExitStatus(1)
			}
			files = append(files, args[i])
			i++
		}
	}
doneFlags:
	files = append(files, args[i:]...)

	numberLines := func(r io.Reader, lineNum *int) error {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			shouldNumber := false
			switch bodyStyle {
			case "a":
				shouldNumber = true
			case "t":
				shouldNumber = strings.TrimSpace(line) != ""
			case "n":
				shouldNumber = false
			}

			if shouldNumber {
				var numStr string
				switch numFormat {
				case "ln":
					numStr = fmt.Sprintf("%-*d", width, *lineNum)
				case "rz":
					numStr = fmt.Sprintf("%0*d", width, *lineNum)
				default: // rn
					numStr = fmt.Sprintf("%*d", width, *lineNum)
				}
				fmt.Fprintf(hc.Stdout, "%s%s%s\n", numStr, sep, line)
				*lineNum++
			} else {
				fmt.Fprintf(hc.Stdout, "%s%s\n", strings.Repeat(" ", width), line)
			}
		}
		return scanner.Err()
	}

	lineNum := startNum
	if len(files) == 0 {
		return numberLines(hc.Stdin, &lineNum)
	}

	for _, name := range files {
		path := sc.ResolvePath(name)
		f, err := sc.FS.Open(path)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "nl: %s: %v\n", name, err)
			return interp.ExitStatus(1)
		}
		err = numberLines(f, &lineNum)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

var _ plugins.PluginInfo = NlPlugin{}
