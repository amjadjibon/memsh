package shell

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/afero"
)

// CompleteResult holds the tab-completion results for a given input.
type CompleteResult struct {
	Completions []string `json:"completions"`
	Prefix      string   `json:"prefix"` // input text before the completing token
	Token       string   `json:"token"`  // the partial token being completed
}

// Complete computes tab completions for input at the given cursor position.
// commands is the full list of known command names used for command-position completion.
func Complete(input string, cursor int, fs afero.Fs, cwd string, commands []string) CompleteResult {
	if cursor < 0 || cursor > len(input) {
		cursor = len(input)
	}
	before := input[:cursor]

	// Find the start of the current token by walking back over non-whitespace,
	// stopping at shell metacharacters too.
	tokenStart := len(before)
	for tokenStart > 0 {
		c := before[tokenStart-1]
		if c == ' ' || c == '\t' || c == '|' || c == '&' || c == ';' || c == '(' || c == '\n' {
			break
		}
		tokenStart--
	}
	token := before[tokenStart:]
	prefix := before[:tokenStart]

	// Determine if the token is in command position (first word of its command segment).
	cmdStart := 0
	for i := len(prefix) - 1; i >= 0; i-- {
		c := prefix[i]
		if c == '|' || c == '&' || c == ';' || c == '(' || c == '\n' {
			cmdStart = i + 1
			break
		}
	}
	isCommand := strings.TrimSpace(prefix[cmdStart:]) == ""

	var completions []string
	if isCommand {
		for _, cmd := range commands {
			if strings.HasPrefix(cmd, token) {
				completions = append(completions, cmd)
			}
		}
		sort.Strings(completions)
	} else {
		completions = completePath(token, fs, cwd)
	}

	return CompleteResult{
		Completions: completions,
		Prefix:      prefix,
		Token:       token,
	}
}

// completePath returns all filesystem entries that complete the given token.
// token may be absolute (/et → /etc/) or relative (sr → src/).
func completePath(token string, fs afero.Fs, cwd string) []string {
	var dir, tokenDir, filePrefix string

	switch {
	case token == "":
		dir = cwd
		tokenDir = ""
		filePrefix = ""
	case token == "/":
		dir = "/"
		tokenDir = "/"
		filePrefix = ""
	case strings.HasSuffix(token, "/"):
		tokenDir = token
		dir = absJoin(token, cwd)
		filePrefix = ""
	default:
		if idx := strings.LastIndex(token, "/"); idx >= 0 {
			tokenDir = token[:idx+1]
			dir = absJoin(token[:idx+1], cwd)
			filePrefix = token[idx+1:]
		} else {
			tokenDir = ""
			dir = cwd
			filePrefix = token
		}
	}

	entries, err := afero.ReadDir(fs, dir)
	if err != nil {
		return nil
	}

	results := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, filePrefix) {
			continue
		}
		completion := tokenDir + name
		if e.IsDir() {
			completion += "/"
		}
		results = append(results, completion)
	}
	sort.Strings(results)
	return results
}

// absJoin resolves path relative to cwd when it is not already absolute.
func absJoin(path, cwd string) string {
	if path == "" || path == "." || path == "./" {
		return cwd
	}
	if strings.HasPrefix(path, "/") {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(cwd, path))
}
