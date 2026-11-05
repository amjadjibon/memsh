package session

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/spf13/afero"
)

// ErrQuotaExceeded is returned by a QuotaFs write that would take the
// filesystem past its byte or file limit.
var ErrQuotaExceeded = errors.New("session filesystem quota exceeded")

// QuotaFs wraps an afero.Fs and enforces Limits.MaxBytes / Limits.MaxFiles
// as data is written, instead of after a command has finished. Usage is
// measured once when the wrapper is created and then tracked incrementally.
//
// It accounts for regular-file sizes only (directories are free), matching
// Limits.ValidateFS.
type QuotaFs struct {
	afero.Fs
	maxBytes int64
	maxFiles int

	mu    sync.Mutex
	bytes int64
	files int
}

// WrapFS returns fs wrapped in a QuotaFs when l has filesystem limits, and fs
// unchanged otherwise.
func (l Limits) WrapFS(fs afero.Fs) (afero.Fs, error) {
	if !l.HasFSLimits() {
		return fs, nil
	}
	files, bytes, err := fsUsage(fs)
	if err != nil {
		return nil, err
	}
	return &QuotaFs{Fs: fs, maxBytes: l.MaxBytes, maxFiles: l.MaxFiles, bytes: bytes, files: files}, nil
}

// Usage reports the tracked file count and byte total.
func (q *QuotaFs) Usage() (files int, bytes int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.files, q.bytes
}

func (q *QuotaFs) Name() string { return "QuotaFs(" + q.Fs.Name() + ")" }

// reserve accounts for a change of dBytes bytes and dFiles files, failing
// (and changing nothing) if a growth would exceed a limit.
func (q *QuotaFs) reserve(dBytes int64, dFiles int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if dBytes > 0 && q.maxBytes > 0 && q.bytes+dBytes > q.maxBytes {
		return fmt.Errorf("%w: %d bytes (max %d)", ErrQuotaExceeded, q.bytes+dBytes, q.maxBytes)
	}
	if dFiles > 0 && q.maxFiles > 0 && q.files+dFiles > q.maxFiles {
		return fmt.Errorf("%w: %d files (max %d)", ErrQuotaExceeded, q.files+dFiles, q.maxFiles)
	}
	q.bytes += dBytes
	q.files += dFiles
	return nil
}

// fileSize returns the size of a regular file at name and whether it exists
// as a regular file.
func (q *QuotaFs) fileSize(name string) (int64, bool) {
	info, err := q.Fs.Stat(name)
	if err != nil || info.IsDir() {
		return 0, false
	}
	return info.Size(), true
}

func (q *QuotaFs) Create(name string) (afero.File, error) {
	return q.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

func (q *QuotaFs) Open(name string) (afero.File, error) {
	return q.OpenFile(name, os.O_RDONLY, 0)
}

func (q *QuotaFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) == 0 {
		return q.Fs.OpenFile(name, flag, perm)
	}

	size, exists := q.fileSize(name)
	if !exists && flag&os.O_CREATE != 0 {
		if err := q.reserve(0, 1); err != nil {
			return nil, &os.PathError{Op: "open", Path: name, Err: err}
		}
	}
	f, err := q.Fs.OpenFile(name, flag, perm)
	if err != nil {
		if !exists && flag&os.O_CREATE != 0 {
			_ = q.reserve(0, -1)
		}
		return nil, err
	}
	if exists && flag&os.O_TRUNC != 0 && size > 0 {
		_ = q.reserve(-size, 0)
	}
	info, statErr := f.Stat()
	if statErr == nil && info.IsDir() {
		return f, nil
	}
	return &quotaFile{File: f, q: q, appendMode: flag&os.O_APPEND != 0}, nil
}

func (q *QuotaFs) Remove(name string) error {
	size, exists := q.fileSize(name)
	if err := q.Fs.Remove(name); err != nil {
		return err
	}
	if exists {
		_ = q.reserve(-size, -1)
	}
	return nil
}

func (q *QuotaFs) RemoveAll(path string) error {
	var files int
	var bytes int64
	if info, err := q.Fs.Stat(path); err == nil {
		if info.IsDir() {
			_ = afero.Walk(q.Fs, path, func(_ string, fi os.FileInfo, err error) error {
				if err == nil && !fi.IsDir() {
					files++
					bytes += fi.Size()
				}
				return nil
			})
		} else {
			files, bytes = 1, info.Size()
		}
	}
	if err := q.Fs.RemoveAll(path); err != nil {
		return err
	}
	_ = q.reserve(-bytes, -files)
	return nil
}

func (q *QuotaFs) Rename(oldname, newname string) error {
	dstSize, dstExists := q.fileSize(newname)
	if err := q.Fs.Rename(oldname, newname); err != nil {
		return err
	}
	if dstExists {
		_ = q.reserve(-dstSize, -1)
	}
	return nil
}

func (q *QuotaFs) Chtimes(name string, atime, mtime time.Time) error {
	return q.Fs.Chtimes(name, atime, mtime)
}

// quotaFile charges writes that extend the file against the QuotaFs.
type quotaFile struct {
	afero.File
	q          *QuotaFs
	appendMode bool
	mu         sync.Mutex
}

// growth returns how many bytes writing n bytes at off would add.
func (f *quotaFile) growth(off int64, n int) (int64, error) {
	info, err := f.File.Stat()
	if err != nil {
		return 0, err
	}
	if end := off + int64(n); end > info.Size() {
		return end - info.Size(), nil
	}
	return 0, nil
}

func (f *quotaFile) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var off int64
	if f.appendMode {
		info, err := f.File.Stat()
		if err != nil {
			return 0, err
		}
		off = info.Size()
	} else {
		var err error
		if off, err = f.File.Seek(0, io.SeekCurrent); err != nil {
			return 0, err
		}
	}
	g, err := f.growth(off, len(p))
	if err != nil {
		return 0, err
	}
	if err := f.q.reserve(g, 0); err != nil {
		return 0, &os.PathError{Op: "write", Path: f.Name(), Err: err}
	}
	n, werr := f.File.Write(p)
	if n < len(p) {
		// Refund the part that was reserved but not written.
		if g2, err := f.growth(off, n); err == nil {
			_ = f.q.reserve(g2-g, 0)
		}
	}
	return n, werr
}

func (f *quotaFile) WriteAt(p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, err := f.growth(off, len(p))
	if err != nil {
		return 0, err
	}
	if err := f.q.reserve(g, 0); err != nil {
		return 0, &os.PathError{Op: "write", Path: f.Name(), Err: err}
	}
	n, werr := f.File.WriteAt(p, off)
	if n < len(p) {
		if g2, err := f.growth(off, n); err == nil {
			_ = f.q.reserve(g2-g, 0)
		}
	}
	return n, werr
}

func (f *quotaFile) WriteString(s string) (int, error) {
	return f.Write([]byte(s))
}

func (f *quotaFile) Truncate(size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	info, err := f.File.Stat()
	if err != nil {
		return err
	}
	delta := size - info.Size()
	if err := f.q.reserve(delta, 0); err != nil {
		return &os.PathError{Op: "truncate", Path: f.Name(), Err: err}
	}
	if err := f.File.Truncate(size); err != nil {
		_ = f.q.reserve(-delta, 0)
		return err
	}
	return nil
}
