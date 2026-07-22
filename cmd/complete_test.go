package cmd

import (
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func newCompleterTestShell(t *testing.T) *shell.Shell {
	t.Helper()
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/projects/alpha", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/projects/beta.txt", []byte("x"), 0o644); err != nil {
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

func TestFilterSuffixes(t *testing.T) {
	candidates := []string{"echo", "env", "exit", "expr"}
	results, prefixLen := filterSuffixes(candidates, "e")
	if prefixLen != 1 {
		t.Errorf("prefixLen = %d, want 1", prefixLen)
	}
	if len(results) != 4 {
		t.Fatalf("results = %v, want 4 matches", results)
	}

	results, prefixLen = filterSuffixes(candidates, "ex")
	if prefixLen != 2 {
		t.Errorf("prefixLen = %d, want 2", prefixLen)
	}
	var suffixes []string
	for _, r := range results {
		suffixes = append(suffixes, string(r))
	}
	want := map[string]bool{"it": true, "pr": true}
	if len(suffixes) != 2 {
		t.Fatalf("suffixes = %v, want 2 matches", suffixes)
	}
	for _, s := range suffixes {
		if !want[s] {
			t.Errorf("unexpected suffix %q", s)
		}
	}
}

func TestFilterSuffixesNoMatch(t *testing.T) {
	results, _ := filterSuffixes([]string{"foo", "bar"}, "zzz")
	if len(results) != 0 {
		t.Errorf("results = %v, want none", results)
	}
}

func TestCompleterAllCommands(t *testing.T) {
	s := newCompleterTestShell(t)
	c := &replCompleter{sh: s}

	cmds := c.allCommands()
	found := false
	for _, name := range cmds {
		if name == "ls" {
			found = true
			break
		}
	}
	if !found {
		t.Error("allCommands should include 'ls'")
	}
}

func TestCompleterCompletePath(t *testing.T) {
	s := newCompleterTestShell(t)
	c := &replCompleter{sh: s}

	results, prefixLen := c.completePath("read")
	if prefixLen != len("read") {
		t.Errorf("prefixLen = %d, want %d", prefixLen, len("read"))
	}
	var got []string
	for _, r := range results {
		got = append(got, "read"+string(r))
	}
	if len(got) != 1 || got[0] != "readme.md" {
		t.Errorf("completePath(read) = %v, want [readme.md]", got)
	}
}

func TestCompleterCompletePathWithDir(t *testing.T) {
	s := newCompleterTestShell(t)
	c := &replCompleter{sh: s}

	results, _ := c.completePath("/projects/a")
	var got []string
	for _, r := range results {
		got = append(got, "a"+string(r))
	}
	if len(got) != 1 || got[0] != "alpha" {
		t.Errorf("completePath(/projects/a) = %v, want [alpha]", got)
	}
}

func TestCompleterCompletePathInvalidDir(t *testing.T) {
	s := newCompleterTestShell(t)
	c := &replCompleter{sh: s}

	results, n := c.completePath("/nonexistent/foo")
	if results != nil || n != 0 {
		t.Errorf("completePath on missing dir = %v, %d, want nil, 0", results, n)
	}
}

func TestCompleterDoCommandPosition(t *testing.T) {
	s := newCompleterTestShell(t)
	c := &replCompleter{sh: s}

	line := []rune("pw")
	results, prefixLen := c.Do(line, len(line))
	if prefixLen != 2 {
		t.Errorf("prefixLen = %d, want 2", prefixLen)
	}
	found := false
	for _, r := range results {
		if string(r) == "d" { // "pw" + "d" = "pwd"
			found = true
		}
	}
	if !found {
		t.Error("Do at command position should suggest 'pwd' completion")
	}
}

func TestCompleterDoArgPosition(t *testing.T) {
	s := newCompleterTestShell(t)
	c := &replCompleter{sh: s}

	line := []rune("cat read")
	results, prefixLen := c.Do(line, len(line))
	if prefixLen != len("read") {
		t.Errorf("prefixLen = %d, want %d", prefixLen, len("read"))
	}
	var got []string
	for _, r := range results {
		got = append(got, "read"+string(r))
	}
	if len(got) != 1 || got[0] != "readme.md" {
		t.Errorf("Do at arg position = %v, want [readme.md]", got)
	}
}
