package git

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// ---------------------------------------------------------------------------
// git remote
// ---------------------------------------------------------------------------

func cmdGitRemote(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		// List remote names.
		remotes, err := repo.Remotes()
		if err != nil {
			return fmt.Errorf("git remote: %w", err)
		}
		for _, r := range remotes {
			fmt.Fprintln(w, r.Config().Name)
		}
		return nil
	}

	switch args[0] {
	case "-v":
		remotes, err := repo.Remotes()
		if err != nil {
			return fmt.Errorf("git remote: %w", err)
		}
		for _, r := range remotes {
			cfg := r.Config()
			for _, url := range cfg.URLs {
				fmt.Fprintf(w, "%s\t%s (fetch)\n", cfg.Name, url)
				fmt.Fprintf(w, "%s\t%s (push)\n", cfg.Name, url)
			}
		}
		return nil

	case "add":
		if len(args) < 3 {
			fmt.Fprintln(errW, "usage: git remote add <name> <url>")
			return interp.ExitStatus(1)
		}
		name, url := args[1], args[2]
		_, err := repo.CreateRemote(&config.RemoteConfig{
			Name: name,
			URLs: []string{url},
		})
		if err != nil {
			return fmt.Errorf("git remote add: %w", err)
		}
		return nil

	case "remove", "rm":
		if len(args) < 2 {
			fmt.Fprintln(errW, "usage: git remote remove <name>")
			return interp.ExitStatus(1)
		}
		if err := repo.DeleteRemote(args[1]); err != nil {
			return fmt.Errorf("git remote remove: %w", err)
		}
		return nil

	case "rename":
		if len(args) < 3 {
			fmt.Fprintln(errW, "usage: git remote rename <old> <new>")
			return interp.ExitStatus(1)
		}
		oldName, newName := args[1], args[2]
		r, err := repo.Remote(oldName)
		if err != nil {
			return fmt.Errorf("git remote rename: %w", err)
		}
		urls := r.Config().URLs
		if err := repo.DeleteRemote(oldName); err != nil {
			return fmt.Errorf("git remote rename: %w", err)
		}
		_, err = repo.CreateRemote(&config.RemoteConfig{
			Name: newName,
			URLs: urls,
		})
		if err != nil {
			return fmt.Errorf("git remote rename: %w", err)
		}
		return nil

	case "set-url":
		if len(args) < 3 {
			fmt.Fprintln(errW, "usage: git remote set-url <name> <url>")
			return interp.ExitStatus(1)
		}
		name, newURL := args[1], args[2]
		cfg, err := repo.Config()
		if err != nil {
			return fmt.Errorf("git remote set-url: %w", err)
		}
		rc, ok := cfg.Remotes[name]
		if !ok {
			fmt.Fprintf(errW, "git remote set-url: No such remote '%s'\n", name)
			return interp.ExitStatus(1)
		}
		rc.URLs = []string{newURL}
		return repo.Storer.SetConfig(cfg)

	case "get-url":
		if len(args) < 2 {
			fmt.Fprintln(errW, "usage: git remote get-url <name>")
			return interp.ExitStatus(1)
		}
		r, err := repo.Remote(args[1])
		if err != nil {
			return fmt.Errorf("git remote get-url: %w", err)
		}
		for _, url := range r.Config().URLs {
			fmt.Fprintln(w, url)
		}
		return nil

	default:
		fmt.Fprintf(errW, "git remote: unknown subcommand '%s'\n", args[0])
		return interp.ExitStatus(1)
	}
}

// ---------------------------------------------------------------------------
// git fetch
// ---------------------------------------------------------------------------

func cmdGitFetch(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	remoteName := "origin"
	depth := 0

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--depth":
			i++
			if i < len(args) {
				depth, _ = strconv.Atoi(args[i])
			}
		default:
			if after, ok := strings.CutPrefix(args[i], "--depth="); ok {
				depth, _ = strconv.Atoi(after)
			} else if !strings.HasPrefix(args[i], "-") {
				remoteName = args[i]
			}
		}
	}

	err = repo.Fetch(&gogit.FetchOptions{
		RemoteName: remoteName,
		Depth:      depth,
		Progress:   w,
	})
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return fmt.Errorf("git fetch: %w", err)
	}
	if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		fmt.Fprintln(w, "Already up to date.")
	}
	_ = errW
	return nil
}

// ---------------------------------------------------------------------------
// git pull
// ---------------------------------------------------------------------------

func cmdGitPull(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	remoteName := "origin"
	var branchName plumbing.ReferenceName
	// --rebase is accepted but falls back to merge
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--rebase":
			// accepted, ignored
		default:
			if !strings.HasPrefix(args[i], "-") {
				if remoteName == "origin" || remoteName == "" {
					remoteName = args[i]
				} else {
					branchName = plumbing.NewBranchReferenceName(args[i])
				}
			}
		}
	}

	pullOpts := &gogit.PullOptions{
		RemoteName:    remoteName,
		ReferenceName: branchName,
		Progress:      w,
	}

	err = wt.Pull(pullOpts)
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return fmt.Errorf("git pull: %w", err)
	}
	if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		fmt.Fprintln(w, "Already up to date.")
	}
	_ = errW
	return nil
}

// ---------------------------------------------------------------------------
// git push
// ---------------------------------------------------------------------------

func cmdGitPush(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	remoteName := "origin"
	force := false
	tags := false
	var refSpecs []config.RefSpec

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-f", "--force":
			force = true
		case "--tags":
			tags = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				if remoteName == "origin" {
					remoteName = args[i]
				} else {
					refSpecs = append(refSpecs, config.RefSpec(args[i]))
				}
			}
		}
	}

	pushOpts := &gogit.PushOptions{
		RemoteName: remoteName,
		Force:      force,
		Progress:   w,
	}
	if tags {
		pushOpts.RefSpecs = append(pushOpts.RefSpecs, config.RefSpec("refs/tags/*:refs/tags/*"))
	}
	if len(refSpecs) > 0 {
		pushOpts.RefSpecs = append(pushOpts.RefSpecs, refSpecs...)
	}

	err = repo.Push(pushOpts)
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return fmt.Errorf("git push: %w", err)
	}
	if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		fmt.Fprintln(w, "Everything up-to-date")
	}
	_ = errW
	return nil
}

// ---------------------------------------------------------------------------
// git ls-remote
// ---------------------------------------------------------------------------

func cmdGitLsRemote(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	var urlOrName string
	headsOnly := false
	tagsOnly := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--heads":
			headsOnly = true
		case "--tags":
			tagsOnly = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				urlOrName = args[i]
			}
		}
	}

	if urlOrName == "" {
		fmt.Fprintln(errW, "git ls-remote: you must specify a URL or remote name")
		return interp.ExitStatus(1)
	}

	// Check if it's a named remote in the repo first.
	resolvedURL := urlOrName
	if repo, _, err := openRepo(fs, cwd); err == nil {
		if r, err := repo.Remote(urlOrName); err == nil {
			if len(r.Config().URLs) > 0 {
				resolvedURL = r.Config().URLs[0]
			}
		}
	}

	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{resolvedURL},
	})

	refs, err := remote.List(&gogit.ListOptions{})
	if err != nil {
		return fmt.Errorf("git ls-remote: %w", err)
	}

	for _, ref := range refs {
		name := ref.Name().String()
		if headsOnly && !strings.HasPrefix(name, "refs/heads/") {
			continue
		}
		if tagsOnly && !strings.HasPrefix(name, "refs/tags/") {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\n", ref.Hash().String(), name)
	}
	return nil
}
