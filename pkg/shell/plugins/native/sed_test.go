package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestSedSubstituteFirstOccurrence(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `echo "foo foo" | sed 's/foo/bar/'`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "bar foo" {
		t.Fatalf("output = %q, want bar foo", got)
	}
}

func TestSedSubstituteGlobal(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `echo "foo foo" | sed 's/foo/bar/g'`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "bar bar" {
		t.Fatalf("output = %q, want bar bar", got)
	}
}

func TestSedMissingExpression(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `sed`); err == nil {
		t.Fatal("expected error for missing expression")
	}
}

func TestSedUnsupportedExpression(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `echo hi | sed 'p'`); err == nil {
		t.Fatal("expected error for unsupported expression")
	}
}
