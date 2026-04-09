package git

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

const stashFile = ".git/memsh_stash"

// stashIndex parses "stash@{N}" → N. Returns 0 if arg is empty or unparseable.
func stashIndex(arg string) int {
	if arg == "" {
		return 0
	}
	s := arg
	if strings.HasPrefix(s, "stash@{") && strings.HasSuffix(s, "}") {
		s = s[len("stash@{") : len(s)-1]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// readStash reads stash entries from the stash file.
// Each entry is "<hash>\t<branch>\t<message>".
func readStash(fs afero.Fs, root string) ([]string, error) {
	p := filepath.Join(root, stashFile)
	data, err := afero.ReadFile(fs, p)
	if err != nil {
		// Not found means empty stash.
		return nil, nil
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	var out []string
	for _, l := range lines {
		if l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

// writeStash writes entries back to the stash file.
func writeStash(fs afero.Fs, root string, entries []string) error {
	p := filepath.Join(root, stashFile)
	content := strings.Join(entries, "\n")
	if content != "" {
		content += "\n"
	}
	return afero.WriteFile(fs, p, []byte(content), 0644)
}

// ---------------------------------------------------------------------------
// git stash
// ---------------------------------------------------------------------------

func cmdGitStash(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, root, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	sub := "push"
	if len(args) > 0 {
		sub = args[0]
	}

	switch sub {
	case "push", "save":
		return gitStashPush(w, errW, fs, repo, root)

	case "list":
		return gitStashList(w, fs, root)

	case "apply":
		idx := 0
		if len(args) > 1 {
			idx = stashIndex(args[1])
		}
		return gitStashApply(w, errW, fs, repo, root, idx)

	case "pop":
		idx := 0
		if len(args) > 1 {
			idx = stashIndex(args[1])
		}
		if err := gitStashApply(w, errW, fs, repo, root, idx); err != nil {
			return err
		}
		return gitStashDrop(w, fs, root, idx)

	case "drop":
		idx := 0
		if len(args) > 1 {
			idx = stashIndex(args[1])
		}
		return gitStashDrop(w, fs, root, idx)

	case "show":
		idx := 0
		if len(args) > 1 {
			idx = stashIndex(args[1])
		}
		return gitStashShow(w, errW, repo, fs, root, idx)

	case "clear":
		return writeStash(fs, root, nil)

	default:
		fmt.Fprintf(errW, "git stash: unknown subcommand '%s'\n", sub)
		return interp.ExitStatus(1)
	}
}

func gitStashPush(w io.Writer, errW io.Writer, fs afero.Fs, repo *gogit.Repository, root string) error {
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	// Check for a clean worktree.
	status, err := wt.Status()
	if err != nil {
		return fmt.Errorf("git stash: %w", err)
	}
	if status.IsClean() {
		fmt.Fprintln(w, "No local changes to save")
		return nil
	}

	// Get current HEAD for later reset.
	originalHEAD, err := repo.Head()
	if err != nil {
		return fmt.Errorf("git stash: %w", err)
	}

	branchName := "HEAD"
	if originalHEAD.Name().IsBranch() {
		branchName = originalHEAD.Name().Short()
	}

	headCommit, err := repo.CommitObject(originalHEAD.Hash())
	if err != nil {
		return fmt.Errorf("git stash: %w", err)
	}
	headSubject := firstLine(headCommit.Message)
	hash7 := originalHEAD.Hash().String()[:7]

	// Stage everything.
	if err := wt.AddGlob("."); err != nil {
		return fmt.Errorf("git stash: %w", err)
	}

	stashMsg := fmt.Sprintf("WIP on %s: %s %s", branchName, hash7, headSubject)

	sig := defaultSignature("", "")
	newHash, err := wt.Commit(stashMsg, &gogit.CommitOptions{
		Author: sig,
	})
	if err != nil {
		return fmt.Errorf("git stash: %w", err)
	}

	// Prepend to stash file.
	entries, err := readStash(fs, root)
	if err != nil {
		return err
	}
	entry := fmt.Sprintf("%s\t%s\t%s", newHash.String(), branchName, stashMsg)
	entries = append([]string{entry}, entries...)
	if err := writeStash(fs, root, entries); err != nil {
		return err
	}

	// Hard-reset to original HEAD.
	if err := wt.Reset(&gogit.ResetOptions{
		Commit: originalHEAD.Hash(),
		Mode:   gogit.HardReset,
	}); err != nil {
		return fmt.Errorf("git stash: reset failed: %w", err)
	}

	fmt.Fprintf(w, "Saved working directory and index state %s\n", stashMsg)
	_ = errW
	return nil
}

func gitStashList(w io.Writer, fs afero.Fs, root string) error {
	entries, err := readStash(fs, root)
	if err != nil {
		return err
	}
	for i, e := range entries {
		parts := strings.SplitN(e, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		branch, msg := parts[1], parts[2]
		fmt.Fprintf(w, "stash@{%d}: On %s: %s\n", i, branch, msg)
	}
	return nil
}

func gitStashApply(w io.Writer, errW io.Writer, fs afero.Fs, repo *gogit.Repository, root string, idx int) error {
	entries, err := readStash(fs, root)
	if err != nil {
		return err
	}
	if idx >= len(entries) {
		fmt.Fprintf(errW, "git stash: stash@{%d} does not exist\n", idx)
		return interp.ExitStatus(1)
	}

	parts := strings.SplitN(entries[idx], "\t", 3)
	if len(parts) < 1 {
		return fmt.Errorf("git stash: malformed stash entry")
	}
	stashHash := plumbing.NewHash(parts[0])

	commit, err := repo.CommitObject(stashHash)
	if err != nil {
		return fmt.Errorf("git stash apply: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return fmt.Errorf("git stash apply: %w", err)
	}

	// Walk tree files and restore them.
	if err := tree.Files().ForEach(func(f *object.File) error {
		content, err := f.Contents()
		if err != nil {
			return err
		}
		absPath := filepath.Join(root, f.Name)
		if mkErr := fs.MkdirAll(filepath.Dir(absPath), 0755); mkErr != nil {
			return mkErr
		}
		return afero.WriteFile(fs, absPath, []byte(content), 0644)
	}); err != nil {
		return fmt.Errorf("git stash apply: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	if err := wt.AddGlob("."); err != nil {
		return fmt.Errorf("git stash apply: %w", err)
	}

	fmt.Fprintln(w, "Applied stash.")
	_ = errW
	return nil
}

func gitStashDrop(w io.Writer, fs afero.Fs, root string, idx int) error {
	entries, err := readStash(fs, root)
	if err != nil {
		return err
	}
	if idx >= len(entries) {
		return fmt.Errorf("git stash: stash@{%d} does not exist", idx)
	}
	entries = append(entries[:idx], entries[idx+1:]...)
	if err := writeStash(fs, root, entries); err != nil {
		return err
	}
	fmt.Fprintf(w, "Dropped stash@{%d}\n", idx)
	return nil
}

func gitStashShow(w io.Writer, errW io.Writer, repo *gogit.Repository, fs afero.Fs, root string, idx int) error {
	entries, err := readStash(fs, root)
	if err != nil {
		return err
	}
	if idx >= len(entries) {
		fmt.Fprintf(errW, "git stash show: stash@{%d} does not exist\n", idx)
		return interp.ExitStatus(1)
	}

	parts := strings.SplitN(entries[idx], "\t", 3)
	stashHash := plumbing.NewHash(parts[0])

	commit, err := repo.CommitObject(stashHash)
	if err != nil {
		return fmt.Errorf("git stash show: %w", err)
	}
	stashTree, err := commit.Tree()
	if err != nil {
		return fmt.Errorf("git stash show: %w", err)
	}

	if commit.NumParents() == 0 {
		// No parent — show all files as added.
		return stashTree.Files().ForEach(func(f *object.File) error {
			content, err := f.Contents()
			if err != nil {
				return nil
			}
			unifiedDiff(w, f.Name, "", content)
			return nil
		})
	}

	parent, err := commit.Parents().Next()
	if err != nil {
		return fmt.Errorf("git stash show: %w", err)
	}
	parentTree, err := parent.Tree()
	if err != nil {
		return fmt.Errorf("git stash show: %w", err)
	}

	changes, err := parentTree.Diff(stashTree)
	if err != nil {
		return fmt.Errorf("git stash show: %w", err)
	}

	for _, change := range changes {
		fromFile, toFile, err := change.Files()
		if err != nil {
			continue
		}
		var oldContent, newContent string
		if fromFile != nil {
			oldContent, _ = fromFile.Contents()
		}
		if toFile != nil {
			newContent, _ = toFile.Contents()
		}
		name := change.To.Name
		if name == "" {
			name = change.From.Name
		}
		unifiedDiff(w, name, oldContent, newContent)
	}
	_ = root
	return nil
}
