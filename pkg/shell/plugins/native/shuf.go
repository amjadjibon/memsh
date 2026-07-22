package native

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// ShufPlugin implements `shuf` — shuffle lines randomly.
//
//	shuf [-n count] [-o outfile] [-e args...] [-i lo-hi] [FILE]
type ShufPlugin struct{}

func (ShufPlugin) Name() string        { return "shuf" }
func (ShufPlugin) Description() string { return "generate random permutations of lines" }
func (ShufPlugin) Usage() string       { return "shuf [-n count] [-e args...] [-i lo-hi] [FILE]" }

func (ShufPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	count := -1 // -1 = all
	var echoArgs []string
	rangeMode := false
	rangeLo, rangeHi := 0, 0
	var inputFile string

	i := 1
	for i < len(args) {
		switch args[i] {
		case "--":
			goto doneFlags
		case "-n", "--head-count":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "shuf: -n requires an argument")
				return interp.ExitStatus(1)
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 0 {
				fmt.Fprintf(hc.Stderr, "shuf: invalid count %q\n", args[i+1])
				return interp.ExitStatus(1)
			}
			count = n
			i += 2
		case "-e", "--echo":
			// All remaining args before next flag are the echo set.
			i++
			for i < len(args) && !strings.HasPrefix(args[i], "-") {
				echoArgs = append(echoArgs, args[i])
				i++
			}
		case "-i", "--input-range":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "shuf: -i requires LO-HI argument")
				return interp.ExitStatus(1)
			}
			parts := strings.SplitN(args[i+1], "-", 2)
			if len(parts) != 2 {
				fmt.Fprintf(hc.Stderr, "shuf: invalid range %q\n", args[i+1])
				return interp.ExitStatus(1)
			}
			var err error
			rangeLo, err = strconv.Atoi(parts[0])
			if err != nil {
				fmt.Fprintf(hc.Stderr, "shuf: invalid range %q\n", args[i+1])
				return interp.ExitStatus(1)
			}
			rangeHi, err = strconv.Atoi(parts[1])
			if err != nil || rangeHi < rangeLo {
				fmt.Fprintf(hc.Stderr, "shuf: invalid range %q\n", args[i+1])
				return interp.ExitStatus(1)
			}
			rangeMode = true
			i += 2
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(hc.Stderr, "shuf: invalid option -- %q\n", args[i])
				return interp.ExitStatus(1)
			}
			inputFile = args[i]
			i++
		}
	}
doneFlags:

	var lines []string

	switch {
	case len(echoArgs) > 0:
		lines = echoArgs
	case rangeMode:
		for n := rangeLo; n <= rangeHi; n++ {
			lines = append(lines, strconv.Itoa(n))
		}
	default:
		var r io.Reader
		if inputFile != "" {
			path := sc.ResolvePath(inputFile)
			f, err := sc.FS.Open(path)
			if err != nil {
				fmt.Fprintf(hc.Stderr, "shuf: %s: %v\n", inputFile, err)
				return interp.ExitStatus(1)
			}
			defer f.Close()
			r = f
		} else {
			r = hc.Stdin
		}
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return err
		}
	}

	rand.Shuffle(len(lines), func(i, j int) { lines[i], lines[j] = lines[j], lines[i] })

	if count >= 0 && count < len(lines) {
		lines = lines[:count]
	}

	for _, l := range lines {
		fmt.Fprintln(hc.Stdout, l)
	}
	return nil
}

var _ plugins.PluginInfo = ShufPlugin{}
