package shell

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// openHandler intercepts file opens (like redirects) to route them to MemMapFs.
func (s *Shell) openHandler(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
	// Intercept URL reads before path resolution. filepath.Join/Clean would
	// mangle "http://host/path" into "/http:/host/path", so we must check first.
	if strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://") {
		if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0 {
			return nil, errors.New("cannot open URL for writing: " + path)
		}
		data, err := sourceURL(ctx, path, s.networkDialer)
		if err != nil {
			return nil, err
		}
		return &urlReadCloser{bytes.NewReader(data)}, nil
	}

	absPath := s.resolvePath(path)
	f, err := s.fs.OpenFile(absPath, flag, perm)
	return f, err
}

// urlReadCloser wraps a bytes.Reader as an io.ReadWriteCloser.
// Writes are rejected; the URL content was fetched at open time.
type urlReadCloser struct{ *bytes.Reader }

func (urlReadCloser) Write(_ []byte) (int, error) { return 0, errors.New("read-only: URL source") }
func (urlReadCloser) Close() error                { return nil }

// resolvePath resolves a path relative to the current working directory.
func (s *Shell) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.cwd, path)
}
