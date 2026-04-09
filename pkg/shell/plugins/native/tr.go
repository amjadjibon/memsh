package native

import (
	"context"
	"fmt"
	"io"
	"strings"

	"mvdan.cc/sh/v3/interp"
)

type TrPlugin struct{}

func (TrPlugin) Name() string                                 { return "tr" }
func (TrPlugin) Description() string                          { return "translate or delete characters" }
func (TrPlugin) Usage() string                                { return "tr [-d] [-s] [-c] <set1> [set2]" }
func (TrPlugin) Run(ctx context.Context, args []string) error { return runTr(ctx, args) }

func runTr(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	deleteMode, squeeze, complement := false, false, false
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
		switch a {
		case "--delete":
			deleteMode = true
			continue
		case "--squeeze-repeats":
			squeeze = true
			continue
		case "--complement":
			complement = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'd':
				deleteMode = true
			case 's':
				squeeze = true
			case 'c':
				complement = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("tr: invalid option -- '%s'", unknown)
		}
	}

	if len(positional) == 0 {
		return fmt.Errorf("tr: missing operand")
	}

	set1 := expandTrSet(positional[0])
	if complement {
		var complemented []rune
		for r := rune(0); r <= 127; r++ {
			if runeIndex(set1, r) < 0 {
				complemented = append(complemented, r)
			}
		}
		set1 = complemented
	}

	set1Map := make(map[rune]bool, len(set1))
	for _, r := range set1 {
		set1Map[r] = true
	}

	var set2 []rune
	if !deleteMode && len(positional) >= 2 {
		set2 = expandTrSet(positional[1])
	}

	input, err := io.ReadAll(hc.Stdin)
	if err != nil {
		return err
	}

	var sb strings.Builder
	prev := rune(-1)
	for _, r := range string(input) {
		if deleteMode {
			if !set1Map[r] {
				sb.WriteRune(r)
			}
			continue
		}
		if idx := runeIndex(set1, r); idx >= 0 && len(set2) > 0 {
			mapped := set2[min(idx, len(set2)-1)]
			if squeeze && mapped == prev {
				continue
			}
			sb.WriteRune(mapped)
			prev = mapped
		} else if squeeze && set1Map[r] && r == prev {
			continue
		} else {
			sb.WriteRune(r)
			prev = r
		}
	}
	_, err = fmt.Fprint(hc.Stdout, sb.String())
	return err
}
