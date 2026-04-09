// Package native contains the built-in native Go plugins shipped with memsh.
package native

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/dop251/goja"
	"mvdan.cc/sh/v3/interp"
)

// GojaPlugin executes JavaScript code using goja.
//
//	goja [-e 'code']        execute inline JavaScript code
//	goja [file.js]          execute JavaScript file from virtual filesystem
//	                    (reads JavaScript code from stdin when no args)
type GojaPlugin struct{}

func (GojaPlugin) Name() string        { return "goja" }
func (GojaPlugin) Description() string { return "execute JavaScript code using goja" }
func (GojaPlugin) Usage() string       { return "goja [-e 'code' | file.js]" }

func (GojaPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	var code string

	// Parse arguments
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-e":
			if i+1 < len(args) {
				code = args[i+1]
				i++
			} else {
				return fmt.Errorf("goja: -e requires an argument")
			}
		default:
			// Treat as a file path
			filePath := sc.ResolvePath(args[i])
			f, err := sc.FS.Open(filePath)
			if err != nil {
				return fmt.Errorf("goja: %s: %w", args[i], err)
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("goja: %s: read error: %w", args[i], err)
			}
			code = string(data)
		}
	}

	// If no code provided, read from stdin
	if code == "" {
		input, err := io.ReadAll(hc.Stdin)
		if err != nil {
			return fmt.Errorf("goja: read stdin: %w", err)
		}
		code = string(input)
	}

	// Create JavaScript runtime
	vm := goja.New()

	// Capture console output
	consoleBuf := &strings.Builder{}
	consoleObj := map[string]func(goja.FunctionCall) goja.Value{
		"log": func(call goja.FunctionCall) goja.Value {
			for i, arg := range call.Arguments {
				if i > 0 {
					consoleBuf.WriteString(" ")
				}
				consoleBuf.WriteString(arg.String())
			}
			consoleBuf.WriteString("\n")
			return goja.Undefined()
		},
	}

	if err := vm.Set("console", consoleObj); err != nil {
		return fmt.Errorf("goja: setup console: %w", err)
	}

	// Provide filesystem access via fs module
	fsObj := map[string]func(goja.FunctionCall) goja.Value{
		"readFile": func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return vm.ToValue("fs.readFile requires a path argument")
			}
			path := call.Argument(0).String()
			resolvedPath := sc.ResolvePath(path)
			f, err := sc.FS.Open(resolvedPath)
			if err != nil {
				return vm.ToValue(goja.Null())
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil {
				return vm.ToValue(goja.Null())
			}
			return vm.ToValue(string(data))
		},
	}

	if err := vm.Set("fs", fsObj); err != nil {
		return fmt.Errorf("goja: setup fs: %w", err)
	}

	// Execute the code
	if _, err := vm.RunString(code); err != nil {
		return fmt.Errorf("goja: %w", err)
	}

	// Write captured console output
	if consoleBuf.Len() > 0 {
		if _, err := hc.Stdout.Write([]byte(consoleBuf.String())); err != nil {
			return err
		}
	}

	return nil
}

// ensure GojaPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = GojaPlugin{}
