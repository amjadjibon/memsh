package native

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	calc "github.com/amjadjibon/calc-go"
	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/shell/plugins"
)

// ── shared evaluator ─────────────────────────────────────────────────────────

// evalExpr evaluates expr using calc-go, recovering from panics.
// Returns the result as a string and any error.
func evalExpr(expr string, scale int) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", nil
	}

	var result any
	var evalErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				evalErr = fmt.Errorf("%v", r)
			}
		}()
		p := calc.NewParser(expr)
		node := p.Parse()
		result = node.Eval()
	}()

	if evalErr != nil {
		return "", evalErr
	}

	switch v := result.(type) {
	case float64:
		return formatFloat(v, scale), nil
	case string:
		return v, nil
	case int:
		return strconv.Itoa(v), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// formatFloat renders a float64 with up to `scale` decimal places,
// trimming trailing zeros (unless scale forces them, like real bc does).
func formatFloat(f float64, scale int) string {
	if scale == 0 {
		// integer truncation towards zero (bc default: scale=0)
		trunc := math.Trunc(f)
		return strconv.FormatInt(int64(trunc), 10)
	}
	s := strconv.FormatFloat(f, 'f', scale, 64)
	// trim trailing zeros after decimal point
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

// ── bc ───────────────────────────────────────────────────────────────────────

// BcPlugin is an interactive/batch math evaluator modelled on GNU bc.
//
//	bc                    read expressions from stdin
//	bc file.bc            read from virtual-FS file
//	echo "2^10" | bc
//	bc -l                 use math library scale (6 decimal places)
//	bc -q                 quiet (suppress welcome banner)
type BcPlugin struct{}

func (BcPlugin) Name() string        { return "bc" }
func (BcPlugin) Description() string { return "arbitrary-precision calculator" }
func (BcPlugin) Usage() string       { return "bc [-l] [-q] [file...]" }

func (BcPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	mathLib := false
	quiet := false
	var files []string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			files = append(files, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		switch a {
		case "--mathlib":
			mathLib = true
		case "--quiet", "--warn":
			quiet = true
		default:
			unknown := ""
			for _, c := range a[1:] {
				switch c {
				case 'l':
					mathLib = true
				case 'q':
					quiet = true
				case 'w':
					// warn about extensions — no-op
				default:
					unknown += string(c)
				}
			}
			if unknown != "" {
				return fmt.Errorf("bc: invalid option -- '%s'", unknown)
			}
		}
	}

	scale := 0
	if mathLib {
		scale = 6
	}

	if !quiet {
		fmt.Fprintln(hc.Stdout, "bc (memsh) 1.0")
	}

	// process files, then stdin if no files given
	if len(files) == 0 {
		return bcRunReader(hc, hc.Stdin, "-", scale)
	}
	for _, f := range files {
		abs := sc.ResolvePath(f)
		fh, err := sc.FS.Open(abs)
		if err != nil {
			return fmt.Errorf("bc: %s: %w", f, err)
		}
		err = bcRunReader(hc, fh, f, scale)
		fh.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func bcRunReader(hc interp.HandlerContext, r io.Reader, name string, scale int) error {
	scanner := bufio.NewScanner(r)
	// carry multi-line expressions joined by backslash
	var pending strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// continuation line
		if before, ok := strings.CutSuffix(line, "\\"); ok {
			pending.WriteString(before)
			continue
		}
		pending.WriteString(line)
		expr := pending.String()
		pending.Reset()

		expr = strings.TrimSpace(expr)
		if expr == "" || strings.HasPrefix(expr, "#") {
			continue
		}
		if strings.EqualFold(expr, "quit") {
			break
		}

		// handle scale= assignment
		if lo := strings.ToLower(expr); strings.HasPrefix(lo, "scale=") {
			val := strings.TrimPrefix(lo, "scale=")
			fmt.Sscanf(val, "%d", &scale)
			continue
		}

		result, err := evalExpr(expr, scale)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "bc: %s\n", err)
			continue
		}
		if result != "" {
			fmt.Fprintln(hc.Stdout, result)
		}
	}
	return scanner.Err()
}

// ensure BcPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = BcPlugin{}

// ── expr ─────────────────────────────────────────────────────────────────────

// ExprPlugin evaluates a single expression given as command-line tokens.
//
//	expr 2 + 3           → 5
//	expr 2 \* 3          → 6
//	expr '(1+2)*4'       → 12
//	expr 10 % 3          → 1
//	expr 5 > 3           → 1  (comparison: 1=true, 0=false)
//
// Exits with status 1 if the result is 0 (or empty).
type ExprPlugin struct{}

func (ExprPlugin) Name() string        { return "expr" }
func (ExprPlugin) Description() string { return "evaluate an expression" }
func (ExprPlugin) Usage() string       { return "expr <expression...>" }

func (ExprPlugin) Run(ctx context.Context, args []string) error {
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

// joinExprArgs joins tokens into a single expression string, normalising
// common expr-style tokens that differ from calc-go syntax.
func joinExprArgs(tokens []string) string {
	return strings.Join(tokens, " ")
}

// normaliseExprOperators rewrites operators that calc-go doesn't support:
//   - POSIX comparison operators (>, <, >=, <=, =, !=) → "1" or "0"
//   - modulo (%) → integer remainder evaluated directly
func normaliseExprOperators(expr string) (string, error) {
	// longer operators first to avoid partial matches
	ops := []string{"!=", ">=", "<=", ">", "<", "=", "%"}
	for _, op := range ops {
		target := " " + op + " "
		lhs, rhs, found := strings.Cut(expr, target)
		if !found {
			continue
		}
		lhs = strings.TrimSpace(lhs)
		rhs = strings.TrimSpace(rhs)

		lv, err := evalExpr(lhs, 6)
		if err != nil {
			return "", err
		}
		rv, err := evalExpr(rhs, 6)
		if err != nil {
			return "", err
		}

		lf, lerr := strconv.ParseFloat(lv, 64)
		rf, rerr := strconv.ParseFloat(rv, 64)

		if op == "%" {
			if lerr != nil || rerr != nil {
				return "", fmt.Errorf("non-numeric argument to %%")
			}
			if rf == 0 {
				return "", fmt.Errorf("division by zero")
			}
			rem := int64(lf) % int64(rf)
			return strconv.FormatInt(rem, 10), nil
		}

		var cmp bool
		if lerr == nil && rerr == nil {
			switch op {
			case ">":
				cmp = lf > rf
			case "<":
				cmp = lf < rf
			case ">=":
				cmp = lf >= rf
			case "<=":
				cmp = lf <= rf
			case "=":
				cmp = lf == rf
			case "!=":
				cmp = lf != rf
			}
		} else {
			switch op {
			case ">":
				cmp = lv > rv
			case "<":
				cmp = lv < rv
			case ">=":
				cmp = lv >= rv
			case "<=":
				cmp = lv <= rv
			case "=":
				cmp = lv == rv
			case "!=":
				cmp = lv != rv
			}
		}
		if cmp {
			return "1", nil
		}
		return "0", nil
	}
	return expr, nil
}

// ensure ExprPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = ExprPlugin{}
