package git

import (
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/afero"
)

// ---------------------------------------------------------------------------
// git show
// ---------------------------------------------------------------------------

func cmdGitShow(w io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	var rev string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		rev = args[0]
	} else {
		rev = "HEAD"
	}

	h, err := repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return fmt.Errorf("git show: %w", err)
	}
	commit, err := repo.CommitObject(*h)
	if err != nil {
		return fmt.Errorf("git show: %w", err)
	}

	fmt.Fprintf(w, "commit %s\n", commit.Hash.String())
	fmt.Fprintf(w, "Author: %s <%s>\n", commit.Author.Name, commit.Author.Email)
	fmt.Fprintf(w, "Date:   %s\n", commit.Author.When.Format("Mon Jan 2 15:04:05 2006 -0700"))
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "    %s\n", commit.Message)
	fmt.Fprintln(w, "")

	// Show changed files.
	if commit.NumParents() == 0 {
		tree, err := commit.Tree()
		if err != nil {
			return err
		}
		return tree.Files().ForEach(func(f *object.File) error {
			fmt.Fprintf(w, "diff --git a/%s b/%s\n", f.Name, f.Name)
			fmt.Fprintf(w, "new file mode %04o\n", f.Mode)
			fmt.Fprintf(w, "+++ b/%s\n", f.Name)
			content, err := f.Contents()
			if err != nil {
				return nil
			}
			for _, line := range splitLines(content) {
				fmt.Fprintf(w, "+%s\n", line)
			}
			return nil
		})
	}

	parent, err := commit.Parents().Next()
	if err != nil {
		return nil
	}
	patch, err := parent.Patch(commit)
	if err != nil {
		return nil
	}
	fmt.Fprintln(w, patch.String())
	return nil
}
