package native

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// StringsPlugin implements the `strings` command.
// It scans binary input and prints sequences of printable characters that are
// at least minLen bytes long, one per line — useful for inspecting binaries.
//
//	strings [-n min] [-t format] [file...]
type StringsPlugin struct{}

func (StringsPlugin) Name() string        { return "strings" }
func (StringsPlugin) Description() string { return "extract printable strings from binary data" }
func (StringsPlugin) Usage() string       { return "strings [-n minlen] [-t d|o|x] [file...]" }

func (StringsPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	minLen := 4
	offsetFmt := "" // "", "d" (decimal), "o" (octal), "x" (hex)
	var files []string

	i := 1
	for i < len(args) {
		switch args[i] {
		case "--":
			i++
			goto doneFlags
		case "-n", "--bytes":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "strings: -n requires an argument")
				return interp.ExitStatus(1)
			}
			if _, err := fmt.Sscanf(args[i+1], "%d", &minLen); err != nil || minLen < 1 {
				fmt.Fprintf(hc.Stderr, "strings: invalid minimum length %q\n", args[i+1])
				return interp.ExitStatus(1)
			}
			i += 2
		case "-t", "--radix":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "strings: -t requires an argument")
				return interp.ExitStatus(1)
			}
			offsetFmt = args[i+1]
			if offsetFmt != "d" && offsetFmt != "o" && offsetFmt != "x" {
				fmt.Fprintf(hc.Stderr, "strings: invalid radix %q (use d, o, or x)\n", offsetFmt)
				return interp.ExitStatus(1)
			}
			i += 2
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(hc.Stderr, "strings: invalid option -- %q\n", args[i])
				return interp.ExitStatus(1)
			}
			files = append(files, args[i])
			i++
		}
	}
doneFlags:
	files = append(files, args[i:]...)

	if len(files) == 0 {
		// Read from stdin.
		data, err := io.ReadAll(hc.Stdin)
		if err != nil {
			return err
		}
		printStrings(hc.Stdout, data, minLen, offsetFmt)
		return nil
	}

	for _, f := range files {
		path := sc.ResolvePath(f)
		data, err := func() ([]byte, error) {
			fh, err := sc.FS.Open(path)
			if err != nil {
				return nil, err
			}
			defer fh.Close()
			return io.ReadAll(fh)
		}()
		if err != nil {
			fmt.Fprintf(hc.Stderr, "strings: %s: %v\n", f, err)
			continue
		}
		printStrings(hc.Stdout, data, minLen, offsetFmt)
	}
	return nil
}

// printStrings scans data for runs of printable ASCII chars >= minLen long.
func printStrings(w io.Writer, data []byte, minLen int, offsetFmt string) {
	var run strings.Builder
	offset := 0
	runStart := 0

	flush := func() {
		if run.Len() >= minLen {
			if offsetFmt != "" {
				switch offsetFmt {
				case "d":
					fmt.Fprintf(w, "%7d %s\n", runStart, run.String())
				case "o":
					fmt.Fprintf(w, "%7o %s\n", runStart, run.String())
				case "x":
					fmt.Fprintf(w, "%7x %s\n", runStart, run.String())
				}
			} else {
				fmt.Fprintln(w, run.String())
			}
		}
		run.Reset()
	}

	for _, b := range data {
		r := rune(b)
		if b >= 0x20 && b < 0x7f || unicode.IsPrint(r) && b < 0x80 {
			if run.Len() == 0 {
				runStart = offset
			}
			run.WriteByte(b)
		} else {
			flush()
		}
		offset++
	}
	flush()
}

var _ plugins.PluginInfo = StringsPlugin{}
