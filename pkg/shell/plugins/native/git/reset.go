package git

import (
	"errors"
	"fmt"
	"io"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// ---------------------------------------------------------------------------
// git reset
// ---------------------------------------------------------------------------

func cmdGitReset(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	mode := gogit.MixedReset
	var target string

	for _, a := range args {
		switch a {
		case "--soft":
			mode = gogit.SoftReset
		case "--mixed":
			mode = gogit.MixedReset
		case "--hard":
			mode = gogit.HardReset
		default:
			if !strings.HasPrefix(a, "-") {
				target = a
			}
		}
	}

	var hash plumbing.Hash
	if target == "" || target == "HEAD" {
		head, err := repo.Head()
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			fmt.Fprintln(errW, "git reset: no commits yet")
			return interp.ExitStatus(1)
		}
		if err != nil {
			return err
		}
		hash = head.Hash()
	} else {
		h, err := repo.ResolveRevision(plumbing.Revision(target))
		if err != nil {
			return fmt.Errorf("git reset: %w", err)
		}
		hash = *h
	}

	if err := wt.Reset(&gogit.ResetOptions{
		Commit: hash,
		Mode:   mode,
	}); err != nil {
		return fmt.Errorf("git reset: %w", err)
	}
	fmt.Fprintf(w, "HEAD is now at %s\n", hash.String()[:7])
	return nil
}
