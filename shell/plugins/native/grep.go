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
	var pattern string
	var files []string

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-i":
			caseInsensitive = true
		case "-n":
			showLineNums = true
		case "-v":
			invertMatch = true
		case "-r", "-R":
			recursive = true
		default:
			if pattern == "" {
				pattern = args[i]
			} else {
				files = append(files, args[i])
			}
		}
	}

	if pattern == "" {
		return fmt.Errorf("grep: missing pattern")
	}
	if caseInsensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("grep: invalid pattern: %v", err)
	}

	multiFile := len(files) > 1 || recursive
	matched := false

	if len(files) == 0 {
		m, err := grepReader(hc.Stdin, hc.Stdout, "", re, invertMatch, showLineNums)
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
					m, _ := grepReader(r, hc.Stdout, label, re, invertMatch, showLineNums)
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
				label = abs
			}
			m, grepErr := grepReader(r, hc.Stdout, label, re, invertMatch, showLineNums)
			r.Close()
			if m {
				matched = true
			}
			if grepErr != nil {
				return grepErr
			}
		}
	}

	if !matched {
		return interp.ExitStatus(1)
	}
	return nil
}

func grepReader(r io.Reader, w io.Writer, label string, re *regexp.Regexp, invert, lineNums bool) (bool, error) {
	sc := bufio.NewScanner(r)
	lineNum := 0
	matched := false
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
		switch {
		case label != "" && lineNums:
			fmt.Fprintf(w, "%s:%d:%s\n", label, lineNum, line)
		case label != "":
			fmt.Fprintf(w, "%s:%s\n", label, line)
		case lineNums:
			fmt.Fprintf(w, "%d:%s\n", lineNum, line)
		default:
			fmt.Fprintln(w, line)
		}
	}
	return matched, sc.Err()
}

// ensure GrepPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = GrepPlugin{}
