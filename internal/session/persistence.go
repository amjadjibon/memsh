package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

type persistedSession struct {
	ID        string          `json:"id"`
	Cwd       string          `json:"cwd"`
	CreatedAt time.Time       `json:"created_at"`
	LastUse   time.Time       `json:"last_use"`
	RcLoaded  bool            `json:"rc_loaded"`
	RuntimeNS int64           `json:"runtime_ns"`
	Snapshot  *shell.Snapshot `json:"snapshot"`
}

func sessionFilePath(dir, id string) string {
	sum := sha256.Sum256([]byte(id))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json")
}

func writePersistedSession(dir string, rec persistedSession) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("session store: create dir: %w", err)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	target := sessionFilePath(dir, rec.ID)
	tmp, err := os.CreateTemp(dir, "session-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace session file: %w", err)
	}
	return nil
}

func removePersistedSession(dir, id string) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	if err := os.Remove(sessionFilePath(dir, id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func readPersistedSession(dir, id string) (*persistedSession, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, os.ErrNotExist
	}
	data, err := os.ReadFile(sessionFilePath(dir, id))
	if err != nil {
		return nil, err
	}
	var rec persistedSession
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// LoadShellSession restores one persisted shell session by id.
func LoadShellSession(dir, id string) (afero.Fs, string, time.Duration, bool, error) {
	rec, err := readPersistedSession(dir, id)
	if err != nil {
		return nil, "", 0, false, err
	}
	if rec.Snapshot == nil {
		return nil, "", 0, false, fmt.Errorf("invalid persisted session: missing snapshot")
	}
	fs, cwd, err := shell.RestoreSnapshot(rec.Snapshot)
	if err != nil {
		return nil, "", 0, false, err
	}
	if cwd == "" {
		cwd = rec.Cwd
	}
	if cwd == "" {
		cwd = "/"
	}
	return fs, cwd, time.Duration(rec.RuntimeNS), rec.RcLoaded, nil
}

// SaveShellSession saves one shell session to persistence storage.
func SaveShellSession(dir, id string, fs afero.Fs, cwd string, runtime time.Duration, rcLoaded bool) error {
	if strings.TrimSpace(dir) == "" || strings.TrimSpace(id) == "" {
		return nil
	}
	snap, err := shell.TakeSnapshot(fs, cwd)
	if err != nil {
		return err
	}
	now := time.Now()
	return writePersistedSession(dir, persistedSession{
		ID:        id,
		Cwd:       cwd,
		CreatedAt: now,
		LastUse:   now,
		RcLoaded:  rcLoaded,
		RuntimeNS: runtime.Nanoseconds(),
		Snapshot:  snap,
	})
}
