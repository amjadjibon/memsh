package git

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/afero"
)

// ---------------------------------------------------------------------------
// git log
// ---------------------------------------------------------------------------

func cmdGitLog(w io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	oneline := false
	maxCount := -1

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--oneline":
			oneline = true
		case "-n":
			i++
			if i < len(args) {
				if n, err := strconv.Atoi(args[i]); err == nil {
					maxCount = n
				}
			}
		default:
			// Handle -n<N> combined form.
			if strings.HasPrefix(args[i], "-") && len(args[i]) > 1 {
				if n, err := strconv.Atoi(args[i][1:]); err == nil {
					maxCount = n
				}
			}
		}
	}

	iter, err := repo.Log(&gogit.LogOptions{})
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			fmt.Fprintln(w, "fatal: your current branch does not have any commits yet")
			return nil
		}
		return fmt.Errorf("git log: %w", err)
	}
	defer iter.Close()

	count := 0
	return iter.ForEach(func(c *object.Commit) error {
		if maxCount >= 0 && count >= maxCount {
			return fmt.Errorf("stop") // sentinel to stop iteration
		}
		count++
		if oneline {
			fmt.Fprintf(w, "%s %s\n", c.Hash.String()[:7], firstLine(c.Message))
		} else {
			fmt.Fprintf(w, "commit %s\n", c.Hash.String())
			fmt.Fprintf(w, "Author: %s <%s>\n", c.Author.Name, c.Author.Email)
			fmt.Fprintf(w, "Date:   %s\n", c.Author.When.Format("Mon Jan 2 15:04:05 2006 -0700"))
			fmt.Fprintln(w, "")
			fmt.Fprintf(w, "    %s\n", c.Message)
			fmt.Fprintln(w, "")
		}
		return nil
	})
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
