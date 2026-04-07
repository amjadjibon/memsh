package shell

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

const snapshotVersion = 1

// Snapshot is a portable, JSON-serialisable representation of a memsh session:
// the complete virtual filesystem plus the working directory.
// File content is stored as raw bytes; the standard encoding/json package
// encodes []byte as base64 automatically.
type Snapshot struct {
	Version int            `json:"version"`
	Cwd     string         `json:"cwd"`
	Files   []SnapshotFile `json:"files"`
}

// SnapshotFile represents one node (regular file or directory) in the snapshot.
type SnapshotFile struct {
	Path    string      `json:"path"`
	Mode    os.FileMode `json:"mode"`
	IsDir   bool        `json:"is_dir,omitempty"`
	Content []byte      `json:"content,omitempty"` // nil/absent for directories
}

// TakeSnapshot walks fs and returns a Snapshot containing every file and
// directory, together with cwd. It never follows symbolic links.
func TakeSnapshot(fs afero.Fs, cwd string) (*Snapshot, error) {
	snap := &Snapshot{
		Version: snapshotVersion,
		Cwd:     cwd,
	}

	err := afero.Walk(fs, "/", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "/" {
			return nil // skip the root itself
		}
		entry := SnapshotFile{
			Path:  path,
			Mode:  info.Mode(),
			IsDir: info.IsDir(),
		}
		if !info.IsDir() {
			f, err := fs.Open(path)
			if err != nil {
				return fmt.Errorf("snapshot: open %s: %w", path, err)
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				return fmt.Errorf("snapshot: read %s: %w", path, err)
			}
			entry.Content = data
		}
		snap.Files = append(snap.Files, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// RestoreSnapshot creates a new in-memory filesystem from snap and returns it
// together with the saved working directory.
func RestoreSnapshot(snap *Snapshot) (afero.Fs, string, error) {
	if snap.Version != snapshotVersion {
		return nil, "", fmt.Errorf("snapshot: unsupported version %d (want %d)", snap.Version, snapshotVersion)
	}
	fs := afero.NewMemMapFs()
	for _, entry := range snap.Files {
		if entry.IsDir {
			if err := fs.MkdirAll(entry.Path, entry.Mode); err != nil {
				return nil, "", fmt.Errorf("snapshot: mkdir %s: %w", entry.Path, err)
			}
			continue
		}
		// Ensure parent directory exists.
		if dir := filepath.Dir(entry.Path); dir != "/" && dir != "." {
			if err := fs.MkdirAll(dir, 0755); err != nil {
				return nil, "", fmt.Errorf("snapshot: mkdir parent %s: %w", dir, err)
			}
		}
		if err := afero.WriteFile(fs, entry.Path, entry.Content, entry.Mode); err != nil {
			return nil, "", fmt.Errorf("snapshot: write %s: %w", entry.Path, err)
		}
	}
	cwd := snap.Cwd
	if cwd == "" {
		cwd = "/"
	}
	return fs, cwd, nil
}

// MarshalSnapshot serialises snap to JSON.
func MarshalSnapshot(snap *Snapshot) ([]byte, error) {
	return json.MarshalIndent(snap, "", "  ")
}

// UnmarshalSnapshot parses JSON produced by MarshalSnapshot.
func UnmarshalSnapshot(data []byte) (*Snapshot, error) {
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("snapshot: parse: %w", err)
	}
	return &snap, nil
}
