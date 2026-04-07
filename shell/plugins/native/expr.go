package native

import (
	"context"
	"fmt"

	"mvdan.cc/sh/v3/interp"
)

type ExprPlugin struct{}

func (ExprPlugin) Name() string                                 { return "expr" }
func (ExprPlugin) Description() string                          { return "evaluate an expression" }
func (ExprPlugin) Usage() string                                { return "expr <expression...>" }
func (ExprPlugin) Run(ctx context.Context, args []string) error { return runExpr(ctx, args) }

func runExpr(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	if len(args) < 2 {
		fmt.Fprintln(hc.Stderr, "expr: missing operand")
		return interp.ExitStatus(2)
	}

	// join tokens; the shell already handles quoting so each arg is a token
	expr := joinExprArgs(args[1:])

	// handle comparison operators that calc-go doesn't understand natively
	expr, err := normaliseExprOperators(expr)
	if err != nil {
		fmt.Fprintf(hc.Stderr, "expr: %v\n", err)
		return interp.ExitStatus(2)
	}

	result, evalErr := evalExpr(expr, 0) // expr always returns integers
	if evalErr != nil {
		fmt.Fprintf(hc.Stderr, "expr: %v\n", evalErr)
		return interp.ExitStatus(2)
	}

	fmt.Fprintln(hc.Stdout, result)

	// exit 1 if result is 0 or empty (POSIX expr semantics)
	if result == "" || result == "0" {
		return interp.ExitStatus(1)
	}
	return nil
}
