package git

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// ---------------------------------------------------------------------------
// git blame
// ---------------------------------------------------------------------------

func cmdGitBlame(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, root, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	var filePath string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			filePath = a
		}
	}
	if filePath == "" {
		fmt.Fprintln(errW, "git blame: no file specified")
		return interp.ExitStatus(1)
	}

	relPath, err := relToRoot(root, func() string {
		if strings.HasPrefix(filePath, "/") {
			return filePath
		}
		return cwd + "/" + filePath
	}())
	if err != nil {
		return fmt.Errorf("git blame: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("git blame: %w", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return fmt.Errorf("git blame: %w", err)
	}

	result, err := gogit.Blame(commit, relPath)
	if err != nil {
		return fmt.Errorf("git blame: %w", err)
	}

	for i, line := range result.Lines {
		fmt.Fprintf(w, "%s (%-20s %s %4d) %s\n",
			line.Hash.String()[:8],
			line.AuthorName,
			line.Date.Format("2006-01-02"),
			i+1,
			line.Text,
		)
	}
	return nil
}

// ---------------------------------------------------------------------------
// git ls-files
// ---------------------------------------------------------------------------

func cmdGitLsFiles(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	showCached := true
	showOthers := false
	showModified := false

	for _, a := range args {
		switch a {
		case "-c", "--cached":
			showCached = true
		case "-o", "--others":
			showOthers = true
			showCached = false
		case "-m", "--modified":
			showModified = true
			showCached = false
		}
	}

	if showCached {
		idx, err := repo.Storer.Index()
		if err != nil {
			return fmt.Errorf("git ls-files: %w", err)
		}
		for _, entry := range idx.Entries {
			fmt.Fprintln(w, entry.Name)
		}
		return nil
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	status, err := wt.Status()
	if err != nil {
		return fmt.Errorf("git ls-files: %w", err)
	}

	for path, s := range status {
		if showOthers && s.Worktree == gogit.Untracked {
			fmt.Fprintln(w, path)
		}
		if showModified && (s.Worktree == gogit.Modified || s.Staging == gogit.Modified) {
			fmt.Fprintln(w, path)
		}
	}
	_ = errW
	return nil
}

// ---------------------------------------------------------------------------
// git ls-tree
// ---------------------------------------------------------------------------

func cmdGitLsTree(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	recursive := false
	longFormat := false
	var treeIsh string
	var pathFilter string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-r":
			recursive = true
		case "-l", "--long":
			longFormat = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				if treeIsh == "" {
					treeIsh = args[i]
				} else {
					pathFilter = args[i]
				}
			}
		}
	}

	if treeIsh == "" {
		treeIsh = "HEAD"
	}

	h, err := repo.ResolveRevision(plumbing.Revision(treeIsh))
	if err != nil {
		// Maybe it's a tree hash directly.
		hash := plumbing.NewHash(treeIsh)
		if hash.IsZero() {
			fmt.Fprintf(errW, "git ls-tree: not a valid object name '%s'\n", treeIsh)
			return interp.ExitStatus(128)
		}
		// Try as commit.
		commit, cerr := repo.CommitObject(hash)
		if cerr == nil {
			tree, terr := commit.Tree()
			if terr != nil {
				return fmt.Errorf("git ls-tree: %w", terr)
			}
			return lsTreePrint(w, repo, tree, pathFilter, recursive, longFormat)
		}
		// Try as tree directly.
		tree, terr := repo.TreeObject(hash)
		if terr != nil {
			fmt.Fprintf(errW, "git ls-tree: not a tree object '%s'\n", treeIsh)
			return interp.ExitStatus(128)
		}
		return lsTreePrint(w, repo, tree, pathFilter, recursive, longFormat)
	}

	commit, err := repo.CommitObject(*h)
	if err != nil {
		return fmt.Errorf("git ls-tree: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return fmt.Errorf("git ls-tree: %w", err)
	}

	// Navigate sub-path if provided.
	if pathFilter != "" {
		entry, err := tree.FindEntry(pathFilter)
		if err == nil && entry.Mode.IsFile() {
			if longFormat {
				fmt.Fprintf(w, "%06o blob %s\t%s\n", entry.Mode, entry.Hash.String(), entry.Name)
			} else {
				fmt.Fprintf(w, "%06o blob %s\t%s\n", entry.Mode, entry.Hash.String(), entry.Name)
			}
			return nil
		}
		subTree, err := tree.Tree(pathFilter)
		if err != nil {
			fmt.Fprintf(errW, "git ls-tree: path '%s' not found\n", pathFilter)
			return interp.ExitStatus(1)
		}
		tree = subTree
	}

	return lsTreePrint(w, repo, tree, "", recursive, longFormat)
}

func lsTreePrint(w io.Writer, repo *gogit.Repository, tree *object.Tree, pathFilter string, recursive, long bool) error {
	if recursive {
		return tree.Files().ForEach(func(f *object.File) error {
			if pathFilter != "" && !strings.HasPrefix(f.Name, pathFilter) {
				return nil
			}
			if long {
				fmt.Fprintf(w, "%06o blob %s %7d\t%s\n", f.Mode, f.Hash.String(), f.Size, f.Name)
			} else {
				fmt.Fprintf(w, "%06o blob %s\t%s\n", f.Mode, f.Hash.String(), f.Name)
			}
			return nil
		})
	}

	for _, entry := range tree.Entries {
		objType := "blob"
		if !entry.Mode.IsFile() {
			objType = "tree"
		}
		if long && objType == "blob" {
			blob, err := repo.BlobObject(entry.Hash)
			size := int64(0)
			if err == nil {
				size = blob.Size
			}
			fmt.Fprintf(w, "%06o %s %s %7d\t%s\n", entry.Mode, objType, entry.Hash.String(), size, entry.Name)
		} else {
			fmt.Fprintf(w, "%06o %s %s\t%s\n", entry.Mode, objType, entry.Hash.String(), entry.Name)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// git shortlog
// ---------------------------------------------------------------------------

func cmdGitShortlog(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	summary := false
	sortByCount := false
	showEmail := false

	for _, a := range args {
		switch a {
		case "-s", "--summary":
			summary = true
		case "-n", "--numbered":
			sortByCount = true
		case "-e", "--email":
			showEmail = true
		}
	}

	iter, err := repo.Log(&gogit.LogOptions{})
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil
		}
		return fmt.Errorf("git shortlog: %w", err)
	}
	defer iter.Close()

	type authorInfo struct {
		name     string
		email    string
		count    int
		messages []string
	}
	authorMap := make(map[string]*authorInfo)
	var authorOrder []string

	if err := iter.ForEach(func(c *object.Commit) error {
		key := c.Author.Name
		if showEmail {
			key = fmt.Sprintf("%s <%s>", c.Author.Name, c.Author.Email)
		}
		info, ok := authorMap[key]
		if !ok {
			info = &authorInfo{name: c.Author.Name, email: c.Author.Email}
			authorMap[key] = info
			authorOrder = append(authorOrder, key)
		}
		info.count++
		info.messages = append(info.messages, firstLine(c.Message))
		return nil
	}); err != nil {
		return fmt.Errorf("git shortlog: %w", err)
	}

	if sortByCount {
		sort.Slice(authorOrder, func(i, j int) bool {
			return authorMap[authorOrder[i]].count > authorMap[authorOrder[j]].count
		})
	}

	for _, key := range authorOrder {
		info := authorMap[key]
		if summary {
			fmt.Fprintf(w, "%6d\t%s\n", info.count, key)
		} else {
			fmt.Fprintf(w, "%s (%d):\n", key, info.count)
			for _, msg := range info.messages {
				fmt.Fprintf(w, "      %s\n", msg)
			}
			fmt.Fprintln(w)
		}
	}

	_ = errW
	return nil
}

// ---------------------------------------------------------------------------
// git describe
// ---------------------------------------------------------------------------

func cmdGitDescribe(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	// Parse optional <commit-ish> and flags.
	rev := "HEAD"
	tagsOnly := false // --tags: also consider lightweight tags (default behaviour here)
	for _, a := range args {
		switch a {
		case "--tags":
			tagsOnly = true
		case "--always", "--long", "--abbrev":
			// accepted, not yet fully implemented
		default:
			if !strings.HasPrefix(a, "-") {
				rev = a
			}
		}
	}
	_ = tagsOnly

	// Build map of commit hash → tag name.
	tagMap := make(map[plumbing.Hash]string)
	tagIter, err := repo.Tags()
	if err != nil {
		return fmt.Errorf("git describe: %w", err)
	}
	_ = tagIter.ForEach(func(ref *plumbing.Reference) error {
		tagName := ref.Name().Short()
		hash := ref.Hash()
		// Annotated tags: resolve to the commit they point at.
		if tagObj, err := repo.TagObject(hash); err == nil {
			tagMap[tagObj.Target] = tagName
		} else {
			tagMap[hash] = tagName
		}
		return nil
	})

	if len(tagMap) == 0 {
		fmt.Fprintln(errW, "fatal: No names found, cannot describe anything.")
		return interp.ExitStatus(128)
	}

	h, err := repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return fmt.Errorf("git describe: %w", err)
	}

	// Walk log from the target commit.
	iter, err := repo.Log(&gogit.LogOptions{From: *h})
	if err != nil {
		return fmt.Errorf("git describe: %w", err)
	}
	defer iter.Close()

	count := 0
	var foundTag string
	var foundHash plumbing.Hash
	_ = iter.ForEach(func(c *object.Commit) error {
		if tag, ok := tagMap[c.Hash]; ok {
			foundTag = tag
			foundHash = c.Hash
			return fmt.Errorf("stop")
		}
		count++
		return nil
	})

	if foundTag == "" {
		fmt.Fprintln(errW, "fatal: No names found, cannot describe anything.")
		return interp.ExitStatus(128)
	}

	if foundHash == *h {
		fmt.Fprintln(w, foundTag)
	} else {
		fmt.Fprintf(w, "%s-%d-g%s\n", foundTag, count, h.String()[:7])
	}
	return nil
}
