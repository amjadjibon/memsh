package native

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/itchyny/gojq"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/shell/plugins"
)

// JqPlugin processes JSON using the gojq library.
//
//	jq [options] <expr> [file...]
//	jq .field data.json
//	echo '{"a":1}' | jq .a
type JqPlugin struct{}

func (JqPlugin) Name() string        { return "jq" }
func (JqPlugin) Description() string { return "command-line JSON processor" }
func (JqPlugin) Usage() string {
	return "jq [-c] [-r] [-n] [-e] <expr> [file...]"
}

func (JqPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	compact := false
	rawOutput := false
	nullInput := false
	exitStatus := false
	var expr string
	var files []string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			if expr == "" {
				expr = a
			} else {
				files = append(files, a)
			}
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		// long flags
		switch a {
		case "--compact-output":
			compact = true
			continue
		case "--raw-output":
			rawOutput = true
			continue
		case "--null-input":
			nullInput = true
			continue
		case "--exit-status":
			exitStatus = true
			continue
		case "--join-output":
			rawOutput = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'c':
				compact = true
			case 'r':
				rawOutput = true
			case 'n':
				nullInput = true
			case 'e':
				exitStatus = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("jq: invalid option -- '%s'", unknown)
		}
	}

	if expr == "" {
		expr = "."
	}

	query, err := gojq.Parse(expr)
	if err != nil {
		return fmt.Errorf("jq: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return fmt.Errorf("jq: %w", err)
	}

	marshal := func(v any) ([]byte, error) {
		if compact {
			return json.Marshal(v)
		}
		return json.MarshalIndent(v, "", "  ")
	}

	hasResult := false

	emit := func(out any) error {
		if err, ok := out.(error); ok {
			return fmt.Errorf("jq: %w", err)
		}
		hasResult = true
		if rawOutput {
			if s, ok := out.(string); ok {
				fmt.Fprintln(hc.Stdout, s)
				return nil
			}
		}
		if out == nil {
			fmt.Fprintln(hc.Stdout, "null")
			return nil
		}
		b, err := marshal(out)
		if err != nil {
			return err
		}
		fmt.Fprintln(hc.Stdout, string(b))
		return nil
	}

	runOn := func(input any) error {
		iter := code.Run(input)
		for {
			out, ok := iter.Next()
			if !ok {
				break
			}
			if err := emit(out); err != nil {
				return err
			}
		}
		return nil
	}

	if nullInput {
		return runOn(nil)
	}

	runReader := func(r io.Reader) error {
		dec := json.NewDecoder(r)
		for {
			var v any
			if err := dec.Decode(&v); err != nil {
				if err == io.EOF {
					return nil
				}
				return fmt.Errorf("jq: invalid JSON: %w", err)
			}
			if err := runOn(v); err != nil {
				return err
			}
		}
	}

	if len(files) == 0 {
		if err := runReader(hc.Stdin); err != nil {
			return err
		}
	} else {
		for _, f := range files {
			data, err := afero.ReadFile(sc.FS, sc.ResolvePath(f))
			if err != nil {
				return fmt.Errorf("jq: %s: %w", f, err)
			}
			if err := runReader(newBytesReader(data)); err != nil {
				return err
			}
		}
	}

	if exitStatus && !hasResult {
		return interp.ExitStatus(1)
	}
	return nil
}

// ensure JqPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = JqPlugin{}
