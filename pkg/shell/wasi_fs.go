package shell

import (
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/spf13/afero"
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	wazeroSys "github.com/tetratelabs/wazero/sys"
)

// aferoSysFS adapts afero.Fs to wazero's experimentalsys.FS so that WASI
// modules can read and write files directly in the virtual MemMapFs — no
// temp-directory bridge required.
//
// Path convention: wazero passes guest-relative paths without a leading "/".
// afero.MemMapFs stores paths with a leading "/". toAferoPath adds the prefix.
type aferoSysFS struct {
	experimentalsys.UnimplementedFS
	vfs afero.Fs
}

func (a aferoSysFS) toAferoPath(path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func (a aferoSysFS) oflags(flag experimentalsys.Oflag) int {
	// Low two bits encode access mode (not bitflags).
	var osFlag int
	switch flag & 3 {
	case experimentalsys.O_RDWR:
		osFlag = os.O_RDWR
	case experimentalsys.O_WRONLY:
		osFlag = os.O_WRONLY
	default:
		osFlag = os.O_RDONLY
	}
	if flag&experimentalsys.O_APPEND != 0 {
		osFlag |= os.O_APPEND
	}
	if flag&experimentalsys.O_CREAT != 0 {
		osFlag |= os.O_CREATE
	}
	if flag&experimentalsys.O_EXCL != 0 {
		osFlag |= os.O_EXCL
	}
	if flag&experimentalsys.O_TRUNC != 0 {
		osFlag |= os.O_TRUNC
	}
	return osFlag
}

func (a aferoSysFS) OpenFile(path string, flag experimentalsys.Oflag, perm fs.FileMode) (experimentalsys.File, experimentalsys.Errno) {
	afPath := a.toAferoPath(path)

	// Check if it's a directory open.
	info, statErr := a.vfs.Stat(afPath)
	if statErr == nil && info.IsDir() {
		f, err := a.vfs.Open(afPath)
		if err != nil {
			return nil, toErrno(err)
		}
		return &aferoSysDirFile{f: f}, 0
	}

	osFlags := a.oflags(flag)
	if perm == 0 {
		perm = 0o644
	}
	f, err := a.vfs.OpenFile(afPath, osFlags, perm)
	if err != nil {
		return nil, toErrno(err)
	}
	isAppend := flag&experimentalsys.O_APPEND != 0
	return &aferoSysFile{f: f, append: isAppend}, 0
}

func (a aferoSysFS) Stat(path string) (wazeroSys.Stat_t, experimentalsys.Errno) {
	info, err := a.vfs.Stat(a.toAferoPath(path))
	if err != nil {
		return wazeroSys.Stat_t{}, toErrno(err)
	}
	return infoToStat(info), 0
}

func (a aferoSysFS) Lstat(path string) (wazeroSys.Stat_t, experimentalsys.Errno) {
	// afero.MemMapFs has no symlinks; Lstat == Stat.
	return a.Stat(path)
}

func (a aferoSysFS) Mkdir(path string, perm fs.FileMode) experimentalsys.Errno {
	err := a.vfs.MkdirAll(a.toAferoPath(path), perm)
	return toErrno(err)
}

func (a aferoSysFS) Rename(from, to string) experimentalsys.Errno {
	err := a.vfs.Rename(a.toAferoPath(from), a.toAferoPath(to))
	return toErrno(err)
}

func (a aferoSysFS) Unlink(path string) experimentalsys.Errno {
	afPath := a.toAferoPath(path)
	info, err := a.vfs.Stat(afPath)
	if err != nil {
		return toErrno(err)
	}
	if info.IsDir() {
		return experimentalsys.EISDIR
	}
	return toErrno(a.vfs.Remove(afPath))
}

func (a aferoSysFS) Rmdir(path string) experimentalsys.Errno {
	return toErrno(a.vfs.Remove(a.toAferoPath(path)))
}

func (a aferoSysFS) Utimens(path string, _, mtim int64) experimentalsys.Errno {
	mtime := time.Unix(0, mtim)
	return toErrno(a.vfs.Chtimes(a.toAferoPath(path), mtime, mtime))
}

// infoToStat converts fs.FileInfo to wazeroSys.Stat_t.
func infoToStat(info fs.FileInfo) wazeroSys.Stat_t {
	mode := info.Mode()
	nlink := uint64(1)
	if mode.IsDir() {
		nlink = 2
	}
	return wazeroSys.Stat_t{
		Mode:  mode,
		Size:  info.Size(),
		Nlink: nlink,
		Mtim:  info.ModTime().UnixNano(),
		Atim:  info.ModTime().UnixNano(),
		Ctim:  info.ModTime().UnixNano(),
	}
}

// toErrno maps a Go error to an experimentalsys.Errno.
func toErrno(err error) experimentalsys.Errno {
	if err == nil {
		return 0
	}
	if os.IsNotExist(err) {
		return experimentalsys.ENOENT
	}
	if os.IsExist(err) {
		return experimentalsys.EEXIST
	}
	if os.IsPermission(err) {
		return experimentalsys.EACCES
	}
	return experimentalsys.EIO
}

// ---------------------------------------------------------------------------
// aferoSysFile — wraps afero.File as experimentalsys.File (regular file)
// ---------------------------------------------------------------------------

type aferoSysFile struct {
	experimentalsys.UnimplementedFile
	f      afero.File
	append bool
}

func (f *aferoSysFile) IsAppend() bool { return f.append }

func (f *aferoSysFile) SetAppend(enable bool) experimentalsys.Errno {
	f.append = enable
	return 0
}

func (f *aferoSysFile) IsDir() (bool, experimentalsys.Errno) { return false, 0 }

func (f *aferoSysFile) Stat() (wazeroSys.Stat_t, experimentalsys.Errno) {
	info, err := f.f.Stat()
	if err != nil {
		return wazeroSys.Stat_t{}, experimentalsys.EIO
	}
	return infoToStat(info), 0
}

func (f *aferoSysFile) Read(buf []byte) (int, experimentalsys.Errno) {
	n, err := f.f.Read(buf)
	if err != nil && err != io.EOF {
		return n, experimentalsys.EIO
	}
	return n, 0
}

func (f *aferoSysFile) Pread(buf []byte, off int64) (int, experimentalsys.Errno) {
	n, err := f.f.ReadAt(buf, off)
	if err != nil && err != io.EOF {
		return n, experimentalsys.EIO
	}
	return n, 0
}

//nolint:govet
func (f *aferoSysFile) Seek(offset int64, whence int) (int64, experimentalsys.Errno) {
	n, err := f.f.Seek(offset, whence)
	if err != nil {
		return n, experimentalsys.EIO
	}
	return n, 0
}

func (f *aferoSysFile) Write(buf []byte) (int, experimentalsys.Errno) {
	n, err := f.f.Write(buf)
	if err != nil {
		return n, experimentalsys.EIO
	}
	return n, 0
}

func (f *aferoSysFile) Pwrite(buf []byte, off int64) (int, experimentalsys.Errno) {
	n, err := f.f.WriteAt(buf, off)
	if err != nil {
		return n, experimentalsys.EIO
	}
	return n, 0
}

func (f *aferoSysFile) Truncate(size int64) experimentalsys.Errno {
	return toErrno(f.f.Truncate(size))
}

func (f *aferoSysFile) Utimens(_, mtim int64) experimentalsys.Errno {
	// afero.File has no Chtimes; no-op is fine for WASI.
	_ = mtim
	return 0
}

func (f *aferoSysFile) Close() experimentalsys.Errno {
	return toErrno(f.f.Close())
}

// ---------------------------------------------------------------------------
// aferoSysDirFile — wraps afero.File as experimentalsys.File (directory)
// ---------------------------------------------------------------------------

type aferoSysDirFile struct {
	experimentalsys.UnimplementedFile
	f afero.File
}

func (d *aferoSysDirFile) IsDir() (bool, experimentalsys.Errno)     { return true, 0 }
func (d *aferoSysDirFile) IsAppend() bool                           { return false }
func (d *aferoSysDirFile) SetAppend(bool) experimentalsys.Errno     { return experimentalsys.EISDIR }
func (d *aferoSysDirFile) Read([]byte) (int, experimentalsys.Errno) { return 0, experimentalsys.EISDIR }
func (d *aferoSysDirFile) Pread([]byte, int64) (int, experimentalsys.Errno) {
	return 0, experimentalsys.EISDIR
}

func (d *aferoSysDirFile) Write([]byte) (int, experimentalsys.Errno) {
	return 0, experimentalsys.EISDIR
}

func (d *aferoSysDirFile) Pwrite([]byte, int64) (int, experimentalsys.Errno) {
	return 0, experimentalsys.EISDIR
}
func (d *aferoSysDirFile) Truncate(int64) experimentalsys.Errno { return experimentalsys.EISDIR }

func (d *aferoSysDirFile) Stat() (wazeroSys.Stat_t, experimentalsys.Errno) {
	info, err := d.f.Stat()
	if err != nil {
		return wazeroSys.Stat_t{}, experimentalsys.EIO
	}
	return infoToStat(info), 0
}

func (d *aferoSysDirFile) Readdir(n int) ([]experimentalsys.Dirent, experimentalsys.Errno) {
	infos, err := d.f.Readdir(n)
	if err != nil && err != io.EOF {
		return nil, experimentalsys.EIO
	}
	dirents := make([]experimentalsys.Dirent, 0, len(infos))
	for _, info := range infos {
		t := info.Mode().Type()
		dirents = append(dirents, experimentalsys.Dirent{
			Name: info.Name(),
			Type: t,
		})
	}
	return dirents, 0
}

//nolint:govet
func (d *aferoSysDirFile) Seek(offset int64, whence int) (int64, experimentalsys.Errno) {
	if whence == io.SeekStart && offset == 0 {
		_, err := d.f.Seek(0, 0)
		return 0, toErrno(err)
	}
	return 0, experimentalsys.ENOSYS
}

func (d *aferoSysDirFile) Close() experimentalsys.Errno {
	return toErrno(d.f.Close())
}
