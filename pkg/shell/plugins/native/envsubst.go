package native

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

// envVarRe matches $VAR and ${VAR} (POSIX variable names: [A-Za-z_][A-Za-z0-9_]*).
var envVarRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// EnvsubstPlugin substitutes environment variable references in text.
//
//	envsubst                    read stdin, replace all $VAR / ${VAR}
//	envsubst '$VAR1 $VAR2'      only substitute the listed variables
//	envsubst < template.txt
//	cat tmpl.txt | envsubst '$HOME $USER'
type EnvsubstPlugin struct{}

func (EnvsubstPlugin) Name() string        { return "envsubst" }
func (EnvsubstPlugin) Description() string { return "substitute environment variables in text" }
func (EnvsubstPlugin) Usage() string       { return "envsubst ['$VAR ...'] [file...]" }

func (EnvsubstPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	// collect which variables to substitute (empty = all)
	var allowList map[string]bool
	var files []string

	for _, a := range args[1:] {
		if strings.Contains(a, "$") {
			// treat as a variable list: '$FOO $BAR' or '$FOO,$BAR'
			if allowList == nil {
				allowList = make(map[string]bool)
			}
			for _, name := range envVarRe.FindAllString(a, -1) {
				name = strings.TrimPrefix(name, "$")
				name = strings.Trim(name, "{}")
				allowList[name] = true
			}
		} else {
			files = append(files, a)
		}
	}

	subst := func(text string) string {
		return envVarRe.ReplaceAllStringFunc(text, func(match string) string {
			// extract the variable name from $VAR or ${VAR}
			sub := envVarRe.FindStringSubmatch(match)
			name := sub[1]
			if name == "" {
				name = sub[2]
			}
			if allowList != nil && !allowList[name] {
				return match // leave unchanged
			}
			return sc.Env(name)
		})
	}

	process := func(r io.Reader) error {
		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(hc.Stdout, subst(string(data)))
		return err
	}

	if len(files) == 0 {
		return process(hc.Stdin)
	}
	for _, f := range files {
		fh, err := sc.FS.Open(sc.ResolvePath(f))
		if err != nil {
			return fmt.Errorf("envsubst: %s: %w", f, err)
		}
		err = process(fh)
		fh.Close()
		if err != nil {
			return fmt.Errorf("envsubst: %s: %w", f, err)
		}
	}
	return nil
}

// ensure EnvsubstPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = EnvsubstPlugin{}
