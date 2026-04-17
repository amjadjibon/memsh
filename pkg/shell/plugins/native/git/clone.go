package git

import (
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// ---------------------------------------------------------------------------
// git clone
// ---------------------------------------------------------------------------

// repoNameFromURL extracts a directory name from a clone URL, stripping the
// trailing ".git" suffix if present (mirrors real git behaviour).
func repoNameFromURL(rawURL string) string {
	base := path.Base(rawURL)
	base = strings.TrimSuffix(base, ".git")
	if base == "" || base == "." || base == "/" {
		return "repo"
	}
	return base
}

// cmdGitClone clones a remote (or local) repository into the virtual FS.
//
//	git clone [--depth <n>] [-b <branch>] [--single-branch] [-n] [--no-checkout] <url> [<directory>]
func cmdGitClone(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string, getEnv envLookup) error {
	var (
		rawURL       string
		destArg      string
		branch       string
		depth        int
		singleBranch bool
		noCheckout   bool
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--depth":
			i++
			if i < len(args) {
				depth, _ = strconv.Atoi(args[i])
			}
		case "-b", "--branch":
			i++
			if i < len(args) {
				branch = args[i]
			}
		case "--single-branch":
			singleBranch = true
		case "-n", "--no-checkout":
			noCheckout = true
		default:
			if after, ok := strings.CutPrefix(args[i], "--depth="); ok {
				depth, _ = strconv.Atoi(after)
			} else if after, ok := strings.CutPrefix(args[i], "--branch="); ok {
				branch = after
			} else if !strings.HasPrefix(args[i], "-") {
				if rawURL == "" {
					rawURL = args[i]
				} else if destArg == "" {
					destArg = args[i]
				}
			}
		}
	}

	if rawURL == "" {
		fmt.Fprintln(errW, "git clone: you must specify a repository to clone")
		return interp.ExitStatus(1)
	}

	// Determine the destination directory name.
	dirName := destArg
	if dirName == "" {
		dirName = repoNameFromURL(rawURL)
	}

	// Resolve to an absolute path in the virtual FS.
	target := dirName
	if !filepath.IsAbs(target) {
		target = filepath.Join(cwd, dirName)
	}

	if _, err := fs.Stat(target); err == nil {
		fmt.Fprintf(errW, "git clone: destination path '%s' already exists and is not an empty directory\n", dirName)
		return interp.ExitStatus(1)
	}

	if err := fs.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	fmt.Fprintf(w, "Cloning into '%s'...\n", dirName)

	storer := openStorage(fs, target)
	wt := &aferoFS{fs: fs, root: target}

	cloneOpts := &gogit.CloneOptions{
		URL:          rawURL,
		Progress:     w,
		NoCheckout:   noCheckout,
		SingleBranch: singleBranch,
		Depth:        depth,
		Auth:         authFromEnv(getEnv, rawURL),
	}
	if branch != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(branch)
	}

	if _, err := gogit.Clone(storer, wt, cloneOpts); err != nil {
		// Remove the partially initialised directory on failure.
		_ = fs.RemoveAll(target)
		return fmt.Errorf("git clone: %w", err)
	}

	return nil
}
