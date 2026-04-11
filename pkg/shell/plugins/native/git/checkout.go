package git

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// ---------------------------------------------------------------------------
// git checkout
// ---------------------------------------------------------------------------

func cmdGitCheckout(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, root, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	createBranch := false
	var target string

	for _, a := range args {
		switch a {
		case "-b":
			createBranch = true
		default:
			if !strings.HasPrefix(a, "-") {
				target = a
			}
		}
	}

	if target == "" {
		fmt.Fprintln(errW, "git checkout: no target specified")
		return interp.ExitStatus(1)
	}

	if createBranch {
		opts := &gogit.CheckoutOptions{
			Branch: plumbing.NewBranchReferenceName(target),
			Create: true,
		}
		if err := wt.Checkout(opts); err != nil {
			return fmt.Errorf("git checkout: %w", err)
		}
		fmt.Fprintf(w, "Switched to a new branch '%s'\n", target)
		return nil
	}

	// Try branch checkout first.
	opts := &gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(target),
	}
	if err := wt.Checkout(opts); err == nil {
		fmt.Fprintf(w, "Switched to branch '%s'\n", target)
		return nil
	}

	// Try as a file path to restore from index / HEAD.
	abs := target
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, target)
	}
	rel, relErr := relToRoot(root, abs)
	if relErr != nil {
		return fmt.Errorf("git checkout: %w", relErr)
	}
	// Restore file from HEAD.
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("git checkout: %w", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return err
	}
	tree, err := commit.Tree()
	if err != nil {
		return err
	}
	f, err := tree.File(rel)
	if err != nil {
		fmt.Fprintf(errW, "git checkout: pathspec '%s' did not match any file(s) known to git\n", target)
		return interp.ExitStatus(1)
	}
	content, err := f.Contents()
	if err != nil {
		return err
	}
	return afero.WriteFile(fs, abs, []byte(content), 0o644)
}
