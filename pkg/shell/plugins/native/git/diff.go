package git

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/afero"
)

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
