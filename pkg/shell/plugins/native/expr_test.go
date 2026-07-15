package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestExprArithmetic(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `expr 2 + 3`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "5" {
		t.Fatalf("output = %q, want 5", got)
	}
}

func TestExprComparison(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `expr 3 \> 2`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "1" {
		t.Fatalf("output = %q, want 1", got)
	}
}

func TestExprFalseResultExitsNonZero(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `expr 1 \> 2`); err == nil {
		t.Fatal("expected non-zero exit for false comparison")
	}
	if got := strings.TrimSpace(buf.String()); got != "0" {
		t.Fatalf("output = %q, want 0", got)
	}
}

func TestExprMissingOperand(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `expr`); err == nil {
		t.Fatal("expected error for missing operand")
	}
}
