package native

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

type TeePlugin struct{}

func (TeePlugin) Name() string                                 { return "tee" }
func (TeePlugin) Description() string                          { return "read stdin; write to stdout and files" }
func (TeePlugin) Usage() string                                { return "tee [-a] [file]..." }
func (TeePlugin) Run(ctx context.Context, args []string) error { return runTee(ctx, args) }

func runTee(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	appendMode := false
	var targets []string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			targets = append(targets, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		if a == "--append" {
			appendMode = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'a':
				appendMode = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("tee: invalid option -- '%s'", unknown)
		}
	}

	writers := []io.Writer{hc.Stdout}
	var toClose []io.Closer
	for _, t := range targets {
		absPath := sc.ResolvePath(t)
		var f afero.File
		var err error
		if appendMode {
			f, err = sc.FS.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		} else {
			f, err = sc.FS.Create(absPath)
		}
		if err != nil {
			return fmt.Errorf("tee: %s: %w", t, err)
		}
		writers = append(writers, f)
		toClose = append(toClose, f)
	}
	defer func() {
		for _, c := range toClose {
			c.Close()
		}
	}()

	_, err := copyWithContext(ctx, io.MultiWriter(writers...), hc.Stdin)
	if ctx.Err() != nil {
		return nil
	}
	return err
}
