package native

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/amjadjibon/memsh/shell/plugins"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// GrepPlugin searches file contents for lines matching a pattern.
//
//	grep [-i] [-n] [-v] [-r] <pattern> [file...]   read from files or stdin
type GrepPlugin struct{}

func (GrepPlugin) Name() string        { return "grep" }
func (GrepPlugin) Description() string { return "search file contents for patterns" }
func (GrepPlugin) Usage() string       { return "grep [-i] [-n] [-v] [-r] <pattern> [file...]" }

func (GrepPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	caseInsensitive, showLineNums, invertMatch, recursive := false, false, false, false
	countMode, listFiles, wholeWord, onlyMatch := false, false, false, false
	var pattern string
	var files []string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			if pattern == "" {
				pattern = a
			} else {
				files = append(files, a)
			}
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		switch a {
		case "--ignore-case":
			caseInsensitive = true
			continue
		case "--line-number":
			showLineNums = true
			continue
		case "--invert-match":
			invertMatch = true
			continue
		case "--recursive":
			recursive = true
			continue
		case "--count":
			countMode = true
			continue
		case "--files-with-matches":
			listFiles = true
			continue
		case "--word-regexp":
			wholeWord = true
			continue
		case "--only-matching":
			onlyMatch = true
			continue
		case "--extended-regexp", "--quiet", "--silent":
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'i':
				caseInsensitive = true
			case 'n':
				showLineNums = true
			case 'v':
				invertMatch = true
			case 'r', 'R':
				recursive = true
			case 'c':
				countMode = true
			case 'l':
				listFiles = true
			case 'w':
				wholeWord = true
			case 'E', 'q', 's':
			case 'o':
				onlyMatch = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("grep: invalid option -- '%s'", unknown)
		}
	}

	if pattern == "" {
		return fmt.Errorf("grep: missing pattern")
	}
	if caseInsensitive {
		pattern = "(?i)" + pattern
	}
	if wholeWord {
		pattern = `\b` + pattern + `\b`
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("grep: invalid pattern: %w", err)
	}

	multiFile := len(files) > 1 || recursive
	matched := false

	if len(files) == 0 {
		m, err := grepReader(hc.Stdin, hc.Stdout, "", re, invertMatch, showLineNums, countMode, listFiles, onlyMatch, multiFile)
		if err != nil {
			return err
		}
		matched = m
	} else {
		for _, f := range files {
			abs := sc.ResolvePath(f)
			info, statErr := sc.FS.Stat(abs)
			if statErr != nil {
				fmt.Fprintf(hc.Stderr, "grep: %s: No such file or directory\n", f)
				continue
			}
			if info.IsDir() {
				if !recursive {
					fmt.Fprintf(hc.Stderr, "grep: %s: Is a directory\n", f)
					continue
				}
				_ = afero.Walk(sc.FS, abs, func(path string, fi os.FileInfo, walkErr error) error {
					if walkErr != nil || fi.IsDir() {
						return nil
					}
					r, openErr := sc.FS.Open(path)
					if openErr != nil {
						return nil
					}
					defer r.Close()
					label := ""
					if multiFile {
						label = path
					}
					m, _ := grepReader(r, hc.Stdout, label, re, invertMatch, showLineNums, countMode, listFiles, onlyMatch, multiFile)
					if m {
						matched = true
					}
					return nil
				})
				continue
			}
			r, openErr := sc.FS.Open(abs)
			if openErr != nil {
				fmt.Fprintf(hc.Stderr, "grep: %s: %v\n", f, openErr)
				continue
			}
			label := ""
			if multiFile {
				label = f
			}
			m, grepErr := grepReader(r, hc.Stdout, label, re, invertMatch, showLineNums, countMode, listFiles, onlyMatch, multiFile)
			r.Close()
			if m {
				matched = true
			}
			if grepErr != nil {
				return grepErr
			}
		}
	}

	if !matched && !invertMatch {
		return interp.ExitStatus(1)
	}
	return nil
}

func grepReader(r io.Reader, w io.Writer, label string, re *regexp.Regexp, invert, lineNums, countMode, listFiles, onlyMatch, multiFile bool) (bool, error) {
	sc := bufio.NewScanner(r)
	lineNum := 0
	matched := false
	matchCount := 0
	for sc.Scan() {
		lineNum++
		line := sc.Text()
		isMatch := re.MatchString(line)
		if invert {
			isMatch = !isMatch
		}
		if !isMatch {
			continue
		}
		matched = true
		matchCount++

		if listFiles {
			if label != "" {
				fmt.Fprintln(w, label)
			}
			return true, nil
		}
		if countMode {
			continue
		}

		prefix := ""
		if multiFile && label != "" {
			prefix = label + ":"
		}
		if onlyMatch {
			locs := re.FindAllStringIndex(line, -1)
			for _, loc := range locs {
				if lineNums {
					fmt.Fprintf(w, "%s%d:%s\n", prefix, lineNum, line[loc[0]:loc[1]])
				} else {
					fmt.Fprintf(w, "%s%s\n", prefix, line[loc[0]:loc[1]])
				}
			}
			continue
		}
		if lineNums {
			fmt.Fprintf(w, "%s%d:%s\n", prefix, lineNum, line)
		} else {
			fmt.Fprintf(w, "%s%s\n", prefix, line)
		}
	}
	if countMode && matched {
		prefix := ""
		if multiFile && label != "" {
			prefix = label + ":"
		}
		fmt.Fprintf(w, "%s%d\n", prefix, matchCount)
	}
	return matched, sc.Err()
}

// ensure GrepPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = GrepPlugin{}
