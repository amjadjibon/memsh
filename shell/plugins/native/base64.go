// Package native contains the built-in native Go plugins shipped with memsh.
package native

import (
	"context"
	enc "encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/amjadjibon/memsh/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// Base64Plugin encodes or decodes base64 data.
//
//	base64 [data...]        encode positional args (or stdin when none given)
//	base64 -d [data...]     decode
type Base64Plugin struct{}

func (Base64Plugin) Name() string        { return "base64" }
func (Base64Plugin) Description() string { return "encode or decode base64 data" }
func (Base64Plugin) Usage() string       { return "base64 [-d|--decode] [data...]" }

func (Base64Plugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	decode := false
	var dataArgs []string

	for _, a := range args[1:] {
		switch a {
		case "-d", "--decode":
			decode = true
		default:
			dataArgs = append(dataArgs, a)
		}
	}

	var input []byte
	if len(dataArgs) > 0 {
		input = []byte(strings.Join(dataArgs, " "))
	} else {
		var err error
		input, err = io.ReadAll(hc.Stdin)
		if err != nil {
			return err
		}
	}

	if decode {
		src := strings.TrimSpace(string(input))
		decoded, err := enc.StdEncoding.DecodeString(src)
		if err != nil {
			decoded, err = enc.URLEncoding.DecodeString(src)
			if err != nil {
				return fmt.Errorf("base64: invalid input")
			}
		}
		_, err = hc.Stdout.Write(decoded)
		return err
	}

	fmt.Fprintln(hc.Stdout, enc.StdEncoding.EncodeToString(input))
	return nil
}

// ensure Base64Plugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = Base64Plugin{}
