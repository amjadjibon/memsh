package git

import (
	"os"
	"path/filepath"

	billy "github.com/go-git/go-billy/v5"
	"github.com/spf13/afero"
)

// billyFile wraps afero.File and adds the Lock/Unlock no-ops required by
// billy.File.
type billyFile struct {
	afero.File
}

func (f *billyFile) Lock() error   { return nil }
func (f *billyFile) Unlock() error { return nil }

// aferoFS adapts an afero.Fs to the billy.Filesystem interface.
// All relative paths are resolved against root.
type aferoFS struct {
	fs   afero.Fs
	root string
}

// compile-time proof that aferoFS satisfies billy.Filesystem.
var _ billy.Filesystem = (*aferoFS)(nil)

// abs resolves path relative to a.root.
// Empty path → root itself. Absolute path returned as-is.
func (a *aferoFS) abs(path string) string {
	if path == "" {
		return a.root
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(a.root, path)
}

// --------------------------------------------------------------------------
// billy.Basic
// --------------------------------------------------------------------------

func (a *aferoFS) Create(filename string) (billy.File, error) {
	f, err := a.fs.Create(a.abs(filename))
	if err != nil {
		return nil, err
	}
	return &billyFile{f}, nil
}

func (a *aferoFS) Open(filename string) (billy.File, error) {
	f, err := a.fs.Open(a.abs(filename))
	if err != nil {
		return nil, err
	}
	return &billyFile{f}, nil
}

func (a *aferoFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	f, err := a.fs.OpenFile(a.abs(filename), flag, perm)
	if err != nil {
		return nil, err
	}
	return &billyFile{f}, nil
}

func (a *aferoFS) Stat(filename string) (os.FileInfo, error) {
	return a.fs.Stat(a.abs(filename))
}

func (a *aferoFS) Rename(from, to string) error {
	return a.fs.Rename(a.abs(from), a.abs(to))
}

func (a *aferoFS) Remove(filename string) error {
	return a.fs.Remove(a.abs(filename))
}

func (a *aferoFS) Join(elem ...string) string {
	return filepath.Join(elem...)
}

// --------------------------------------------------------------------------
// billy.Dir
// --------------------------------------------------------------------------

func (a *aferoFS) ReadDir(path string) ([]os.FileInfo, error) {
	return afero.ReadDir(a.fs, a.abs(path))
}

func (a *aferoFS) MkdirAll(path string, perm os.FileMode) error {
	return a.fs.MkdirAll(a.abs(path), perm)
}

// --------------------------------------------------------------------------
// billy.Symlink — afero.MemMapFs has no real symlink support; we stub it out
// so go-git does not encounter fatal errors when it optionally tries to read
// symlinks.
// --------------------------------------------------------------------------

func (a *aferoFS) Lstat(filename string) (os.FileInfo, error) {
	// MemMapFs has no symlinks, so Lstat == Stat.
	return a.fs.Stat(a.abs(filename))
}

func (a *aferoFS) Symlink(target, link string) error {
	// No-op: afero.MemMapFs does not support real symlinks.
	return nil
}

func (a *aferoFS) Readlink(link string) (string, error) {
	// Identity: without real symlinks every path resolves to itself.
	return link, nil
}

// --------------------------------------------------------------------------
// billy.TempFile
// --------------------------------------------------------------------------

func (a *aferoFS) TempFile(dir, prefix string) (billy.File, error) {
	f, err := afero.TempFile(a.fs, a.abs(dir), prefix)
	if err != nil {
		return nil, err
	}
	return &billyFile{f}, nil
}

// --------------------------------------------------------------------------
// billy.Chroot
// --------------------------------------------------------------------------

func (a *aferoFS) Chroot(path string) (billy.Filesystem, error) {
	abs := a.abs(path)
	if err := a.fs.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &aferoFS{fs: a.fs, root: abs}, nil
}

func (a *aferoFS) Root() string {
	return a.root
}
