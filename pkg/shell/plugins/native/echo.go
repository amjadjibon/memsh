package native

import (
	"context"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/interp"
)

type EchoPlugin struct{}

func (EchoPlugin) Name() string                                 { return "echo" }
func (EchoPlugin) Description() string                          { return "print arguments" }
func (EchoPlugin) Usage() string                                { return "echo [-n] [-e] [arg]..." }
func (EchoPlugin) Run(ctx context.Context, args []string) error { return runEcho(ctx, args) }

func runEcho(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	noNewline, interpretEsc := false, false
	var parts []string
	skipFlags := true
	for _, arg := range args[1:] {
		if skipFlags {
			if arg == "-n" {
				noNewline = true
				continue
			}
			if arg == "-e" {
				interpretEsc = true
				continue
			}
			if arg == "-ne" || arg == "-en" {
				noNewline = true
				interpretEsc = true
				continue
			}
			if arg == "-E" {
				continue
			}
			if strings.HasPrefix(arg, "-") && len(arg) > 1 {
				hasFlags := true
				for _, c := range arg[1:] {
					if c != 'n' && c != 'e' && c != 'E' {
						hasFlags = false
						break
					}
				}
				if hasFlags {
					for _, c := range arg[1:] {
						switch c {
						case 'n':
							noNewline = true
						case 'e':
							interpretEsc = true
						}
					}
					continue
				}
			}
		}
		skipFlags = false
		parts = append(parts, arg)
	}
	text := strings.Join(parts, " ")
	if interpretEsc {
		text = expandEscapeSequences(text)
	}
	if noNewline {
		fmt.Fprint(hc.Stdout, text)
	} else {
		fmt.Fprintln(hc.Stdout, text)
	}
	return nil
}
