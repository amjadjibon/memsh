package cmd

import (
	"sort"
	"strings"

	"github.com/amjadjibon/memsh/shell"
)

// builtinCmds is the fixed set of built-in shell commands.
var builtinCmds = []string{
	"cat", "cd", "echo", "exit", "ls", "mkdir", "pwd", "quit", "rm", "touch",
}

// replCompleter implements readline.AutoCompleter for the memsh REPL.
// First token → command name completion.
// Subsequent tokens → virtual FS path completion.
type replCompleter struct {
	sh *shell.Shell
}

// Do is called by readline on every Tab press.
// line is the full input up to the cursor; pos is the cursor position.
// Returns candidate suffixes and the length of the prefix being replaced.
func (c *replCompleter) Do(line []rune, pos int) ([][]rune, int) {
	str := string(line[:pos])

	// Find where the current word starts (after the last unquoted space).
	wordStart := strings.LastIndexAny(str, " \t") + 1
	partial := str[wordStart:]

	// Determine if we are completing the command or an argument.
	isCmd := strings.TrimSpace(str[:wordStart]) == ""

	if isCmd {
		return filterSuffixes(c.allCommands(), partial)
	}
	return c.completePath(partial)
}

// allCommands merges built-ins with registered plugin names.
func (c *replCompleter) allCommands() []string {
	cmds := make([]string, len(builtinCmds))
	copy(cmds, builtinCmds)
	cmds = append(cmds, c.sh.RegisteredPlugins()...)
	sort.Strings(cmds)
	return cmds
}

// completePath completes a partial filesystem path against the virtual FS.
func (c *replCompleter) completePath(partial string) ([][]rune, int) {
	dir := c.sh.Cwd()
	prefix := partial

	if idx := strings.LastIndex(partial, "/"); idx >= 0 {
		if idx == 0 {
			dir = "/"
		} else {
			dir = partial[:idx]
		}
		prefix = partial[idx+1:]
	}

	entries, err := c.sh.ListDir(dir)
	if err != nil {
		return nil, 0
	}

	return filterSuffixes(entries, prefix)
}

// filterSuffixes returns the suffix of each candidate that starts with prefix,
// along with the rune-length of the prefix to replace.
func filterSuffixes(candidates []string, prefix string) ([][]rune, int) {
	var results [][]rune
	for _, c := range candidates {
		if strings.HasPrefix(c, prefix) {
			results = append(results, []rune(c[len(prefix):]))
		}
	}
	return results, len([]rune(prefix))
}
