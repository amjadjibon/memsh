package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestDuDefaultTarget(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/a.txt", []byte("hello"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `du /`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "/") {
		t.Fatalf("output = %q, want size and target", buf.String())
	}
}

func TestDuHumanReadable(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/a.txt", []byte(strings.Repeat("x", 2048)), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `du -h /`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "K") {
		t.Fatalf("output = %q, want human-readable size", buf.String())
	}
}

func TestDuInvalidOption(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `du -Z /`); err == nil {
		t.Fatal("expected error for invalid option")
	}
}
