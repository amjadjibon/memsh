package git

import (
	"errors"
	"fmt"
	"io"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// ---------------------------------------------------------------------------
// git tag
// ---------------------------------------------------------------------------

func cmdGitTag(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	// Flags
	deleteFlag := false
	annotate := false
	var message string
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-d", "--delete":
			deleteFlag = true
		case "-a", "--annotate":
			annotate = true
		case "-l", "--list":
			// explicit list flag — fall through to default list behaviour
		case "-m", "--message":
			i++
			if i < len(args) {
				message = args[i]
			}
		default:
			if args[i] != "" && args[i][0] != '-' {
				positional = append(positional, args[i])
			}
		}
	}

	// Delete
	if deleteFlag {
		for _, name := range positional {
			if err := repo.DeleteTag(name); err != nil {
				fmt.Fprintf(errW, "git tag: error deleting tag '%s': %v\n", name, err)
				return interp.ExitStatus(1)
			}
			fmt.Fprintf(w, "Deleted tag '%s'\n", name)
		}
		return nil
	}

	// List (no positional args or -l)
	if len(positional) == 0 {
		iter, err := repo.Tags()
		if err != nil {
			return fmt.Errorf("git tag: %w", err)
		}
		return iter.ForEach(func(ref *plumbing.Reference) error {
			fmt.Fprintln(w, ref.Name().Short())
			return nil
		})
	}

	// Create tag
	tagName := positional[0]
	var targetHash plumbing.Hash

	if len(positional) >= 2 {
		h, err := repo.ResolveRevision(plumbing.Revision(positional[1]))
		if err != nil {
			fmt.Fprintf(errW, "git tag: unknown revision '%s'\n", positional[1])
			return interp.ExitStatus(128)
		}
		targetHash = *h
	} else {
		head, err := repo.Head()
		if err != nil {
			if errors.Is(err, plumbing.ErrReferenceNotFound) {
				fmt.Fprintln(errW, "git tag: fatal: not a valid object name: 'HEAD'")
				return interp.ExitStatus(128)
			}
			return fmt.Errorf("git tag: %w", err)
		}
		targetHash = head.Hash()
	}

	if annotate {
		if message == "" {
			fmt.Fprintln(errW, "git tag: option '-m' required for annotated tags")
			return interp.ExitStatus(1)
		}
		sig := defaultSignature("", "")
		opts := &gogit.CreateTagOptions{
			Tagger:  &object.Signature{Name: sig.Name, Email: sig.Email, When: sig.When},
			Message: message,
		}
		if _, err := repo.CreateTag(tagName, targetHash, opts); err != nil {
			return fmt.Errorf("git tag: %w", err)
		}
	} else {
		// Lightweight tag
		if _, err := repo.CreateTag(tagName, targetHash, nil); err != nil {
			return fmt.Errorf("git tag: %w", err)
		}
	}
	return nil
}
