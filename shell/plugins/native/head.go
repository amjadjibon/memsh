package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/shell/plugins"
	"io"
	"mvdan.cc/sh/v3/interp"
	"strconv"
	"strings"
)

type HeadPlugin struct{}

func (HeadPlugin) Name() string                                 { return "head" }
func (HeadPlugin) Description() string                          { return "print first lines of a file" }
func (HeadPlugin) Usage() string                                { return "head [-n N] [-c N] [file]" }
func (HeadPlugin) Run(ctx context.Context, args []string) error { return runHead(ctx, args) }

func runHead(ctx context.Context, args []string) error {
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
				return fmt.Errorf("head: option requires an argument -- 'n'")
			}
			i++
			v, err := strconv.Atoi(args[i])
			if err != nil || v < 0 {
				return fmt.Errorf("head: invalid number of lines: '%s'", args[i])
			}
			n = v
		case strings.HasPrefix(a, "-n"):
			v, err := strconv.Atoi(a[2:])
			if err != nil || v < 0 {
				return fmt.Errorf("head: invalid number of lines: '%s'", a[2:])
			}
			n = v
		case a == "-c":
			if i+1 >= len(args) {
				return fmt.Errorf("head: option requires an argument -- 'c'")
			}
			i++
			byteMode = true
			v, err := strconv.Atoi(args[i])
			if err != nil || v < 0 {
				return fmt.Errorf("head: invalid number of bytes: '%s'", args[i])
			}
			byteCount = v
		case strings.HasPrefix(a, "-c"):
			byteMode = true
			v, err := strconv.Atoi(a[2:])
			if err != nil || v < 0 {
				return fmt.Errorf("head: invalid number of bytes: '%s'", a[2:])
			}
			byteCount = v
		default:
			files = append(files, a)
		}
	}

	readHead := func(r io.Reader) error {
		if byteMode {
			_, err := io.CopyN(hc.Stdout, r, int64(byteCount))
			if err != nil && err != io.EOF {
				return err
			}
			return nil
		}
		return headLines(r, hc.Stdout, n)
	}

	if len(files) == 0 {
		return readHead(hc.Stdin)
	}

	for _, f := range files {
		r, err := sc.FS.Open(sc.ResolvePath(f))
		if err != nil {
			return fmt.Errorf("head: %s: %w", f, err)
		}
		err = readHead(r)
		r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
