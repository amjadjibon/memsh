package git

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// ---------------------------------------------------------------------------
// git branch
// ---------------------------------------------------------------------------

func cmdGitBranch(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	deleteName := ""
	forceDelete := false
	var newName string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-d":
			i++
			if i < len(args) {
				deleteName = args[i]
			}
		case "-D":
			i++
			if i < len(args) {
				deleteName = args[i]
				forceDelete = true
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				newName = args[i]
			}
		}
	}
	_ = forceDelete

	if deleteName != "" {
		ref := plumbing.NewBranchReferenceName(deleteName)
		if err := repo.Storer.RemoveReference(ref); err != nil {
			fmt.Fprintf(errW, "git branch: error: branch '%s' not found\n", deleteName)
			return interp.ExitStatus(1)
		}
		fmt.Fprintf(w, "Deleted branch %s.\n", deleteName)
		return nil
	}

	if newName != "" {
		// Create branch at HEAD.
		head, err := repo.Head()
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			fmt.Fprintln(errW, "git branch: fatal: not a valid object name: 'HEAD'")
			return interp.ExitStatus(1)
		}
		if err != nil {
			return err
		}
		ref := plumbing.NewHashReference(
			plumbing.NewBranchReferenceName(newName),
			head.Hash(),
		)
		return repo.Storer.SetReference(ref)
	}

	// List branches.
	head, _ := repo.Head()
	iter, err := repo.Branches()
	if err != nil {
		return err
	}
	return iter.ForEach(func(ref *plumbing.Reference) error {
		prefix := "  "
		if head != nil && ref.Name() == head.Name() {
			prefix = "* "
		}
		fmt.Fprintf(w, "%s%s\n", prefix, ref.Name().Short())
		return nil
	})
}
