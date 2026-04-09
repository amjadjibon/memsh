package native

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type TailPlugin struct{}

func (TailPlugin) Name() string                                 { return "tail" }
func (TailPlugin) Description() string                          { return "print last lines of a file" }
func (TailPlugin) Usage() string                                { return "tail [-n N] [-c N] [file]" }
func (TailPlugin) Run(ctx context.Context, args []string) error { return runTail(ctx, args) }

func runTail(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	n := 10
	byteMode := false
	byteCount := 0
	var files []string

	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-n":
			if i+1 >= len(args) {
				return fmt.Errorf("tail: option requires an argument -- 'n'")
			}
			i++
			v, err := strconv.Atoi(args[i])
			if err != nil || v < 0 {
				return fmt.Errorf("tail: invalid number of lines: '%s'", args[i])
			}
			n = v
		case strings.HasPrefix(a, "-n"):
			v, err := strconv.Atoi(a[2:])
			if err != nil || v < 0 {
				return fmt.Errorf("tail: invalid number of lines: '%s'", a[2:])
			}
			n = v
		case a == "-c":
			if i+1 >= len(args) {
				return fmt.Errorf("tail: option requires an argument -- 'c'")
			}
			i++
			byteMode = true
			v, err := strconv.Atoi(args[i])
			if err != nil || v < 0 {
				return fmt.Errorf("tail: invalid number of bytes: '%s'", args[i])
			}
			byteCount = v
		case strings.HasPrefix(a, "-c"):
			byteMode = true
			v, err := strconv.Atoi(a[2:])
			if err != nil || v < 0 {
				return fmt.Errorf("tail: invalid number of bytes: '%s'", a[2:])
			}
			byteCount = v
		default:
			files = append(files, a)
		}
	}

	readTail := func(r io.Reader) error {
		if byteMode {
			data, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			start := len(data) - byteCount
			if start < 0 {
				start = 0
			}
			_, err = hc.Stdout.Write(data[start:])
			return err
		}
		return tailLines(r, hc.Stdout, n)
	}

	if len(files) == 0 {
		return readTail(hc.Stdin)
	}

	for _, f := range files {
		r, err := sc.FS.Open(sc.ResolvePath(f))
		if err != nil {
			return fmt.Errorf("tail: %s: %w", f, err)
		}
		err = readTail(r)
		r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
