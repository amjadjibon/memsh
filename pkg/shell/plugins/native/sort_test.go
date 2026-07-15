package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestSortLexical(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `printf 'b\na\nc\n' | sort`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "a\nb\nc" {
		t.Fatalf("output = %q, want a\\nb\\nc", got)
	}
}

func TestSortReverseNumericUnique(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `printf '3\n1\n1\n2\n' | sort -r -n -u`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "3\n2\n1" {
		t.Fatalf("output = %q, want 3\\n2\\n1", got)
	}
}

func TestSortInvalidOption(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `echo hi | sort -Z`); err == nil {
		t.Fatal("expected error for invalid option")
	}
}
