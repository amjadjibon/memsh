package cmd

import (
	"sort"
	"strings"

	"github.com/amjadjibon/memsh/shell"
)

var builtinCmds = []string{
	"awk", "base64", "cat", "cd", "chmod", "cp", "cut", "date", "diff",
	"du", "df", "echo", "env", "exit", "find", "grep", "head", "help",
	"ln", "ls", "man", "mkdir", "mv", "printf", "pwd", "read", "rm",
	"rmdir", "sed", "seq", "sleep", "sort", "source", "stat", "tail",
	"tee", "touch", "tr", "uniq", "wc", "which", "xargs", "yes",
}

type replCompleter struct {
	sh *shell.Shell
}

func (c *replCompleter) Do(line []rune, pos int) ([][]rune, int) {
	str := string(line[:pos])

	wordStart := strings.LastIndexAny(str, " \t") + 1
	partial := str[wordStart:]

	isCmd := strings.TrimSpace(str[:wordStart]) == ""

	if isCmd {
		return filterSuffixes(c.allCommands(), partial)
	}
	return c.completePath(partial)
}

func (c *replCompleter) allCommands() []string {
	cmds := make([]string, len(builtinCmds))
	copy(cmds, builtinCmds)
	cmds = append(cmds, c.sh.RegisteredPlugins()...)
	sort.Strings(cmds)
	return cmds
}

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

func filterSuffixes(candidates []string, prefix string) ([][]rune, int) {
	var results [][]rune
	for _, c := range candidates {
		if strings.HasPrefix(c, prefix) {
			results = append(results, []rune(c[len(prefix):]))
		}
	}
	return results, len([]rune(prefix))
}
