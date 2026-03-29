package shell

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// openHandler intercepts file opens (like redirects) to route them to MemMapFs.
func (s *Shell) openHandler(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
	// Resolve path relative to cwd
	absPath := s.resolvePath(path)

	// Since afero.Fs doesn't return an io.ReadWriteCloser necessarily in all backends, 
	// afero.File implements io.ReadWriteCloser, io.Seeker, io.ReaderAt etc.
	f, err := s.fs.OpenFile(absPath, flag, perm)
	return f, err
}

// resolvePath resolves a path relative to the current working directory.
func (s *Shell) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.cwd, path)
}
