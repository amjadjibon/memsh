package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestReadSingleVar(t *testing.T) {
	var buf strings.Builder
	s := NewTestShell(t, &buf,
		shell.WithStdIO(strings.NewReader("hello world\n"), &buf, &buf),
	)
	ctx := context.Background()

	if err := s.Run(ctx, "read line; echo \"got: $line\""); err != nil {
		t.Fatalf("read: %v", err)
	}
	if out := strings.TrimSpace(buf.String()); out != "got: hello world" {
		t.Errorf("output = %q", out)
	}
}

func TestReadDefaultReply(t *testing.T) {
	var buf strings.Builder
	s := NewTestShell(t, &buf,
		shell.WithStdIO(strings.NewReader("value\n"), &buf, &buf),
	)
	ctx := context.Background()

	if err := s.Run(ctx, "read; echo \"reply: $REPLY\""); err != nil {
		t.Fatalf("read: %v", err)
	}
	if out := strings.TrimSpace(buf.String()); out != "reply: value" {
		t.Errorf("output = %q", out)
	}
}

func TestReadMultipleVars(t *testing.T) {
	var buf strings.Builder
	s := NewTestShell(t, &buf,
		shell.WithStdIO(strings.NewReader("one two three four\n"), &buf, &buf),
	)
	ctx := context.Background()

	if err := s.Run(ctx, "read a b c; echo \"a=$a b=$b c=$c\""); err != nil {
		t.Fatalf("read: %v", err)
	}
	// The last variable absorbs any remaining fields.
	if out := strings.TrimSpace(buf.String()); out != "a=one b=two c=three four" {
		t.Errorf("output = %q", out)
	}
}

func TestReadFewerFieldsThanVars(t *testing.T) {
	var buf strings.Builder
	s := NewTestShell(t, &buf,
		shell.WithStdIO(strings.NewReader("only\n"), &buf, &buf),
	)
	ctx := context.Background()

	if err := s.Run(ctx, "read a b; echo \"a=$a b=$b\""); err != nil {
		t.Fatalf("read: %v", err)
	}
	if out := strings.TrimSpace(buf.String()); out != "a=only b=" {
		t.Errorf("output = %q", out)
	}
}

func TestReadNoInputErrors(t *testing.T) {
	var buf strings.Builder
	s := NewTestShell(t, &buf,
		shell.WithStdIO(strings.NewReader(""), &buf, &buf),
	)
	ctx := context.Background()

	if err := s.Run(ctx, "read line"); err == nil {
		t.Error("expected error reading from empty stdin")
	}
}
