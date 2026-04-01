package native

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/shell/plugins"
)

// YqPlugin processes YAML (and JSON) using gojq.
// Input is parsed as YAML (a superset of JSON), the jq expression is applied,
// and results are written back as YAML by default or JSON with -j.
//
//	yq [options] <expr> [file...]
//	yq .name data.yaml
//	echo 'name: alice' | yq .name
type YqPlugin struct{}

func (YqPlugin) Name() string        { return "yq" }
func (YqPlugin) Description() string { return "command-line YAML/JSON processor" }
func (YqPlugin) Usage() string {
	return "yq [-j] [-c] [-r] [-n] <expr> [file...]"
}

func (YqPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	jsonOutput := false
	compact := false
	rawOutput := false
	nullInput := false
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
		switch a {
		case "--json-output", "--output-format=json":
			jsonOutput = true
			continue
		case "--compact-output":
			compact = true
			continue
		case "--raw-output":
			rawOutput = true
			continue
		case "--null-input":
			nullInput = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'j':
				jsonOutput = true
			case 'c':
				compact = true
			case 'r':
				rawOutput = true
			case 'n':
				nullInput = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("yq: invalid option -- '%s'", unknown)
		}
	}

	if expr == "" {
		expr = "."
	}

	query, err := gojq.Parse(expr)
	if err != nil {
		return fmt.Errorf("yq: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return fmt.Errorf("yq: %w", err)
	}

	emit := func(out any) error {
		if err, ok := out.(error); ok {
			return fmt.Errorf("yq: %w", err)
		}
		if rawOutput {
			if s, ok := out.(string); ok {
				fmt.Fprintln(hc.Stdout, s)
				return nil
			}
		}
		if jsonOutput {
			var b []byte
			if compact {
				b, err = json.Marshal(out)
			} else {
				b, err = json.MarshalIndent(out, "", "  ")
			}
			if err != nil {
				return err
			}
			fmt.Fprintln(hc.Stdout, string(b))
			return nil
		}
		// YAML output
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(out); err != nil {
			return err
		}
		_ = enc.Close()
		// yaml.Encoder appends a trailing "---\n" separator on Close for
		// multi-doc streams; trim the document-end marker for single values.
		result := strings.TrimRight(buf.String(), "\n")
		fmt.Fprintln(hc.Stdout, result)
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

	// parseYAMLDocs decodes all YAML documents from r into []any.
	// Each document may contain a single value or a map/sequence.
	parseYAMLDocs := func(r io.Reader) ([]any, error) {
		dec := yaml.NewDecoder(r)
		var docs []any
		for {
			var raw any
			if err := dec.Decode(&raw); err != nil {
				if err == io.EOF {
					break
				}
				return nil, fmt.Errorf("yq: invalid YAML: %w", err)
			}
			// yaml.v3 decodes maps as map[string]any but gojq needs
			// map[string]any — normalise through JSON round-trip so that
			// types are consistent with what jq expects.
			normalised, err := normaliseViaJSON(raw)
			if err != nil {
				return nil, err
			}
			docs = append(docs, normalised)
		}
		return docs, nil
	}

	runDocs := func(docs []any) error {
		for _, doc := range docs {
			if err := runOn(doc); err != nil {
				return err
			}
		}
		return nil
	}

	if len(files) == 0 {
		docs, err := parseYAMLDocs(hc.Stdin)
		if err != nil {
			return err
		}
		return runDocs(docs)
	}

	for _, f := range files {
		data, err := afero.ReadFile(sc.FS, sc.ResolvePath(f))
		if err != nil {
			return fmt.Errorf("yq: %s: %w", f, err)
		}
		docs, err := parseYAMLDocs(newBytesReader(data))
		if err != nil {
			return err
		}
		if err := runDocs(docs); err != nil {
			return err
		}
	}
	return nil
}

// normaliseViaJSON round-trips a value through JSON so that yaml.v3 types
// (e.g. map[string]interface{}) become the plain Go types gojq expects.
func normaliseViaJSON(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// newBytesReader wraps a []byte in an io.Reader (avoids importing bytes in
// every caller).
func newBytesReader(b []byte) io.Reader {
	return bytes.NewReader(b)
}

// ensure YqPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = YqPlugin{}
