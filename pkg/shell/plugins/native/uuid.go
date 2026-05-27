package native

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

// UUIDPlugin implements the `uuid` command.
// Generates one or more UUIDs (v4 by default).
//
//	uuid              generate one UUID v4
//	uuid -n 5         generate 5 UUIDs
//	uuid -v 7         generate UUID v7 (time-ordered)
type UUIDPlugin struct{}

func (UUIDPlugin) Name() string        { return "uuid" }
func (UUIDPlugin) Description() string { return "generate universally unique identifiers" }
func (UUIDPlugin) Usage() string       { return "uuid [-n count] [-v 4|7] [-u]" }

func (UUIDPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	count := 1
	version := 4
	upper := false

	i := 1
	for i < len(args) {
		switch args[i] {
		case "--":
			i++
			goto doneFlags
		case "-n", "--count":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "uuid: -n requires an argument")
				return interp.ExitStatus(1)
			}
			if _, err := fmt.Sscanf(args[i+1], "%d", &count); err != nil || count < 1 {
				fmt.Fprintf(hc.Stderr, "uuid: invalid count %q\n", args[i+1])
				return interp.ExitStatus(1)
			}
			i += 2
		case "-v", "--version":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "uuid: -v requires an argument")
				return interp.ExitStatus(1)
			}
			if _, err := fmt.Sscanf(args[i+1], "%d", &version); err != nil {
				fmt.Fprintf(hc.Stderr, "uuid: invalid version %q\n", args[i+1])
				return interp.ExitStatus(1)
			}
			if version != 4 && version != 7 {
				fmt.Fprintf(hc.Stderr, "uuid: unsupported version %d (use 4 or 7)\n", version)
				return interp.ExitStatus(1)
			}
			i += 2
		case "-u", "--upper":
			upper = true
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(hc.Stderr, "uuid: invalid option -- %q\n", args[i])
				return interp.ExitStatus(1)
			}
			goto doneFlags
		}
	}
doneFlags:
	for range count {
		var id uuid.UUID
		var err error
		switch version {
		case 7:
			id, err = uuid.NewV7()
		default:
			id, err = uuid.NewRandom()
		}
		if err != nil {
			return fmt.Errorf("uuid: %w", err)
		}
		s := id.String()
		if upper {
			s = strings.ToUpper(s)
		}
		fmt.Fprintln(hc.Stdout, s)
	}
	return nil
}

var _ plugins.PluginInfo = UUIDPlugin{}
