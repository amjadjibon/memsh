package git

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/spf13/afero"
)

// ---------------------------------------------------------------------------
// git init
// ---------------------------------------------------------------------------

func cmdGitInit(w io.Writer, fs afero.Fs, cwd string, args []string) error {
	quiet := false
	branch := ""
	var dirs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			dirs = append(dirs, args[i+1:]...)
			i = len(args)
		case a == "-q" || a == "--quiet":
			quiet = true
		case a == "-b" || a == "--initial-branch":
			if i+1 >= len(args) {
				return fmt.Errorf("git init: option '%s' requires a value", a)
			}
			i++
			branch = args[i]
		case strings.HasPrefix(a, "--initial-branch="):
			branch = strings.TrimPrefix(a, "--initial-branch=")
		case strings.HasPrefix(a, "-") && a != "-":
			return fmt.Errorf("git init: unknown option '%s'", a)
		default:
			dirs = append(dirs, a)
		}
	}
	if len(dirs) > 1 {
		return fmt.Errorf("git init: too many arguments")
	}

	target := cwd
	if len(dirs) == 1 {
		if filepath.IsAbs(dirs[0]) {
			target = dirs[0]
		} else {
			target = filepath.Join(cwd, dirs[0])
		}
	}
	if err := fs.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	opts := gogit.InitOptions{}
	if branch != "" {
		opts.DefaultBranch = plumbing.NewBranchReferenceName(branch)
	}
	storer := openStorage(fs, target)
	wt := &aferoFS{fs: fs, root: target}
	_, err := gogit.InitWithOptions(storer, wt, opts)
	if err != nil && !errors.Is(err, gogit.ErrRepositoryAlreadyExists) {
		return fmt.Errorf("git init: %w", err)
	}
	if !quiet {
		fmt.Fprintf(w, "Initialized empty Git repository in %s/.git/\n", target)
	}
	return nil
}
