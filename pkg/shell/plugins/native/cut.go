package native

import (
	"context"
	"fmt"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type CutPlugin struct{}

func (CutPlugin) Name() string        { return "cut" }
func (CutPlugin) Description() string { return "extract fields or characters" }
func (CutPlugin) Usage() string {
	return "cut -d <delim> -f <fields> [file]\n       cut -c <chars> [file]"
}
func (CutPlugin) Run(ctx context.Context, args []string) error { return runCut(ctx, args) }

func runCut(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	delim := "\t"
	var fieldList, charList string
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
		if a == "-d" || a == "--delimiter" {
			if i+1 >= len(args) {
				return fmt.Errorf("cut: missing operand for -d")
			}
			i++
			delim = args[i]
			continue
		}
		if a == "-f" || a == "--fields" {
			if i+1 >= len(args) {
				return fmt.Errorf("cut: missing operand for -f")
			}
			i++
			fieldList = args[i]
			continue
		}
		if a == "-c" || a == "--characters" {
			if i+1 >= len(args) {
				return fmt.Errorf("cut: missing operand for -c")
			}
			i++
			charList = args[i]
			continue
		}
		if a == "-s" || a == "--only-delimited" {
			continue
		}
		return fmt.Errorf("cut: invalid option -- '%s'", a)
	}

	if fieldList == "" && charList == "" {
		return fmt.Errorf("cut: must specify -f or -c")
	}

	lines, err := readLines(ctx, hc, sc, files)
	if err != nil {
		return err
	}

	for _, line := range lines {
		if charList != "" {
			runes := []rune(line)
			indices, parseErr := parseRangeList(charList, len(runes))
			if parseErr != nil {
				return fmt.Errorf("cut: invalid character range: %v", parseErr)
			}
			var out []rune
			for _, idx := range indices {
				if idx < len(runes) {
					out = append(out, runes[idx])
				}
			}
			fmt.Fprintln(hc.Stdout, string(out))
			continue
		}

		parts := strings.Split(line, delim)
		indices, parseErr := parseRangeList(fieldList, len(parts))
		if parseErr != nil {
			return fmt.Errorf("cut: invalid field range: %v", parseErr)
		}
		var selected []string
		for _, idx := range indices {
			if idx < len(parts) {
				selected = append(selected, parts[idx])
			}
		}
		fmt.Fprintln(hc.Stdout, strings.Join(selected, delim))
	}
	return nil
}
