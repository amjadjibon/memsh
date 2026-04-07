package native

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

type SleepPlugin struct{}

func (SleepPlugin) Name() string                                 { return "sleep" }
func (SleepPlugin) Description() string                          { return "delay for a specified time" }
func (SleepPlugin) Usage() string                                { return "sleep <seconds>" }
func (SleepPlugin) Run(ctx context.Context, args []string) error { return runSleep(ctx, args) }

func runSleep(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("sleep: missing operand")
	}
	d, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return fmt.Errorf("sleep: invalid time interval '%s'", args[1])
	}
	timer := time.NewTimer(time.Duration(d * float64(time.Second)))
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
