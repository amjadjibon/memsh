package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestBc(t *testing.T) {
	ctx := context.Background()

	run := func(t *testing.T, script string) string {
		t.Helper()
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, script); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return strings.TrimSpace(buf.String())
	}

	t.Run("basic arithmetic", func(t *testing.T) {
		out := run(t, `echo "2 + 3" | bc -q`)
		if out != "5" {
			t.Errorf("got %q, want 5", out)
		}
	})

	t.Run("multiplication", func(t *testing.T) {
		out := run(t, `echo "6 * 7" | bc -q`)
		if out != "42" {
			t.Errorf("got %q, want 42", out)
		}
	})

	t.Run("integer division truncates (scale=0)", func(t *testing.T) {
		out := run(t, `echo "7 / 2" | bc -q`)
		if out != "3" {
			t.Errorf("got %q, want 3", out)
		}
	})

	t.Run("-l math library sets scale=6", func(t *testing.T) {
		out := run(t, `echo "7 / 2" | bc -lq`)
		if out != "3.5" {
			t.Errorf("got %q, want 3.5", out)
		}
	})

	t.Run("scale= directive", func(t *testing.T) {
		out := run(t, `printf "scale=4\n22/7\n" | bc -q`)
		// 22/7 ≈ 3.142857... → 3.1428 at scale 4
		if !strings.HasPrefix(out, "3.142") {
			t.Errorf("got %q, want prefix 3.142", out)
		}
	})

	t.Run("exponentiation", func(t *testing.T) {
		out := run(t, `echo "2^10" | bc -q`)
		if out != "1024" {
			t.Errorf("got %q, want 1024", out)
		}
	})

	t.Run("trig function sin(pi/2)=1", func(t *testing.T) {
		out := run(t, `echo "sin(pi/2)" | bc -q`)
		if out != "1" {
			t.Errorf("got %q, want 1", out)
		}
	})

	t.Run("sqrt", func(t *testing.T) {
		out := run(t, `echo "sqrt(144)" | bc -q`)
		if out != "12" {
			t.Errorf("got %q, want 12", out)
		}
	})

	t.Run("multiple expressions", func(t *testing.T) {
		out := run(t, `printf "1+1\n2*3\n" | bc -q`)
		lines := strings.Split(out, "\n")
		if lines[0] != "2" || lines[1] != "6" {
			t.Errorf("got %q", out)
		}
	})

	t.Run("quit stops evaluation", func(t *testing.T) {
		out := run(t, `printf "1+1\nquit\n99+99\n" | bc -q`)
		if strings.Contains(out, "198") {
			t.Errorf("lines after quit should not be evaluated: %q", out)
		}
	})

	t.Run("comments with #", func(t *testing.T) {
		out := run(t, `printf "# comment\n3+3\n" | bc -q`)
		if out != "6" {
			t.Errorf("got %q, want 6", out)
		}
	})

	t.Run("file input", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/calc.bc", []byte("10 * 10\n"), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "bc -q /calc.bc"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(buf.String()) != "100" {
			t.Errorf("got %q, want 100", buf.String())
		}
	})

	t.Run("error on bad expression continues", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		// bad line followed by good line — bc should continue
		if err := s.Run(ctx, `printf "1 @ 2\n5+5\n" | bc -q`); err != nil {
			t.Fatalf("bc should not return error on bad expr line: %v", err)
		}
		if !strings.Contains(buf.String(), "10") {
			t.Errorf("expected 10 from 5+5 after error: %q", buf.String())
		}
	})
}

func TestExpr(t *testing.T) {
	ctx := context.Background()

	run := func(t *testing.T, script string) (string, error) {
		t.Helper()
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		err := s.Run(ctx, script)
		return strings.TrimSpace(buf.String()), err
	}

	t.Run("addition", func(t *testing.T) {
		out, err := run(t, "expr 2 + 3")
		if err != nil || out != "5" {
			t.Errorf("got %q err=%v, want 5", out, err)
		}
	})

	t.Run("subtraction", func(t *testing.T) {
		out, err := run(t, "expr 10 - 4")
		if err != nil || out != "6" {
			t.Errorf("got %q err=%v, want 6", out, err)
		}
	})

	t.Run("multiplication (quoted)", func(t *testing.T) {
		out, err := run(t, `expr 6 '*' 7`)
		if err != nil || out != "42" {
			t.Errorf("got %q err=%v, want 42", out, err)
		}
	})

	t.Run("division", func(t *testing.T) {
		out, err := run(t, "expr 10 / 2")
		if err != nil || out != "5" {
			t.Errorf("got %q err=%v, want 5", out, err)
		}
	})

	t.Run("modulo", func(t *testing.T) {
		out, err := run(t, "expr 10 % 3")
		if err != nil || out != "1" {
			t.Errorf("got %q err=%v, want 1", out, err)
		}
	})

	t.Run("parenthesised expression", func(t *testing.T) {
		out, err := run(t, "expr '(2 + 3) * 4'")
		if err != nil || out != "20" {
			t.Errorf("got %q err=%v, want 20", out, err)
		}
	})

	t.Run("comparison > true returns 1", func(t *testing.T) {
		// escape > so the shell doesn't treat it as a redirect
		out, err := run(t, `expr 5 \> 3`)
		if err != nil || out != "1" {
			t.Errorf("got %q err=%v, want 1", out, err)
		}
	})

	t.Run("comparison > false returns 0 (exit 1)", func(t *testing.T) {
		out, err := run(t, `expr 2 \> 5`)
		if out != "0" {
			t.Errorf("got %q, want 0", out)
		}
		if err == nil {
			t.Error("expected exit status 1 for false comparison")
		}
	})

	t.Run("comparison =", func(t *testing.T) {
		out, err := run(t, "expr 3 = 3")
		if err != nil || out != "1" {
			t.Errorf("got %q err=%v, want 1", out, err)
		}
	})

	t.Run("comparison !=", func(t *testing.T) {
		out, err := run(t, "expr 3 != 4")
		if err != nil || out != "1" {
			t.Errorf("got %q err=%v, want 1", out, err)
		}
	})

	t.Run("zero result exits 1", func(t *testing.T) {
		_, err := run(t, "expr 1 - 1")
		if err == nil {
			t.Error("expected exit status 1 for zero result")
		}
	})

	t.Run("result used in shell arithmetic", func(t *testing.T) {
		out, err := run(t, `X=$(expr 3 + 4) && echo $X`)
		if err != nil || out != "7" {
			t.Errorf("got %q err=%v, want 7", out, err)
		}
	})

	t.Run("power via expr token", func(t *testing.T) {
		out, err := run(t, "expr 2 ^ 8")
		if err != nil || out != "256" {
			t.Errorf("got %q err=%v, want 256", out, err)
		}
	})
}
