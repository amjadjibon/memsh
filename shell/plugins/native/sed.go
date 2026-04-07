package native

import (
	"context"
	"fmt"
	"github.com/amjadjibon/memsh/shell/plugins"
	"mvdan.cc/sh/v3/interp"
	"regexp"
	"strings"
)

type SedPlugin struct{}

func (SedPlugin) Name() string                                 { return "sed" }
func (SedPlugin) Description() string                          { return "stream editor (substitution)" }
func (SedPlugin) Usage() string                                { return "sed 's/pattern/replacement/[g]' [file]" }
func (SedPlugin) Run(ctx context.Context, args []string) error { return runSed(ctx, args) }

func runSed(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	if len(args) < 2 {
		return fmt.Errorf("sed: missing expression")
	}

	expr := args[1]
	files := args[2:]
	if !strings.HasPrefix(expr, "s") || len(expr) < 3 {
		return fmt.Errorf("sed: unsupported expression '%s' (only s/// is supported)", expr)
	}

	sep := expr[1]
	parts := strings.Split(expr[2:], string(sep))
	if len(parts) < 2 {
		return fmt.Errorf("sed: invalid substitution expression")
	}
	pattern := parts[0]
	replacement := parts[1]
	global := len(parts) > 2 && strings.Contains(parts[len(parts)-1], "g")

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("sed: invalid pattern: %w", err)
	}

	lines, err := readLines(ctx, hc, sc, files)
	if err != nil {
		return err
	}

	for _, line := range lines {
		out := line
		if global {
			out = re.ReplaceAllString(line, replacement)
		} else if loc := re.FindStringIndex(line); loc != nil {
			out = line[:loc[0]] + re.ReplaceAllString(line[loc[0]:loc[1]], replacement) + line[loc[1]:]
		}
		fmt.Fprintln(hc.Stdout, out)
	}
	return nil
}
