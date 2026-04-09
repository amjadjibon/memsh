package native

import (
	"bufio"
	"fmt"
	calc "github.com/amjadjibon/calc-go"
	"io"
	"math"
	"mvdan.cc/sh/v3/interp"
	"strconv"
	"strings"
)

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

func joinExprArgs(tokens []string) string {
	return strings.Join(tokens, " ")
}

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
