package git

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// ---------------------------------------------------------------------------
// git commit
// ---------------------------------------------------------------------------

func cmdGitCommit(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	var msg string
	var authorName, authorEmail string
	allowEmpty := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-m", "--message":
			i++
			if i >= len(args) {
				return errors.New("git commit: option '-m' requires a value")
			}
			msg = args[i]
		case "--allow-empty":
			allowEmpty = true
		case "--author":
			i++
			if i >= len(args) {
				return errors.New("git commit: option '--author' requires a value")
			}
			authorName, authorEmail = parseAuthor(args[i])
		default:
			// Ignore unknown flags for forward compatibility.
		}
	}

	if msg == "" {
		fmt.Fprintln(errW, "git commit: please supply the commit message with the -m option")
		return interp.ExitStatus(1)
	}

	sig := defaultSignature(authorName, authorEmail)

	opts := &gogit.CommitOptions{
		Author:            sig,
		AllowEmptyCommits: allowEmpty,
	}

	hash, err := wt.Commit(msg, opts)
	if err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	head, err := repo.Head()
	branch := "HEAD"
	if err == nil && head.Name().IsBranch() {
		branch = head.Name().Short()
	}
	fmt.Fprintf(w, "[%s %s] %s\n", branch, hash.String()[:7], msg)
	return nil
}

// parseAuthor parses "Name <email>" into name, email components.
func parseAuthor(s string) (name, email string) {
	before, rest, ok := strings.Cut(s, "<")
	if !ok {
		return s, ""
	}
	email, _, _ = strings.Cut(rest, ">")
	return strings.TrimSpace(before), email
}

func defaultSignature(name, email string) *object.Signature {
	if name == "" {
		name = "User"
	}
	if email == "" {
		email = "user@example.com"
	}
	return &object.Signature{
		Name:  name,
		Email: email,
		When:  time.Now(),
	}
}
