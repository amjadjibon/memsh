package native

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/spf13/afero"
)

type ChmodPlugin struct{}

func (ChmodPlugin) Name() string                                 { return "chmod" }
func (ChmodPlugin) Description() string                          { return "change file permissions" }
func (ChmodPlugin) Usage() string                                { return "chmod [-R] <mode> <file>..." }
func (ChmodPlugin) Run(ctx context.Context, args []string) error { return runChmod(ctx, args) }

func runChmod(ctx context.Context, args []string) error {
	sc := plugins.ShellCtx(ctx)
	recursive := false
	endOfFlags := false
	var positional []string
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			positional = append(positional, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		if a == "--recursive" {
			recursive = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'R':
				recursive = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("chmod: invalid option -- '%s'", unknown)
		}
	}
	if len(positional) < 2 {
		return fmt.Errorf("chmod: missing operand")
	}
	modeStr := positional[0]
	targets := positional[1:]
	var mode os.FileMode
	if strings.ContainsAny(modeStr, "ugoa+-=rwx") {
		m, err := parseSymbolicMode(modeStr)
		if err != nil {
			return fmt.Errorf("chmod: invalid mode '%s': %w", modeStr, err)
		}
		mode = m
	} else {
		v, err := strconv.ParseUint(modeStr, 8, 32)
		if err != nil {
			return fmt.Errorf("chmod: invalid mode '%s'", modeStr)
		}
		mode = os.FileMode(v)
	}
	for _, target := range targets {
		absPath := sc.ResolvePath(target)
		if recursive {
			if err := afero.Walk(sc.FS, absPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				return sc.FS.Chmod(path, mode)
			}); err != nil {
				return fmt.Errorf("chmod: cannot chmod '%s': %w", target, err)
			}
		} else if err := sc.FS.Chmod(absPath, mode); err != nil {
			return fmt.Errorf("chmod: cannot chmod '%s': %w", target, err)
		}
	}
	return nil
}

func parseSymbolicMode(spec string) (os.FileMode, error) {
	mode := os.FileMode(0644)
	for _, part := range strings.Split(spec, ",") {
		who, op, perm := "", "", ""
		i := 0
		for i < len(part) && strings.ContainsRune("ugoa", rune(part[i])) {
			who += string(part[i])
			i++
		}
		if who == "" {
			who = "a"
		}
		if i >= len(part) {
			return 0, fmt.Errorf("invalid mode")
		}
		op = string(part[i])
		i++
		for i < len(part) && strings.ContainsRune("rwx", rune(part[i])) {
			perm += string(part[i])
			i++
		}
		for _, w := range who {
			for _, p := range perm {
				bit := os.FileMode(0)
				switch p {
				case 'r':
					switch w {
					case 'u':
						bit = 0400
					case 'g':
						bit = 0040
					case 'o':
						bit = 0004
					case 'a':
						bit = 0444
					}
				case 'w':
					switch w {
					case 'u':
						bit = 0200
					case 'g':
						bit = 0020
					case 'o':
						bit = 0002
					case 'a':
						bit = 0222
					}
				case 'x':
					switch w {
					case 'u':
						bit = 0100
					case 'g':
						bit = 0010
					case 'o':
						bit = 0001
					case 'a':
						bit = 0111
					}
				}
				switch op {
				case "+":
					mode |= bit
				case "-":
					mode &^= bit
				case "=":
					mask := os.FileMode(0)
					switch w {
					case 'u':
						mask = 0700
					case 'g':
						mask = 0070
					case 'o':
						mask = 0007
					case 'a':
						mask = 0777
					}
					mode = (mode &^ mask) | bit
				}
			}
		}
	}
	return mode, nil
}
