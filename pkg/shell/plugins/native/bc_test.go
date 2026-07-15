package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestBcEvaluatesStdinExpressions(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `printf '2+3\n' | bc -q`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "5" {
		t.Fatalf("output = %q, want 5", got)
	}
}

func TestBcMathlibScale(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `printf '10/3\n' | bc -l -q`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "3.333333" {
		t.Fatalf("output = %q, want 3.333333", got)
	}
}

func TestBcPrintsBanner(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `printf '1+1\n' | bc`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "bc (memsh)") {
		t.Fatalf("output = %q, want banner", buf.String())
	}
}

func TestBcReadsFromFile(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/expr.bc", []byte("4*4\n"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `bc -q /expr.bc`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "16" {
		t.Fatalf("output = %q, want 16", got)
	}
}

func TestBcMissingFile(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `bc /missing.bc`); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestBcInvalidOption(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `bc -Z`); err == nil {
		t.Fatal("expected error for invalid option")
	}
}
