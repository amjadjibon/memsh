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
// git switch
// ---------------------------------------------------------------------------

func cmdGitSwitch(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	create := false
	forceCreate := false
	var branchName string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-c", "--create":
			create = true
		case "-C", "--force-create":
			forceCreate = true
			create = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				branchName = args[i]
			}
		}
	}

	if branchName == "" {
		fmt.Fprintln(errW, "git switch: no branch name specified")
		return interp.ExitStatus(1)
	}

	opts := &gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: create,
		Force:  forceCreate,
	}

	if err := wt.Checkout(opts); err != nil {
		return fmt.Errorf("git switch: %w", err)
	}

	if create {
		fmt.Fprintf(w, "Switched to a new branch '%s'\n", branchName)
	} else {
		fmt.Fprintf(w, "Switched to branch '%s'\n", branchName)
	}
	return nil
}

// ---------------------------------------------------------------------------
// git restore
// ---------------------------------------------------------------------------

func cmdGitRestore(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, root, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	staged := false
	var source string
	var filePaths []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--staged", "-S":
			staged = true
		case "--worktree", "-W":
			// default, ignore
		case "--source":
			i++
			if i < len(args) {
				source = args[i]
			}
		default:
			if after, ok := strings.CutPrefix(args[i], "--source="); ok {
				source = after
			} else if !strings.HasPrefix(args[i], "-") {
				filePaths = append(filePaths, args[i])
			}
		}
	}

	if len(filePaths) == 0 {
		fmt.Fprintln(errW, "git restore: you must specify path(s) to restore")
		return interp.ExitStatus(1)
	}

	// Determine the source tree.
	var treeSource *plumbing.Hash
	if source != "" {
		h, err := repo.ResolveRevision(plumbing.Revision(source))
		if err != nil {
			fmt.Fprintf(errW, "git restore: invalid source '%s'\n", source)
			return interp.ExitStatus(1)
		}
		treeSource = h
	} else {
		head, err := repo.Head()
		if err != nil {
			return fmt.Errorf("git restore: %w", err)
		}
		treeSource = &plumbing.Hash{}
		*treeSource = head.Hash()
	}

	commit, err := repo.CommitObject(*treeSource)
	if err != nil {
		return fmt.Errorf("git restore: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return fmt.Errorf("git restore: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	for _, p := range filePaths {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, p)
		}
		rel, err := relToRoot(root, abs)
		if err != nil {
			return fmt.Errorf("git restore: %w", err)
		}

		f, err := tree.File(rel)
		if err != nil {
			fmt.Fprintf(errW, "git restore: pathspec '%s' did not match any file(s) known to git\n", p)
			return interp.ExitStatus(1)
		}
		content, err := f.Contents()
		if err != nil {
			return fmt.Errorf("git restore: %w", err)
		}

		if staged {
			// Restore index entry from HEAD — write file content then re-add.
			if err := afero.WriteFile(fs, abs, []byte(content), 0o644); err != nil {
				return fmt.Errorf("git restore: %w", err)
			}
			if _, err := wt.Add(rel); err != nil {
				return fmt.Errorf("git restore: %w", err)
			}
		} else {
			// Restore worktree.
			if err := afero.WriteFile(fs, abs, []byte(content), 0o644); err != nil {
				return fmt.Errorf("git restore: %w", err)
			}
		}
	}

	_ = w
	return nil
}
