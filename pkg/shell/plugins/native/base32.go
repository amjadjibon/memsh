package native

import (
	"context"
	enc "encoding/base32"
	"fmt"
	"io"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// Base32Plugin encodes or decodes base32 data, mirroring the base64 plugin.
//
//	base32 [data...]        encode positional args (or stdin when none given)
//	base32 -d [data...]     decode
type Base32Plugin struct{}

func (Base32Plugin) Name() string        { return "base32" }
func (Base32Plugin) Description() string { return "encode or decode base32 data" }
func (Base32Plugin) Usage() string       { return "base32 [-d|--decode] [data...]" }

func (Base32Plugin) Run(ctx context.Context, args []string) error {
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
		src := strings.TrimSpace(strings.ToUpper(string(input)))
		decoded, err := enc.StdEncoding.DecodeString(src)
		if err != nil {
			return fmt.Errorf("base32: invalid input: %w", err)
		}
		_, err = hc.Stdout.Write(decoded)
		return err
	}

	fmt.Fprintln(hc.Stdout, enc.StdEncoding.EncodeToString(input))
	return nil
}

var _ plugins.PluginInfo = Base32Plugin{}
