package git

import (
	"fmt"
	"io"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// ---------------------------------------------------------------------------
// git merge
// ---------------------------------------------------------------------------

func cmdGitMerge(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	noFF := false
	squash := false
	var branchArg string

	for _, a := range args {
		switch a {
		case "--no-ff":
			noFF = true
		case "--squash":
			squash = true
		default:
			if !strings.HasPrefix(a, "-") {
				branchArg = a
			}
		}
	}

	if squash {
		fmt.Fprintln(errW, "git merge: --squash is not supported")
		return interp.ExitStatus(1)
	}
	if noFF {
		fmt.Fprintln(errW, "git merge: --no-ff is not supported; only fast-forward merges are implemented")
		return interp.ExitStatus(1)
	}

	if branchArg == "" {
		fmt.Fprintln(errW, "git merge: no branch specified")
		return interp.ExitStatus(1)
	}

	// Resolve current HEAD.
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("git merge: %w", err)
	}
	currentHash := head.Hash()

	// Resolve target branch ref.
	targetRef, err := repo.Reference(plumbing.NewBranchReferenceName(branchArg), true)
	if err != nil {
		// Try as a revision.
		h, err2 := repo.ResolveRevision(plumbing.Revision(branchArg))
		if err2 != nil {
			fmt.Fprintf(errW, "git merge: branch '%s' not found\n", branchArg)
			return interp.ExitStatus(1)
		}
		targetHash := *h
		return doFastForward(w, errW, repo, head, currentHash, targetHash)
	}
	targetHash := targetRef.Hash()

	return doFastForward(w, errW, repo, head, currentHash, targetHash)
}

func doFastForward(w io.Writer, errW io.Writer, repo *gogit.Repository, head *plumbing.Reference, currentHash, targetHash plumbing.Hash) error {
	if currentHash == targetHash {
		fmt.Fprintln(w, "Already up to date.")
		return nil
	}

	// Check if currentHash is an ancestor of targetHash (fast-forward possible).
	iter, err := repo.Log(&gogit.LogOptions{From: targetHash})
	if err != nil {
		return fmt.Errorf("git merge: %w", err)
	}
	defer iter.Close()

	isFastForward := false
	count := 0
	_ = iter.ForEach(func(c *object.Commit) error {
		if count > 1000 {
			return fmt.Errorf("stop")
		}
		count++
		if c.Hash == currentHash {
			isFastForward = true
			return fmt.Errorf("stop")
		}
		return nil
	})

	if !isFastForward {
		fmt.Fprintln(errW, "git merge: not possible to fast-forward; refusing merge")
		return interp.ExitStatus(1)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	// Remember current branch name so we can restore the ref after detach.
	branchRefName := head.Name()

	// Checkout target hash (may detach HEAD).
	if err := wt.Checkout(&gogit.CheckoutOptions{
		Hash:  targetHash,
		Force: true,
	}); err != nil {
		return fmt.Errorf("git merge: checkout failed: %w", err)
	}

	// If we were on a branch, restore the branch ref to point at targetHash.
	if branchRefName.IsBranch() {
		newRef := plumbing.NewHashReference(branchRefName, targetHash)
		if err := repo.Storer.SetReference(newRef); err != nil {
			return fmt.Errorf("git merge: update ref failed: %w", err)
		}
		// Also update HEAD to point to the branch (not detached).
		headRef := plumbing.NewSymbolicReference(plumbing.HEAD, branchRefName)
		if err := repo.Storer.SetReference(headRef); err != nil {
			return fmt.Errorf("git merge: update HEAD failed: %w", err)
		}
	}

	fmt.Fprintf(w, "Fast-forward\n")
	fmt.Fprintf(w, "HEAD is now at %s\n", targetHash.String()[:7])
	return nil
}
