package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestEnvListsVariables(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithEnv(map[string]string{"FOO": "bar"}))
	if err := s.Run(ctx, `env`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "FOO=bar") {
		t.Fatalf("output = %q, want FOO=bar", buf.String())
	}
}

func TestPrintenvDelegatesToEnv(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithEnv(map[string]string{"BAZ": "qux"}))
	if err := s.Run(ctx, `printenv`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "BAZ=qux") {
		t.Fatalf("output = %q, want BAZ=qux", buf.String())
	}
}
