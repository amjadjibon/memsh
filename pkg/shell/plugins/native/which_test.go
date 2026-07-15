package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestWhichKnownCommand(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `which cat`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "cat") {
		t.Fatalf("output = %q, want cat info", buf.String())
	}
}

func TestWhichUnknownCommand(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `which nosuchcmd`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "not found") {
		t.Fatalf("output = %q, want not found message", buf.String())
	}
}

func TestWhichAlias(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithAliases(map[string]string{"ll": "ls -l"}))
	if err := s.Run(ctx, `which ll`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "aliased to") {
		t.Fatalf("output = %q, want aliased message", buf.String())
	}
}

func TestWhichNoArgument(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `which`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
