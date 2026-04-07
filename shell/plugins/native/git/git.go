package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/shell/plugins"
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
func cmdGitClone(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
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

	if err := fs.MkdirAll(target, 0755); err != nil {
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

// ---------------------------------------------------------------------------
// git init
// ---------------------------------------------------------------------------

func cmdGitInit(w io.Writer, fs afero.Fs, cwd string, args []string) error {
	target := cwd
	if len(args) > 0 {
		if filepath.IsAbs(args[0]) {
			target = args[0]
		} else {
			target = filepath.Join(cwd, args[0])
		}
	}
	if err := fs.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	storer := openStorage(fs, target)
	wt := &aferoFS{fs: fs, root: target}
	_, err := gogit.Init(storer, wt)
	if err != nil && !errors.Is(err, gogit.ErrRepositoryAlreadyExists) {
		return fmt.Errorf("git init: %w", err)
	}
	fmt.Fprintf(w, "Initialized empty Git repository in %s/.git/\n", target)
	return nil
}

// ---------------------------------------------------------------------------
// git add
// ---------------------------------------------------------------------------

func cmdGitAdd(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, root, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		fmt.Fprintln(errW, "Nothing specified, nothing added.")
		return nil
	}

	for _, arg := range args {
		if arg == "." || arg == "-A" || arg == "--all" {
			if err := wt.AddGlob("."); err != nil {
				return fmt.Errorf("git add: %w", err)
			}
			continue
		}
		// Resolve to absolute then make relative to worktree root.
		abs := arg
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, arg)
		}
		rel, err := relToRoot(root, abs)
		if err != nil {
			return fmt.Errorf("git add %s: %w", arg, err)
		}
		if _, err := wt.Add(rel); err != nil {
			return fmt.Errorf("git add %s: %w", arg, err)
		}
	}
	_ = w
	return nil
}

// ---------------------------------------------------------------------------
// git rm
// ---------------------------------------------------------------------------

func cmdGitRm(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, root, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	cached := false
	var paths []string
	for _, a := range args {
		switch a {
		case "--cached":
			cached = true
		default:
			paths = append(paths, a)
		}
	}
	_ = cached // go-git Remove always updates the index

	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, p)
		}
		rel, err := relToRoot(root, abs)
		if err != nil {
			return fmt.Errorf("git rm %s: %w", p, err)
		}
		if _, err := wt.Remove(rel); err != nil {
			fmt.Fprintf(errW, "git rm: pathspec '%s' did not match any files\n", p)
		} else {
			fmt.Fprintf(w, "rm '%s'\n", rel)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// git status
// ---------------------------------------------------------------------------

func cmdGitStatus(w io.Writer, fs afero.Fs, cwd string, _ []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	// Print current branch.
	head, err := repo.Head()
	if err != nil && !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return fmt.Errorf("git status: %w", err)
	}
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		fmt.Fprintln(w, "On branch main")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "No commits yet")
	} else if head.Name().IsBranch() {
		fmt.Fprintf(w, "On branch %s\n", head.Name().Short())
	} else {
		fmt.Fprintf(w, "HEAD detached at %s\n", head.Hash().String()[:7])
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	status, err := wt.Status()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}

	if status.IsClean() {
		fmt.Fprintln(w, "nothing to commit, working tree clean")
		return nil
	}

	// Collect entries by category.
	var staged, changed, untracked []string
	for path, s := range status {
		if s.Staging != gogit.Unmodified && s.Staging != gogit.Untracked {
			staged = append(staged, fmt.Sprintf("\t%s: %s", stagingLabel(s.Staging), path))
		}
		if s.Worktree != gogit.Unmodified && s.Worktree != gogit.Untracked {
			changed = append(changed, fmt.Sprintf("\t%s: %s", worktreeLabel(s.Worktree), path))
		}
		if s.Worktree == gogit.Untracked {
			untracked = append(untracked, "\t"+path)
		}
	}

	if len(staged) > 0 {
		fmt.Fprintln(w, "Changes to be committed:")
		for _, l := range staged {
			fmt.Fprintln(w, l)
		}
		fmt.Fprintln(w, "")
	}
	if len(changed) > 0 {
		fmt.Fprintln(w, "Changes not staged for commit:")
		for _, l := range changed {
			fmt.Fprintln(w, l)
		}
		fmt.Fprintln(w, "")
	}
	if len(untracked) > 0 {
		fmt.Fprintln(w, "Untracked files:")
		for _, l := range untracked {
			fmt.Fprintln(w, l)
		}
		fmt.Fprintln(w, "")
	}
	return nil
}

func stagingLabel(c gogit.StatusCode) string {
	switch c {
	case gogit.Added:
		return "new file"
	case gogit.Modified:
		return "modified"
	case gogit.Deleted:
		return "deleted"
	case gogit.Renamed:
		return "renamed"
	case gogit.Copied:
		return "copied"
	default:
		return string(c)
	}
}

func worktreeLabel(c gogit.StatusCode) string {
	switch c {
	case gogit.Modified:
		return "modified"
	case gogit.Deleted:
		return "deleted"
	default:
		return string(c)
	}
}

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

// ---------------------------------------------------------------------------
// git diff
// ---------------------------------------------------------------------------

func cmdGitDiff(w io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, root, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	cached := false
	for _, a := range args {
		if a == "--cached" || a == "--staged" {
			cached = true
		}
	}

	if cached {
		// Diff HEAD tree vs index.
		head, err := repo.Head()
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			fmt.Fprintln(w, "(no commits yet)")
			return nil
		}
		if err != nil {
			return fmt.Errorf("git diff: %w", err)
		}
		commit, err := repo.CommitObject(head.Hash())
		if err != nil {
			return err
		}
		headTree, err := commit.Tree()
		if err != nil {
			return err
		}

		idx, err := repo.Storer.Index()
		if err != nil {
			return err
		}
		for _, entry := range idx.Entries {
			oldContent := ""
			if f, err := headTree.File(entry.Name); err == nil {
				if content, err := f.Contents(); err == nil {
					oldContent = content
				}
			}
			newBlob, err := repo.BlobObject(entry.Hash)
			if err != nil {
				continue
			}
			newReader, err := newBlob.Reader()
			if err != nil {
				continue
			}
			newBytes, err := io.ReadAll(newReader)
			newReader.Close()
			if err != nil {
				continue
			}
			newContent := string(newBytes)
			if oldContent != newContent {
				unifiedDiff(w, entry.Name, oldContent, newContent)
			}
		}
		return nil
	}

	// Working tree diff: compare index / HEAD vs working files.
	status, err := wt.Status()
	if err != nil {
		return fmt.Errorf("git diff: %w", err)
	}

	head, headErr := repo.Head()
	var headTree *object.Tree
	if headErr == nil {
		if commit, err := repo.CommitObject(head.Hash()); err == nil {
			headTree, _ = commit.Tree()
		}
	}

	for path, s := range status {
		if s.Worktree == gogit.Unmodified || s.Worktree == gogit.Untracked {
			continue
		}
		oldContent := ""
		if headTree != nil {
			if f, err := headTree.File(path); err == nil {
				if content, err := f.Contents(); err == nil {
					oldContent = content
				}
			}
		}
		absPath := filepath.Join(root, path)
		newBytes, err := afero.ReadFile(fs, absPath)
		if err != nil {
			continue
		}
		unifiedDiff(w, path, oldContent, string(newBytes))
	}
	return nil
}

// unifiedDiff writes a simple unified diff of oldContent vs newContent.
func unifiedDiff(w io.Writer, filename, oldContent, newContent string) {
	fmt.Fprintf(w, "diff --git a/%s b/%s\n", filename, filename)
	fmt.Fprintf(w, "--- a/%s\n", filename)
	fmt.Fprintf(w, "+++ b/%s\n", filename)

	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	// Simple all-lines diff: show all removals then all additions.
	fmt.Fprintf(w, "@@ -1,%d +1,%d @@\n", len(oldLines), len(newLines))
	for _, l := range oldLines {
		fmt.Fprintf(w, "-%s\n", l)
	}
	for _, l := range newLines {
		fmt.Fprintf(w, "+%s\n", l)
	}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// Remove trailing empty element from a trailing newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

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
	return afero.WriteFile(fs, abs, []byte(content), 0644)
}

// ---------------------------------------------------------------------------
// git reset
// ---------------------------------------------------------------------------

func cmdGitReset(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	mode := gogit.MixedReset
	var target string

	for _, a := range args {
		switch a {
		case "--soft":
			mode = gogit.SoftReset
		case "--mixed":
			mode = gogit.MixedReset
		case "--hard":
			mode = gogit.HardReset
		default:
			if !strings.HasPrefix(a, "-") {
				target = a
			}
		}
	}

	var hash plumbing.Hash
	if target == "" || target == "HEAD" {
		head, err := repo.Head()
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			fmt.Fprintln(errW, "git reset: no commits yet")
			return interp.ExitStatus(1)
		}
		if err != nil {
			return err
		}
		hash = head.Hash()
	} else {
		h, err := repo.ResolveRevision(plumbing.Revision(target))
		if err != nil {
			return fmt.Errorf("git reset: %w", err)
		}
		hash = *h
	}

	if err := wt.Reset(&gogit.ResetOptions{
		Commit: hash,
		Mode:   mode,
	}); err != nil {
		return fmt.Errorf("git reset: %w", err)
	}
	fmt.Fprintf(w, "HEAD is now at %s\n", hash.String()[:7])
	return nil
}

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

// ---------------------------------------------------------------------------
// git config
// ---------------------------------------------------------------------------

func cmdGitConfig(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	cfg, err := repo.Config()
	if err != nil {
		return fmt.Errorf("git config: %w", err)
	}

	// Minimal: git config user.name / user.email get/set
	if len(args) == 1 {
		// Get.
		switch args[0] {
		case "user.name":
			fmt.Fprintln(w, cfg.User.Name)
		case "user.email":
			fmt.Fprintln(w, cfg.User.Email)
		default:
			fmt.Fprintf(errW, "git config: key '%s' not found\n", args[0])
			return interp.ExitStatus(1)
		}
		return nil
	}
	if len(args) == 2 {
		// Set.
		switch args[0] {
		case "user.name":
			cfg.User.Name = args[1]
		case "user.email":
			cfg.User.Email = args[1]
		default:
			fmt.Fprintf(errW, "git config: unsupported key '%s'\n", args[0])
			return interp.ExitStatus(1)
		}
		return repo.Storer.SetConfig(cfg)
	}

	// List all.
	fmt.Fprintf(w, "user.name=%s\n", cfg.User.Name)
	fmt.Fprintf(w, "user.email=%s\n", cfg.User.Email)
	return nil
}
