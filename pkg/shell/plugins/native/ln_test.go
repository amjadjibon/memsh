package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestLnReportsUnsupported(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	err := s.Run(ctx, `ln -s /a /b`)
	if err == nil {
		t.Fatal("expected error: virtual filesystem does not support links")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("err = %v, want not supported message", err)
	}
}

func TestLnMissingOperand(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `ln -s /a`); err == nil {
		t.Fatal("expected error for missing operand")
	}
}

func TestLnInvalidOption(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `ln -Z /a /b`); err == nil {
		t.Fatal("expected error for invalid option")
	}
}
