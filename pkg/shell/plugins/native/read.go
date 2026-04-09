package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
	"strings"
)

type ReadPlugin struct{}

func (ReadPlugin) Name() string                                 { return "read" }
func (ReadPlugin) Description() string                          { return "read a line from stdin" }
func (ReadPlugin) Usage() string                                { return "read [var]..." }
func (ReadPlugin) Run(ctx context.Context, args []string) error { return runRead(ctx, args) }

func runRead(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)
	varNames := args[1:]
	if len(varNames) == 0 {
		varNames = []string{"REPLY"}
	}
	scanner := scanWithContext(ctx, hc.Stdin)
	if !scanner.Scan() {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("read: no input")
	}
	fields := strings.Fields(scanner.Text())
	for i, name := range varNames {
		if i < len(fields) {
			if i == len(varNames)-1 && len(fields) > len(varNames) {
				sc.SetEnv(name, strings.Join(fields[i:], " "))
			} else {
				sc.SetEnv(name, fields[i])
			}
		} else {
			sc.SetEnv(name, "")
		}
	}
	return nil
}
