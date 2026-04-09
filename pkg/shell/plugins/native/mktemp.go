package native

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

const randChars = "abcdefghijklmnopqrstuvwxyz0123456789"

// MktempPlugin creates a temporary file or directory in the virtual FS.
//
//	mktemp                      /tmp/tmp.XXXXXXXXXX
//	mktemp -d                   /tmp/tmp.XXXXXXXXXX  (directory)
//	mktemp -p /var/tmp          use alternate base dir
//	mktemp --suffix .json       append suffix after substituted Xs
//	mktemp /tmp/myapp.XXXXXX    custom template
//	mktemp -u TEMPLATE          dry-run: print name without creating
type MktempPlugin struct{}

func (MktempPlugin) Name() string        { return "mktemp" }
func (MktempPlugin) Description() string { return "create a temporary file or directory" }
func (MktempPlugin) Usage() string {
	return "mktemp [-d] [-u] [-q] [-p dir] [--suffix suf] [TEMPLATE]"
}

func (MktempPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	isDir := false
	dryRun := false
	quiet := false
	baseDir := ""
	suffix := ""
	template := ""

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			template = a
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		switch a {
		case "--directory":
			isDir = true
			continue
		case "--dry-run":
			dryRun = true
			continue
		case "--quiet":
			quiet = true
			continue
		case "-p", "--tmpdir":
			if i+1 >= len(args) {
				return fmt.Errorf("mktemp: option requires an argument -- 'p'")
			}
			i++
			baseDir = args[i]
			continue
		case "--suffix":
			if i+1 >= len(args) {
				return fmt.Errorf("mktemp: option requires an argument -- 'suffix'")
			}
			i++
			suffix = args[i]
			continue
		}
		// combined short flags
		unknown := ""
		for j := 1; j < len(a); j++ {
			c := a[j]
			switch c {
			case 'd':
				isDir = true
			case 'u':
				dryRun = true
			case 'q':
				quiet = true
			case 'p':
				if j+1 < len(a) {
					baseDir = a[j+1:]
					j = len(a)
				} else if i+1 < len(args) {
					i++
					baseDir = args[i]
				} else {
					return fmt.Errorf("mktemp: option requires an argument -- 'p'")
				}
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("mktemp: invalid option -- '%s'", unknown)
		}
	}

	// resolve template
	if template == "" {
		dir := "/tmp"
		if baseDir != "" {
			dir = sc.ResolvePath(baseDir)
		}
		template = dir + "/tmp.XXXXXXXXXX"
	} else {
		// if the template has no directory component, prepend baseDir or /tmp
		if !strings.Contains(template, "/") {
			dir := "/tmp"
			if baseDir != "" {
				dir = sc.ResolvePath(baseDir)
			}
			template = dir + "/" + template
		} else {
			template = sc.ResolvePath(template)
		}
	}

	path, err := mktempGenerate(sc, template, suffix, isDir, dryRun)
	if err != nil {
		if !quiet {
			fmt.Fprintf(hc.Stderr, "mktemp: %v\n", err)
		}
		return interp.ExitStatus(1)
	}

	fmt.Fprintln(hc.Stdout, path)
	return nil
}

// mktempGenerate resolves the template, substitutes Xs, appends suffix, and creates the file/dir.
func mktempGenerate(sc plugins.ShellContext, template, suffix string, isDir, dryRun bool) (string, error) {
	// count trailing Xs in the base name (before any suffix after the last X run)
	// GNU mktemp substitutes the last contiguous run of X's.
	lastRun := lastXRun(template)
	if lastRun < 0 {
		return "", fmt.Errorf("template '%s' must end in at least 3 Xs", template)
	}
	if len(template)-lastRun < 3 {
		return "", fmt.Errorf("template '%s' must end in at least 3 Xs", template)
	}

	prefix := template[:lastRun]
	xCount := 0
	for i := lastRun; i < len(template); i++ {
		if template[i] == 'X' {
			xCount++
		} else {
			break
		}
	}
	// any characters after the X-run in the template become part of the suffix
	suffix = template[lastRun+xCount:] + suffix

	// ensure parent directory exists
	parentDir := dirOf(prefix)
	if parentDir != "" && parentDir != "/" {
		if err := sc.FS.MkdirAll(parentDir, 0o755); err != nil {
			return "", fmt.Errorf("cannot create directory '%s': %w", parentDir, err)
		}
	}

	// try up to 100 random names
	for range 100 {
		rand := randString(xCount)
		path := prefix + rand + suffix
		_, err := sc.FS.Stat(path)
		if err == nil {
			continue // exists, try another
		}
		if dryRun {
			return path, nil
		}
		if isDir {
			if err := sc.FS.Mkdir(path, 0o700); err != nil {
				continue
			}
		} else {
			f, err := sc.FS.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				continue
			}
			f.Close()
		}
		return path, nil
	}
	return "", fmt.Errorf("could not create temp path from template '%s'", template)
}

// lastXRun returns the index of the start of the last contiguous run of 'X'
// characters in s, or -1 if none.
func lastXRun(s string) int {
	i := len(s) - 1
	for i >= 0 && s[i] == 'X' {
		i--
	}
	if i == len(s)-1 {
		return -1 // no trailing Xs
	}
	return i + 1
}

// dirOf returns the directory portion of a path (everything before the last /).
func dirOf(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return ""
	}
	if i == 0 {
		return "/"
	}
	return path[:i]
}

// randString generates a random string of n characters from randChars.
func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = randChars[rand.Intn(len(randChars))]
	}
	return string(b)
}

// ensure MktempPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = MktempPlugin{}
