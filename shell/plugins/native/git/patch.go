package git

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// ---------------------------------------------------------------------------
// git cherry-pick
// ---------------------------------------------------------------------------

func cmdGitCherryPick(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, root, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		fmt.Fprintln(errW, "git cherry-pick: no commit specified")
		return interp.ExitStatus(1)
	}

	h, err := repo.ResolveRevision(plumbing.Revision(args[0]))
	if err != nil {
		hash := plumbing.NewHash(args[0])
		h = &hash
	}

	pickCommit, err := repo.CommitObject(*h)
	if err != nil {
		return fmt.Errorf("git cherry-pick: %w", err)
	}

	pickTree, err := pickCommit.Tree()
	if err != nil {
		return fmt.Errorf("git cherry-pick: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	if pickCommit.NumParents() == 0 {
		// Root commit: apply all files from tree.
		if err := pickTree.Files().ForEach(func(f *object.File) error {
			content, err := f.Contents()
			if err != nil {
				return err
			}
			absPath := filepath.Join(root, f.Name)
			if mkErr := fs.MkdirAll(filepath.Dir(absPath), 0755); mkErr != nil {
				return mkErr
			}
			if err := afero.WriteFile(fs, absPath, []byte(content), 0644); err != nil {
				return err
			}
			rel, relErr := relToRoot(root, absPath)
			if relErr != nil {
				return relErr
			}
			_, addErr := wt.Add(rel)
			return addErr
		}); err != nil {
			return fmt.Errorf("git cherry-pick: %w", err)
		}
	} else {
		parent, err := pickCommit.Parents().Next()
		if err != nil {
			return fmt.Errorf("git cherry-pick: %w", err)
		}
		parentTree, err := parent.Tree()
		if err != nil {
			return fmt.Errorf("git cherry-pick: %w", err)
		}

		changes, err := parentTree.Diff(pickTree)
		if err != nil {
			return fmt.Errorf("git cherry-pick: %w", err)
		}

		for _, change := range changes {
			if err := applyChange(fs, repo, wt, root, change, false); err != nil {
				return fmt.Errorf("git cherry-pick: %w", err)
			}
		}
	}

	// Commit with pick commit's message and author; committer = default.
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("git cherry-pick: %w", err)
	}
	branch := "HEAD"
	if head.Name().IsBranch() {
		branch = head.Name().Short()
	}

	committer := defaultSignature("", "")
	newHash, err := wt.Commit(pickCommit.Message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  pickCommit.Author.Name,
			Email: pickCommit.Author.Email,
			When:  pickCommit.Author.When,
		},
		Committer: committer,
	})
	if err != nil {
		return fmt.Errorf("git cherry-pick: %w", err)
	}

	fmt.Fprintf(w, "[%s %s] %s\n", branch, newHash.String()[:7], firstLine(pickCommit.Message))
	_ = errW
	return nil
}

// applyChange applies a single tree change to the worktree.
// If reversed is true, the direction is inverted (for revert).
func applyChange(fs afero.Fs, repo *gogit.Repository, wt *gogit.Worktree, root string, change *object.Change, reversed bool) error {
	fromEntry := change.From
	toEntry := change.To
	if reversed {
		fromEntry, toEntry = toEntry, fromEntry
	}

	if toEntry.Name == "" {
		// Delete
		absPath := filepath.Join(root, fromEntry.Name)
		_ = fs.Remove(absPath)
		return nil
	}

	// Insert or modify: write from toEntry blob.
	blob, err := repo.BlobObject(toEntry.TreeEntry.Hash)
	if err != nil {
		return err
	}
	reader, err := blob.Reader()
	if err != nil {
		return err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	absPath := filepath.Join(root, toEntry.Name)
	if mkErr := fs.MkdirAll(filepath.Dir(absPath), 0755); mkErr != nil {
		return mkErr
	}
	if err := afero.WriteFile(fs, absPath, data, 0644); err != nil {
		return err
	}
	rel, err := relToRoot(root, absPath)
	if err != nil {
		return err
	}
	_, err = wt.Add(rel)
	return err
}

// ---------------------------------------------------------------------------
// git revert
// ---------------------------------------------------------------------------

func cmdGitRevert(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, root, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		fmt.Fprintln(errW, "git revert: no commit specified")
		return interp.ExitStatus(1)
	}

	h, err := repo.ResolveRevision(plumbing.Revision(args[0]))
	if err != nil {
		hash := plumbing.NewHash(args[0])
		h = &hash
	}

	pickCommit, err := repo.CommitObject(*h)
	if err != nil {
		return fmt.Errorf("git revert: %w", err)
	}

	pickTree, err := pickCommit.Tree()
	if err != nil {
		return fmt.Errorf("git revert: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	if pickCommit.NumParents() == 0 {
		// Root commit revert: remove all files.
		if err := pickTree.Files().ForEach(func(f *object.File) error {
			return fs.Remove(filepath.Join(root, f.Name))
		}); err != nil {
			return fmt.Errorf("git revert: %w", err)
		}
	} else {
		parent, err := pickCommit.Parents().Next()
		if err != nil {
			return fmt.Errorf("git revert: %w", err)
		}
		parentTree, err := parent.Tree()
		if err != nil {
			return fmt.Errorf("git revert: %w", err)
		}

		// Reverse diff: from pick to parent.
		changes, err := pickTree.Diff(parentTree)
		if err != nil {
			return fmt.Errorf("git revert: %w", err)
		}

		for _, change := range changes {
			if err := applyChange(fs, repo, wt, root, change, false); err != nil {
				return fmt.Errorf("git revert: %w", err)
			}
		}
	}

	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("git revert: %w", err)
	}
	branch := "HEAD"
	if head.Name().IsBranch() {
		branch = head.Name().Short()
	}

	subject := firstLine(pickCommit.Message)
	msg := fmt.Sprintf("Revert \"%s\"\n\nThis reverts commit %s.\n", subject, pickCommit.Hash.String())

	committer := defaultSignature("", "")
	newHash, err := wt.Commit(msg, &gogit.CommitOptions{
		Author:    committer,
		Committer: committer,
	})
	if err != nil {
		return fmt.Errorf("git revert: %w", err)
	}

	fmt.Fprintf(w, "[%s %s] %s\n", branch, newHash.String()[:7], firstLine(msg))
	_ = errW
	return nil
}

// ---------------------------------------------------------------------------
// git format-patch
// ---------------------------------------------------------------------------

func cmdGitFormatPatch(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	var commits []*object.Commit
	nFlag := -1

	for _, a := range args {
		// Handle -N form (e.g. -3 for last 3 commits)
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			if n, err := strconv.Atoi(a[1:]); err == nil {
				nFlag = n
				continue
			}
		}

		// Handle since..until range
		if strings.Contains(a, "..") {
			parts := strings.SplitN(a, "..", 2)
			since, until := parts[0], parts[1]
			if until == "" {
				until = "HEAD"
			}
			untilHash, err := repo.ResolveRevision(plumbing.Revision(until))
			if err != nil {
				fmt.Fprintf(errW, "git format-patch: invalid revision '%s'\n", until)
				return interp.ExitStatus(1)
			}
			var sinceHash *plumbing.Hash
			if since != "" {
				h, err := repo.ResolveRevision(plumbing.Revision(since))
				if err != nil {
					fmt.Fprintf(errW, "git format-patch: invalid revision '%s'\n", since)
					return interp.ExitStatus(1)
				}
				sinceHash = h
			}

			iter, err := repo.Log(&gogit.LogOptions{From: *untilHash})
			if err != nil {
				return fmt.Errorf("git format-patch: %w", err)
			}
			var collected []*object.Commit
			_ = iter.ForEach(func(c *object.Commit) error {
				if sinceHash != nil && c.Hash == *sinceHash {
					return fmt.Errorf("stop")
				}
				collected = append(collected, c)
				return nil
			})
			iter.Close()
			// Reverse so oldest first.
			for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
				collected[i], collected[j] = collected[j], collected[i]
			}
			commits = collected
			continue
		}
	}

	// If -N flag, get last N commits from HEAD.
	if nFlag > 0 && len(commits) == 0 {
		head, err := repo.Head()
		if err != nil {
			return fmt.Errorf("git format-patch: %w", err)
		}
		iter, err := repo.Log(&gogit.LogOptions{From: head.Hash()})
		if err != nil {
			return fmt.Errorf("git format-patch: %w", err)
		}
		var collected []*object.Commit
		_ = iter.ForEach(func(c *object.Commit) error {
			if len(collected) >= nFlag {
				return fmt.Errorf("stop")
			}
			collected = append(collected, c)
			return nil
		})
		iter.Close()
		// Reverse so oldest first.
		for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
			collected[i], collected[j] = collected[j], collected[i]
		}
		commits = collected
	}

	total := len(commits)
	for n, c := range commits {
		// Build patch content.
		var buf bytes.Buffer
		dateStr := c.Author.When.Format(time.RFC1123Z)
		fmt.Fprintf(&buf, "From %s %s\n", c.Hash.String(), dateStr)
		fmt.Fprintf(&buf, "From: %s <%s>\n", c.Author.Name, c.Author.Email)
		fmt.Fprintf(&buf, "Date: %s\n", dateStr)
		fmt.Fprintf(&buf, "Subject: [PATCH %d/%d] %s\n", n+1, total, firstLine(c.Message))
		fmt.Fprintln(&buf)
		// Body (everything after first line).
		body := strings.TrimPrefix(c.Message, firstLine(c.Message))
		body = strings.TrimPrefix(body, "\n")
		if body != "" {
			fmt.Fprintln(&buf, body)
		}
		fmt.Fprintln(&buf, "---")

		// Unified diff vs parent.
		if c.NumParents() > 0 {
			parent, err := c.Parents().Next()
			if err == nil {
				patch, err := parent.Patch(c)
				if err == nil {
					fmt.Fprintln(&buf, patch.String())
				}
			}
		}

		// Write patch file.
		filename := fmt.Sprintf("%04d-%s.patch", n+1, sanitizeFilename(firstLine(c.Message)))
		absPath := filepath.Join(cwd, filename)
		if err := afero.WriteFile(fs, absPath, buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("git format-patch: %w", err)
		}
		fmt.Fprintln(w, filename)
	}

	_ = errW
	return nil
}

func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == ' ' || r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			b.WriteByte('-')
		} else {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if len(result) > 52 {
		result = result[:52]
	}
	return result
}

// ---------------------------------------------------------------------------
// git apply
// ---------------------------------------------------------------------------

func cmdGitApply(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, root, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	var patchData []byte
	var patchFile string

	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			patchFile = a
		}
	}

	if patchFile != "" {
		abs := patchFile
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, patchFile)
		}
		patchData, err = afero.ReadFile(fs, abs)
		if err != nil {
			fmt.Fprintf(errW, "git apply: cannot read patch file '%s': %v\n", patchFile, err)
			return interp.ExitStatus(1)
		}
	} else {
		// No file arg — nothing to do without stdin support at this layer.
		fmt.Fprintln(errW, "git apply: no patch file specified")
		return interp.ExitStatus(1)
	}

	// Parse and apply the patch.
	modifiedFiles, err := applyPatch(fs, root, patchData)
	if err != nil {
		fmt.Fprintf(errW, "git apply: %v\n", err)
		return interp.ExitStatus(1)
	}

	for _, f := range modifiedFiles {
		rel, _ := relToRoot(root, f)
		if _, err := wt.Add(rel); err != nil {
			// Non-fatal: file may not be tracked yet.
			_ = err
		}
	}

	_ = w
	return nil
}

// applyPatch parses a unified diff patch and applies it to fs under root.
// Returns the list of absolute paths modified.
func applyPatch(fs afero.Fs, root string, patchData []byte) ([]string, error) {
	var modifiedFiles []string

	type hunk struct {
		fromLine int
		lines    []string // '+' or '-' or ' ' prefixed
	}

	type filePatch struct {
		path  string
		hunks []hunk
	}

	var patches []filePatch
	var current *filePatch
	var currentHunk *hunk

	scanner := bufio.NewScanner(bytes.NewReader(patchData))
	for scanner.Scan() {
		line := scanner.Text()

		if path, ok := strings.CutPrefix(line, "+++ b/"); ok {
			patches = append(patches, filePatch{path: path})
			current = &patches[len(patches)-1]
			currentHunk = nil
			continue
		}
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "From ") || strings.HasPrefix(line, "From:") || strings.HasPrefix(line, "Date:") || strings.HasPrefix(line, "Subject:") {
			currentHunk = nil
			continue
		}
		if strings.HasPrefix(line, "@@ ") {
			if current == nil {
				continue
			}
			// Parse @@ -from,count +to,count @@
			var fromStart int
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				fromPart := strings.TrimPrefix(parts[1], "-")
				fromParts := strings.SplitN(fromPart, ",", 2)
				fromStart, _ = strconv.Atoi(fromParts[0])
			}
			current.hunks = append(current.hunks, hunk{fromLine: fromStart})
			currentHunk = &current.hunks[len(current.hunks)-1]
			continue
		}
		if currentHunk != nil && (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, " ")) {
			currentHunk.lines = append(currentHunk.lines, line)
		}
	}

	for _, fp := range patches {
		absPath := filepath.Join(root, fp.path)
		var existingLines []string
		if data, err := afero.ReadFile(fs, absPath); err == nil {
			existingLines = strings.Split(string(data), "\n")
			// Remove trailing empty from final newline.
			if len(existingLines) > 0 && existingLines[len(existingLines)-1] == "" {
				existingLines = existingLines[:len(existingLines)-1]
			}
		}

		for _, h := range fp.hunks {
			var result []string
			lineIdx := 0
			fromLine := h.fromLine
			if fromLine > 0 {
				fromLine-- // Convert to 0-based.
			}
			// Copy lines before the hunk.
			for lineIdx < fromLine && lineIdx < len(existingLines) {
				result = append(result, existingLines[lineIdx])
				lineIdx++
			}
			// Apply hunk lines.
			for _, hunkLine := range h.lines {
				if strings.HasPrefix(hunkLine, "+") {
					result = append(result, hunkLine[1:])
				} else if strings.HasPrefix(hunkLine, " ") {
					if lineIdx < len(existingLines) {
						result = append(result, existingLines[lineIdx])
					}
					lineIdx++
				} else if strings.HasPrefix(hunkLine, "-") {
					lineIdx++
				}
			}
			// Copy remaining lines.
			for lineIdx < len(existingLines) {
				result = append(result, existingLines[lineIdx])
				lineIdx++
			}
			existingLines = result
		}

		newContent := strings.Join(existingLines, "\n") + "\n"
		if err := fs.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return nil, err
		}
		if err := afero.WriteFile(fs, absPath, []byte(newContent), 0644); err != nil {
			return nil, err
		}
		modifiedFiles = append(modifiedFiles, absPath)
	}

	return modifiedFiles, nil
}
