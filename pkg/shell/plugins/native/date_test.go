package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestDateDefaultFormat(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `date`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(buf.String()) == "" {
		t.Fatal("expected non-empty date output")
	}
}

func TestDateAcceptsUtcFlagAndKnownFormatFlag(t *testing.T) {
	// The recognized "+%F"-style flags are matched literally in runDate but
	// are not valid Go time layout tokens, so today they round-trip as-is.
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `date -u +%F`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "%F" {
		t.Fatalf("output = %q, want %%F", got)
	}
}
