package git

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/afero"
)

// ---------------------------------------------------------------------------
// git add
// ---------------------------------------------------------------------------

func cmdGitAdd(_ io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, root, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		fmt.Fprintln(errW, "Nothing specified, nothing added.")
		return nil
	}

	for _, arg := range args {
		if arg == "." || arg == "-A" || arg == "--all" {
			if err := wt.AddGlob("."); err != nil {
				return fmt.Errorf("git add: %w", err)
			}
			continue
		}
		// Resolve to absolute then make relative to worktree root.
		abs := arg
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, arg)
		}
		rel, err := relToRoot(root, abs)
		if err != nil {
			return fmt.Errorf("git add %s: %w", arg, err)
		}
		if _, err := wt.Add(rel); err != nil {
			return fmt.Errorf("git add %s: %w", arg, err)
		}
	}
	return nil
}
