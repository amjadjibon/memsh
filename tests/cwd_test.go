package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

// TestPluginsSeeCdWithinSameScript: plugins must resolve relative paths
// against the directory set by an earlier cd in the same Run, and inside a
// subshell against the subshell's directory.
func TestPluginsSeeCdWithinSameScript(t *testing.T) {
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/d", 0o755)
	_ = fs.MkdirAll("/e", 0o755)
	_ = afero.WriteFile(fs, "/d/f", []byte("in-d\n"), 0o644)
	_ = afero.WriteFile(fs, "/e/f", []byte("in-e\n"), 0o644)

	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(fs))
	script := `cd /d; cat f; (cd /e; cat f); cat f; echo x | tee g > /dev/null; cd /e; cat ../d/g`
	if err := s.Run(context.Background(), script); err != nil {
		t.Fatalf("run: %v (output %q)", err, buf.String())
	}
	if got, want := buf.String(), "in-d\nin-e\nin-d\nx\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if s.Cwd() != "/e" {
		t.Fatalf("Cwd() = %q, want /e", s.Cwd())
	}
}
