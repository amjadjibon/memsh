package native

import (
	"context"
	"fmt"
)

type LnPlugin struct{}

func (LnPlugin) Name() string                                 { return "ln" }
func (LnPlugin) Description() string                          { return "create links" }
func (LnPlugin) Usage() string                                { return "ln [-s] <target> <link>" }
func (LnPlugin) Run(ctx context.Context, args []string) error { return runLn(ctx, args) }

func runLn(ctx context.Context, args []string) error {
	var positional []string
	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			positional = append(positional, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		if a == "--symbolic" || a == "--force" {
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 's', 'f':
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("ln: invalid option -- '%s'", unknown)
		}
	}
	if len(positional) < 2 {
		return fmt.Errorf("ln: missing operand")
	}
	return fmt.Errorf("ln: not supported in virtual filesystem")
}
