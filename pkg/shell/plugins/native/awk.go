package native

import (
	"context"
	"fmt"
	"io"
	"strings"

	gawkinterp "github.com/benhoyt/goawk/interp"
	"github.com/benhoyt/goawk/parser"
	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// AwkPlugin runs awk programs via the goawk interpreter.
//
//	awk '<program>' [file...]
//	awk -f <progfile> [file...]
type AwkPlugin struct{}

func (AwkPlugin) Name() string        { return "awk" }
func (AwkPlugin) Description() string { return "pattern scanning and processing language" }
func (AwkPlugin) Usage() string       { return "awk '<prog>' [file...]\n       awk -f <progfile> [file...]" }

func (AwkPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	var progSrc string
	var files []string
	var fieldSep string

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-f":
			if i+1 >= len(args) {
				return fmt.Errorf("awk: -f requires a file argument")
			}
			i++
			data, err := afero.ReadFile(sc.FS, sc.ResolvePath(args[i]))
			if err != nil {
				return fmt.Errorf("awk: cannot read program file '%s': %v", args[i], err)
			}
			progSrc = string(data)
		case "-F":
			if i+1 >= len(args) {
				return fmt.Errorf("awk: -F requires an argument")
			}
			i++
			fieldSep = args[i]
		default:
			// Handle -F<delim> form (e.g., -F:, -F,)
			if len(args[i]) > 2 && args[i][:2] == "-F" {
				fieldSep = args[i][2:]
			} else if progSrc == "" {
				progSrc = args[i]
			} else {
				files = append(files, args[i])
			}
		}
	}

	if progSrc == "" {
		return fmt.Errorf("awk: missing program")
	}

	// If -F was specified, prepend a BEGIN block to set FS
	if fieldSep != "" {
		// Escape special characters in fieldSep for awk string
		escapedFS := strings.ReplaceAll(fieldSep, "\\", "\\\\")
		escapedFS = strings.ReplaceAll(escapedFS, "\"", "\\\"")
		progSrc = "BEGIN{FS=\"" + escapedFS + "\"}" + progSrc
	}

	prog, err := parser.ParseProgram([]byte(progSrc), nil)
	if err != nil {
		return fmt.Errorf("awk: %v", err)
	}

	// For virtual FS file args, read their content and combine into one reader.
	var stdin io.Reader = hc.Stdin
	if len(files) > 0 {
		var readers []io.Reader
		for _, f := range files {
			data, readErr := afero.ReadFile(sc.FS, sc.ResolvePath(f))
			if readErr != nil {
				return fmt.Errorf("awk: %s: %v", f, readErr)
			}
			readers = append(readers, strings.NewReader(string(data)))
		}
		stdin = io.MultiReader(readers...)
	}

	_, execErr := gawkinterp.ExecProgram(prog, &gawkinterp.Config{
		Stdin:  stdin,
		Output: hc.Stdout,
		Error:  hc.Stderr,
	})
	return execErr
}

// ensure AwkPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = AwkPlugin{}
