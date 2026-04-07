package native

import (
	"context"
	"fmt"
	"mvdan.cc/sh/v3/interp"
	"strconv"
)

type SeqPlugin struct{}

func (SeqPlugin) Name() string                                 { return "seq" }
func (SeqPlugin) Description() string                          { return "print a sequence of numbers" }
func (SeqPlugin) Usage() string                                { return "seq [first [increment]] last" }
func (SeqPlugin) Run(ctx context.Context, args []string) error { return runSeq(ctx, args) }

func runSeq(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	var start, step, stop int
	var err error
	switch len(args) {
	case 1:
		return nil
	case 2:
		start = 1
		stop, err = strconv.Atoi(args[1])
	case 3:
		start, err = strconv.Atoi(args[1])
		if err == nil {
			stop, err = strconv.Atoi(args[2])
		}
	default:
		start, err = strconv.Atoi(args[1])
		if err != nil {
			break
		}
		step, err = strconv.Atoi(args[2])
		if err != nil || step == 0 {
			return fmt.Errorf("seq: invalid increment '%s'", args[2])
		}
		stop, err = strconv.Atoi(args[3])
	}
	if err != nil {
		return fmt.Errorf("seq: invalid number '%s'", args[len(args)-1])
	}
	if step == 0 {
		if start <= stop {
			step = 1
		} else {
			step = -1
		}
	}
	for i := start; ; i += step {
		fmt.Fprintln(hc.Stdout, i)
		if (step > 0 && i >= stop) || (step < 0 && i <= stop) {
			break
		}
	}
	return nil
}
