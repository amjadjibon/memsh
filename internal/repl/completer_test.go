package repl

import (
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func newReplCompleterTestShell(t *testing.T) *shell.Shell {
	t.Helper()
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/projects/alpha", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/readme.md", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := shell.New(shell.WithFS(fs), shell.WithWASMEnabled(false))
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestREPLCompleterAllCommands(t *testing.T) {
	s := newReplCompleterTestShell(t)
	c := &REPLCompleter{sh: s}

	found := false
	for _, name := range c.allCommands() {
		if name == "pwd" {
			found = true
		}
	}
	if !found {
		t.Error("allCommands should include 'pwd'")
	}
}

func TestREPLCompleterCompletePath(t *testing.T) {
	s := newReplCompleterTestShell(t)
	c := &REPLCompleter{sh: s}

	results, prefixLen := c.completePath("read")
	if prefixLen != len("read") {
		t.Errorf("prefixLen = %d, want %d", prefixLen, len("read"))
	}
	if len(results) != 1 || string(results[0]) != "me.md" {
		t.Errorf("completePath(read) results = %v, want [me.md]", results)
	}
}

func TestREPLCompleterCompletePathInvalidDir(t *testing.T) {
	s := newReplCompleterTestShell(t)
	c := &REPLCompleter{sh: s}

	results, n := c.completePath("/nowhere/x")
	if results != nil || n != 0 {
		t.Errorf("completePath on missing dir = %v, %d, want nil, 0", results, n)
	}
}

func TestREPLCompleterDo(t *testing.T) {
	s := newReplCompleterTestShell(t)
	c := &REPLCompleter{sh: s}

	line := []rune("cat read")
	results, prefixLen := c.Do(line, len(line))
	if prefixLen != len("read") {
		t.Errorf("prefixLen = %d, want %d", prefixLen, len("read"))
	}
	if len(results) != 1 || string(results[0]) != "me.md" {
		t.Errorf("Do at arg position results = %v, want [me.md]", results)
	}
}

func TestREPLCompleterDoCommandPosition(t *testing.T) {
	s := newReplCompleterTestShell(t)
	c := &REPLCompleter{sh: s}

	line := []rune("pw")
	results, prefixLen := c.Do(line, len(line))
	if prefixLen != 2 {
		t.Errorf("prefixLen = %d, want 2", prefixLen)
	}
	found := false
	for _, r := range results {
		if string(r) == "d" {
			found = true
		}
	}
	if !found {
		t.Error("Do at command position should suggest 'pwd'")
	}
}
