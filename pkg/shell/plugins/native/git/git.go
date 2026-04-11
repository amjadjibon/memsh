package git

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

// GitPlugin implements a git command backed by go-git, storing all git state
// (including .git/ contents) in the afero virtual filesystem.
//
//	git <subcommand> [args...]
type GitPlugin struct{}

func (GitPlugin) Name() string        { return "git" }
func (GitPlugin) Description() string { return "git version control backed by the virtual FS" }
func (GitPlugin) Usage() string {
	return "git <clone|init|add|rm|status|commit|log|diff|branch|checkout|reset|show|stash|" +
		"merge|fetch|pull|push|remote|tag|blame|ls-files|ls-tree|shortlog|describe|" +
		"switch|restore|cherry-pick|revert|format-patch|apply|cat-file|hash-object|ls-remote|config> [args...]"
}

var _ plugins.PluginInfo = GitPlugin{}

// Run dispatches to the appropriate subcommand.
func (GitPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	if len(args) < 2 {
		fmt.Fprintln(hc.Stderr, "usage: git <command> [args...]")
		return interp.ExitStatus(1)
	}

	fs := sc.FS
	cwd := sc.ResolvePath(".")

	// Support "git -C <path> <subcommand> [args...]" to change the working
	// directory for git operations — important when the shell cwd is a real OS
	// temp dir but the virtual repo lives at a virtual absolute path.
	remaining := args[1:]
	for len(remaining) >= 2 && remaining[0] == "-C" {
		dir := remaining[1]
		if filepath.IsAbs(dir) {
			cwd = dir
		} else {
			cwd = filepath.Join(cwd, dir)
		}
		remaining = remaining[2:]
	}
	if len(remaining) == 0 {
		fmt.Fprintln(hc.Stderr, "usage: git <command> [args...]")
		return interp.ExitStatus(1)
	}

	sub := remaining[0]
	rest := remaining[1:]

	switch sub {
	case "clone":
		return cmdGitClone(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "init":
		return cmdGitInit(hc.Stdout, fs, cwd, rest)
	case "add":
		return cmdGitAdd(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "rm":
		return cmdGitRm(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "status":
		return cmdGitStatus(hc.Stdout, fs, cwd, rest)
	case "commit":
		return cmdGitCommit(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "log":
		return cmdGitLog(hc.Stdout, fs, cwd, rest)
	case "diff":
		return cmdGitDiff(hc.Stdout, fs, cwd, rest)
	case "branch":
		return cmdGitBranch(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "checkout":
		return cmdGitCheckout(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "reset":
		return cmdGitReset(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "show":
		return cmdGitShow(hc.Stdout, fs, cwd, rest)
	case "stash":
		return cmdGitStash(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "merge":
		return cmdGitMerge(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "fetch":
		return cmdGitFetch(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "pull":
		return cmdGitPull(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "push":
		return cmdGitPush(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "remote":
		return cmdGitRemote(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "tag":
		return cmdGitTag(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "blame":
		return cmdGitBlame(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "ls-files":
		return cmdGitLsFiles(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "ls-tree":
		return cmdGitLsTree(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "shortlog":
		return cmdGitShortlog(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "describe":
		return cmdGitDescribe(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "switch":
		return cmdGitSwitch(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "restore":
		return cmdGitRestore(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "cherry-pick":
		return cmdGitCherryPick(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "revert":
		return cmdGitRevert(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "format-patch":
		return cmdGitFormatPatch(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "apply":
		return cmdGitApply(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "cat-file":
		return cmdGitCatFile(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "hash-object":
		return cmdGitHashObject(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "ls-remote":
		return cmdGitLsRemote(hc.Stdout, hc.Stderr, fs, cwd, rest)
	case "config":
		return cmdGitConfig(hc.Stdout, hc.Stderr, fs, cwd, rest)
	default:
		fmt.Fprintf(hc.Stderr, "git: '%s' is not a git command\n", sub)
		return interp.ExitStatus(1)
	}
}

// ---------------------------------------------------------------------------
// Storage / repo helpers
// ---------------------------------------------------------------------------

// findGitRoot walks up from cwd until it finds a directory containing .git.
func findGitRoot(fs afero.Fs, cwd string) (string, error) {
	dir := cwd
	for {
		info, err := fs.Stat(filepath.Join(dir, ".git"))
		if err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("not a git repository (or any of the parent directories): .git")
}

// openStorage creates a go-git filesystem.Storage rooted at <root>/.git.
func openStorage(fs afero.Fs, root string) *filesystem.Storage {
	dotgit := &aferoFS{fs: fs, root: root + "/.git"}
	return filesystem.NewStorage(dotgit, cache.NewObjectLRUDefault())
}

// openRepo opens an existing repository by walking up from cwd.
// Returns the repo, the worktree root path, and any error.
func openRepo(fs afero.Fs, cwd string) (*gogit.Repository, string, error) {
	root, err := findGitRoot(fs, cwd)
	if err != nil {
		return nil, "", err
	}
	storer := openStorage(fs, root)
	wt := &aferoFS{fs: fs, root: root}
	r, err := gogit.Open(storer, wt)
	if err != nil {
		return nil, "", err
	}
	return r, root, nil
}

// relToRoot converts an absolute path to a path relative to root.
// If path is already relative it is returned unchanged.
func relToRoot(root, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Rel(root, path)
}
