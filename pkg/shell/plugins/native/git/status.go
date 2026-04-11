package git

import (
	"errors"
	"fmt"
	"io"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/spf13/afero"
)

// ---------------------------------------------------------------------------
// git status
// ---------------------------------------------------------------------------

func cmdGitStatus(w io.Writer, fs afero.Fs, cwd string, _ []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	// Print current branch.
	head, err := repo.Head()
	if err != nil && !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return fmt.Errorf("git status: %w", err)
	}
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		fmt.Fprintln(w, "On branch main")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "No commits yet")
	} else if head.Name().IsBranch() {
		fmt.Fprintf(w, "On branch %s\n", head.Name().Short())
	} else {
		fmt.Fprintf(w, "HEAD detached at %s\n", head.Hash().String()[:7])
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	status, err := wt.Status()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}

	if status.IsClean() {
		fmt.Fprintln(w, "nothing to commit, working tree clean")
		return nil
	}

	// Collect entries by category.
	var staged, changed, untracked []string
	for path, s := range status {
		if s.Staging != gogit.Unmodified && s.Staging != gogit.Untracked {
			staged = append(staged, fmt.Sprintf("\t%s: %s", stagingLabel(s.Staging), path))
		}
		if s.Worktree != gogit.Unmodified && s.Worktree != gogit.Untracked {
			changed = append(changed, fmt.Sprintf("\t%s: %s", worktreeLabel(s.Worktree), path))
		}
		if s.Worktree == gogit.Untracked {
			untracked = append(untracked, "\t"+path)
		}
	}

	if len(staged) > 0 {
		fmt.Fprintln(w, "Changes to be committed:")
		for _, l := range staged {
			fmt.Fprintln(w, l)
		}
		fmt.Fprintln(w, "")
	}
	if len(changed) > 0 {
		fmt.Fprintln(w, "Changes not staged for commit:")
		for _, l := range changed {
			fmt.Fprintln(w, l)
		}
		fmt.Fprintln(w, "")
	}
	if len(untracked) > 0 {
		fmt.Fprintln(w, "Untracked files:")
		for _, l := range untracked {
			fmt.Fprintln(w, l)
		}
		fmt.Fprintln(w, "")
	}
	return nil
}

func stagingLabel(c gogit.StatusCode) string {
	switch c {
	case gogit.Added:
		return "new file"
	case gogit.Modified:
		return "modified"
	case gogit.Deleted:
		return "deleted"
	case gogit.Renamed:
		return "renamed"
	case gogit.Copied:
		return "copied"
	default:
		return string(c)
	}
}

func worktreeLabel(c gogit.StatusCode) string {
	switch c {
	case gogit.Modified:
		return "modified"
	case gogit.Deleted:
		return "deleted"
	default:
		return string(c)
	}
}
