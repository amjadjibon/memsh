package git

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/afero"
)

// ---------------------------------------------------------------------------
// git rm
// ---------------------------------------------------------------------------

func cmdGitRm(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, root, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	cached := false
	var paths []string
	for _, a := range args {
		switch a {
		case "--cached":
			cached = true
		default:
			paths = append(paths, a)
		}
	}
	_ = cached // go-git Remove always updates the index

	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, p)
		}
		rel, err := relToRoot(root, abs)
		if err != nil {
			return fmt.Errorf("git rm %s: %w", p, err)
		}
		if _, err := wt.Remove(rel); err != nil {
			fmt.Fprintf(errW, "git rm: pathspec '%s' did not match any files\n", p)
		} else {
			fmt.Fprintf(w, "rm '%s'\n", rel)
		}
	}
	return nil
}
