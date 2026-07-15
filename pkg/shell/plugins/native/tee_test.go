package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

func TestTeeWritesStdoutAndFile(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `echo hello | tee /out.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "hello" {
		t.Fatalf("stdout = %q, want hello", got)
	}
	data, err := afero.ReadFile(fs, "/out.txt")
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "hello" {
		t.Fatalf("file content = %q, want hello", data)
	}
}

func TestTeeAppendMode(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/out.txt", []byte("first\n"), 0o644)

	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(fs))
	if err := s.Run(ctx, `echo second | tee -a /out.txt`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := afero.ReadFile(fs, "/out.txt")
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "first\nsecond\n" {
		t.Fatalf("file content = %q, want first\\nsecond\\n", data)
	}
}

func TestTeeInvalidOption(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `echo hi | tee -Z`); err == nil {
		t.Fatal("expected error for invalid option")
	}
}
