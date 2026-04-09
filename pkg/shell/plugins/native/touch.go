package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"os"
	"time"
)

type TouchPlugin struct{}

func (TouchPlugin) Name() string                                 { return "touch" }
func (TouchPlugin) Description() string                          { return "create or update file timestamps" }
func (TouchPlugin) Usage() string                                { return "touch [-c] [-r ref] <file>..." }
func (TouchPlugin) Run(ctx context.Context, args []string) error { return runTouch(ctx, args) }

func runTouch(ctx context.Context, args []string) error {
	sc := plugins.ShellCtx(ctx)
	noCreate := false
	reference := ""
	var targets []string
	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			targets = append(targets, args[i+1:]...)
			break
		}
		if a == "" || a[0] != '-' || a == "-" {
			targets = append(targets, a)
			continue
		}
		if a == "-r" {
			if i+1 >= len(args) {
				return fmt.Errorf("touch: missing operand for -r")
			}
			i++
			reference = args[i]
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'c':
				noCreate = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("touch: invalid option -- '%s'", unknown)
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("touch: missing file operand")
	}
	var refTime time.Time
	if reference != "" {
		refInfo, err := sc.FS.Stat(sc.ResolvePath(reference))
		if err != nil {
			return fmt.Errorf("touch: cannot stat '%s': %w", reference, err)
		}
		refTime = refInfo.ModTime()
	}
	for _, target := range targets {
		absPath := sc.ResolvePath(target)
		t := time.Now()
		if !refTime.IsZero() {
			t = refTime
		}
		if err := sc.FS.Chtimes(absPath, t, t); err != nil {
			if os.IsNotExist(err) {
				if noCreate {
					continue
				}
				f, err := sc.FS.Create(absPath)
				if err != nil {
					return fmt.Errorf("touch: cannot touch '%s': %w", target, err)
				}
				f.Close()
			} else {
				return fmt.Errorf("touch: cannot touch '%s': %w", target, err)
			}
		}
	}
	return nil
}
