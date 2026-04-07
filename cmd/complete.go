package cmd

import (
	"strings"

	"github.com/amjadjibon/memsh/shell"
)

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
	return c.sh.Commands()
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
