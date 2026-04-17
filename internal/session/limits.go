package session

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/afero"
)

// Limits defines optional per-session resource caps.
// A zero value for each field means "unlimited" for that resource.
type Limits struct {
	MaxFiles   int
	MaxBytes   int64
	MaxRuntime time.Duration
}

func (l Limits) HasFSLimits() bool {
	return l.MaxFiles > 0 || l.MaxBytes > 0
}

func (l Limits) ValidateRuntime(used time.Duration) error {
	if l.MaxRuntime <= 0 {
		return nil
	}
	if used >= l.MaxRuntime {
		return fmt.Errorf("session runtime limit exceeded: used %s, max %s", used.Round(time.Millisecond), l.MaxRuntime)
	}
	return nil
}

func (l Limits) EffectiveTimeout(base, used, minTimeout time.Duration) (time.Duration, error) {
	if base <= 0 {
		base = minTimeout
	}
	if l.MaxRuntime <= 0 {
		return base, nil
	}
	remaining := l.MaxRuntime - used
	if remaining <= 0 {
		return 0, fmt.Errorf("session runtime limit exceeded: used %s, max %s", used.Round(time.Millisecond), l.MaxRuntime)
	}
	if remaining < base {
		return remaining, nil
	}
	return base, nil
}

func (l Limits) ValidateFS(fs afero.Fs) error {
	files, bytes, err := fsUsage(fs)
	if err != nil {
		return err
	}
	if l.MaxFiles > 0 && files > l.MaxFiles {
		return fmt.Errorf("session file limit exceeded: %d files (max %d)", files, l.MaxFiles)
	}
	if l.MaxBytes > 0 && bytes > l.MaxBytes {
		return fmt.Errorf("session storage limit exceeded: %d bytes (max %d)", bytes, l.MaxBytes)
	}
	return nil
}

func fsUsage(fs afero.Fs) (files int, bytes int64, err error) {
	err = afero.Walk(fs, "/", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "/" || info.IsDir() {
			return nil
		}
		files++
		bytes += info.Size()
		return nil
	})
	return files, bytes, err
}
