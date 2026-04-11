package git

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
	"github.com/spf13/afero"
)

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
	if err := fs.MkdirAll(target, 0o755); err != nil {
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
