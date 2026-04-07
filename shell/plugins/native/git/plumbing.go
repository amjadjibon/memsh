package git

import (
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// openRepo is defined in git.go — used throughout this file.

// ---------------------------------------------------------------------------
// git cat-file
// ---------------------------------------------------------------------------

func cmdGitCatFile(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	repo, _, err := openRepo(fs, cwd)
	if err != nil {
		return err
	}

	printType := false
	printSize := false
	prettyPrint := false
	existCheck := false
	var objRef string

	for _, a := range args {
		switch a {
		case "-t":
			printType = true
		case "-s":
			printSize = true
		case "-p":
			prettyPrint = true
		case "-e":
			existCheck = true
		default:
			if !strings.HasPrefix(a, "-") {
				objRef = a
			}
		}
	}

	if objRef == "" {
		fmt.Fprintln(errW, "git cat-file: no object specified")
		return interp.ExitStatus(1)
	}

	// Resolve object hash.
	var hash plumbing.Hash
	h, err := repo.ResolveRevision(plumbing.Revision(objRef))
	if err != nil {
		// Try as a raw hash.
		hash = plumbing.NewHash(objRef)
		if hash.IsZero() {
			if existCheck {
				return interp.ExitStatus(1)
			}
			fmt.Fprintf(errW, "git cat-file: not a valid object name '%s'\n", objRef)
			return interp.ExitStatus(128)
		}
	} else {
		hash = *h
	}

	// Try object types in order: commit, tree, blob, tag.
	if commit, err := repo.CommitObject(hash); err == nil {
		if existCheck {
			return nil
		}
		if printType {
			fmt.Fprintln(w, "commit")
			return nil
		}
		if printSize {
			fmt.Fprintf(w, "%d\n", len(commit.Message))
			return nil
		}
		if prettyPrint {
			fmt.Fprintf(w, "tree %s\n", commit.TreeHash.String())
			for _, p := range commit.ParentHashes {
				fmt.Fprintf(w, "parent %s\n", p.String())
			}
			fmt.Fprintf(w, "author %s <%s> %d +0000\n", commit.Author.Name, commit.Author.Email, commit.Author.When.Unix())
			fmt.Fprintf(w, "committer %s <%s> %d +0000\n", commit.Committer.Name, commit.Committer.Email, commit.Committer.When.Unix())
			fmt.Fprintln(w)
			fmt.Fprintln(w, commit.Message)
			return nil
		}
		fmt.Fprintln(w, commit.Message)
		return nil
	}

	if tree, err := repo.TreeObject(hash); err == nil {
		if existCheck {
			return nil
		}
		if printType {
			fmt.Fprintln(w, "tree")
			return nil
		}
		if printSize || prettyPrint {
			for _, entry := range tree.Entries {
				objType := "blob"
				if !entry.Mode.IsFile() {
					objType = "tree"
				}
				fmt.Fprintf(w, "%06o %s %s\t%s\n", entry.Mode, objType, entry.Hash.String(), entry.Name)
			}
			return nil
		}
		for _, entry := range tree.Entries {
			fmt.Fprintf(w, "%s\n", entry.Name)
		}
		return nil
	}

	if blob, err := repo.BlobObject(hash); err == nil {
		if existCheck {
			return nil
		}
		if printType {
			fmt.Fprintln(w, "blob")
			return nil
		}
		if printSize {
			fmt.Fprintf(w, "%d\n", blob.Size)
			return nil
		}
		reader, err := blob.Reader()
		if err != nil {
			return fmt.Errorf("git cat-file: %w", err)
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("git cat-file: %w", err)
		}
		fmt.Fprint(w, string(data))
		return nil
	}

	if tag, err := repo.TagObject(hash); err == nil {
		if existCheck {
			return nil
		}
		if printType {
			fmt.Fprintln(w, "tag")
			return nil
		}
		if printSize || prettyPrint {
			fmt.Fprintf(w, "object %s\n", tag.Target.String())
			fmt.Fprintf(w, "type %s\n", tag.TargetType.String())
			fmt.Fprintf(w, "tag %s\n", tag.Name)
			fmt.Fprintf(w, "tagger %s <%s>\n", tag.Tagger.Name, tag.Tagger.Email)
			fmt.Fprintln(w)
			fmt.Fprintln(w, tag.Message)
			return nil
		}
		fmt.Fprintln(w, tag.Message)
		return nil
	}

	if existCheck {
		return interp.ExitStatus(1)
	}
	fmt.Fprintf(errW, "git cat-file: object '%s' not found\n", objRef)
	return interp.ExitStatus(128)
}

// ---------------------------------------------------------------------------
// git hash-object
// ---------------------------------------------------------------------------

func cmdGitHashObject(w io.Writer, errW io.Writer, fs afero.Fs, cwd string, args []string) error {
	writeObj := false
	objType := "blob"
	fromStdin := false
	var filePath string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-w":
			writeObj = true
		case "-t":
			i++
			if i < len(args) {
				objType = args[i]
			}
		case "--stdin":
			fromStdin = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				filePath = args[i]
			}
		}
	}

	var content []byte
	var err error

	if fromStdin || filePath == "" {
		// Read from stdin is not easily available at this layer;
		// if no file given and no --stdin, report error.
		if !fromStdin {
			fmt.Fprintln(errW, "git hash-object: no file specified")
			return interp.ExitStatus(1)
		}
		// Indicate stdin not supported at this level.
		fmt.Fprintln(errW, "git hash-object: --stdin requires interactive input (not supported here)")
		return interp.ExitStatus(1)
	} else {
		absPath := filePath
		if !strings.HasPrefix(absPath, "/") {
			absPath = cwd + "/" + filePath
		}
		content, err = afero.ReadFile(fs, absPath)
		if err != nil {
			fmt.Fprintf(errW, "git hash-object: cannot read '%s': %v\n", filePath, err)
			return interp.ExitStatus(1)
		}
	}

	var pType plumbing.ObjectType
	switch objType {
	case "blob":
		pType = plumbing.BlobObject
	case "commit":
		pType = plumbing.CommitObject
	case "tree":
		pType = plumbing.TreeObject
	case "tag":
		pType = plumbing.TagObject
	default:
		fmt.Fprintf(errW, "git hash-object: invalid type '%s'\n", objType)
		return interp.ExitStatus(1)
	}

	hash := plumbing.ComputeHash(pType, content)

	if writeObj {
		repo, _, err := openRepo(fs, cwd)
		if err != nil {
			return fmt.Errorf("git hash-object: %w", err)
		}
		obj := repo.Storer.NewEncodedObject()
		obj.SetType(pType)
		obj.SetSize(int64(len(content)))
		writer, err := obj.Writer()
		if err != nil {
			return fmt.Errorf("git hash-object: %w", err)
		}
		if _, err := writer.Write(content); err != nil {
			writer.Close()
			return fmt.Errorf("git hash-object: %w", err)
		}
		writer.Close()
		if _, err := repo.Storer.SetEncodedObject(obj); err != nil {
			return fmt.Errorf("git hash-object: %w", err)
		}
	}

	fmt.Fprintln(w, hash.String())
	_ = errW
	return nil
}

