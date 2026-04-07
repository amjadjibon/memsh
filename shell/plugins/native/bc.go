package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type BcPlugin struct{}

func (BcPlugin) Name() string                                 { return "bc" }
func (BcPlugin) Description() string                          { return "arbitrary-precision calculator" }
func (BcPlugin) Usage() string                                { return "bc [-l] [-q] [file...]" }
func (BcPlugin) Run(ctx context.Context, args []string) error { return runBc(ctx, args) }

func runBc(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	mathLib := false
	quiet := false
	var files []string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			files = append(files, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		switch a {
		case "--mathlib":
			mathLib = true
		case "--quiet", "--warn":
			quiet = true
		default:
			unknown := ""
			for _, c := range a[1:] {
				switch c {
				case 'l':
					mathLib = true
				case 'q':
					quiet = true
				case 'w':
					// warn about extensions — no-op
				default:
					unknown += string(c)
				}
			}
			if unknown != "" {
				return fmt.Errorf("bc: invalid option -- '%s'", unknown)
			}
		}
	}

	scale := 0
	if mathLib {
		scale = 6
	}

	if !quiet {
		fmt.Fprintln(hc.Stdout, "bc (memsh) 1.0")
	}

	// process files, then stdin if no files given
	if len(files) == 0 {
		return bcRunReader(hc, hc.Stdin, "-", scale)
	}
	for _, f := range files {
		abs := sc.ResolvePath(f)
		fh, err := sc.FS.Open(abs)
		if err != nil {
			return fmt.Errorf("bc: %s: %w", f, err)
		}
		err = bcRunReader(hc, fh, f, scale)
		fh.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
