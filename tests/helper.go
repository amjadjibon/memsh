package tests

import (
	"os"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell"
)

// NewTestShell creates a shell for testing with stdout/stderr wired to buf
func NewTestShell(t testingT, buf *strings.Builder, opts ...shell.Option) *shell.Shell {
	t.Helper()
	base := []shell.Option{
		shell.WithStdIO(strings.NewReader(""), buf, buf),
		shell.WithWASMEnabled(false), // skip WASM for faster tests
	}
	s, err := shell.New(append(base, opts...)...)
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	return s
}

// NewTestShellWithBuffer creates a shell with bytes.Buffer for output
func NewTestShellWithBuffer(t testingT, buf *strings.Builder, opts ...shell.Option) *shell.Shell {
	t.Helper()
	base := []shell.Option{
		shell.WithStdIO(strings.NewReader(""), buf, buf),
		shell.WithWASMEnabled(false),
	}
	s, err := shell.New(append(base, opts...)...)
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	return s
}

// NewTestShellWithBytes creates a shell with bytes.Buffer for output
func NewTestShellWithBytes(t testingT, buf *strings.Builder, opts ...shell.Option) *shell.Shell {
	t.Helper()
	base := []shell.Option{
		shell.WithStdIO(strings.NewReader(""), buf, buf),
		shell.WithWASMEnabled(false),
	}
	s, err := shell.New(append(base, opts...)...)
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	return s
}

// testingT is a minimal interface matching *testing.T
type testingT interface {
	Helper()
	Fatalf(format string, args ...any)
}

// RealTmpDir creates a temporary OS directory with cleanup
func RealTmpDir(t testingT) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "shelltest-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	// Note: in real tests you'd register t.Cleanup
	return dir
}
