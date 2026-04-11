// Package session manages persistent shell sessions for the memsh HTTP server.
// Each session has its own virtual filesystem (afero.MemMapFs) and working
// directory that persist across HTTP requests when using the X-Session-ID header.
//
// Sessions support:
//   - Virtual filesystem isolation between sessions
//   - Working directory persistence
//   - Automatic alias persistence
//   - Cron job scheduling per session
//   - TTL-based expiration and reaping
package session

import (
	"slices"
	"sync"
	"time"

	"github.com/spf13/afero"
)

// Entry holds the persistent state (filesystem + cwd) shared across
// requests. Each request still creates its own shell.Shell pointing at the
// session's FS, so per-request I/O capture works correctly.
type Entry struct {
	Fs        afero.Fs
	Cwd       string
	CreatedAt time.Time
	LastUse   time.Time
	RcLoaded  bool       // true after /.memshrc has been sourced for this session
	CronMu    sync.Mutex // serialises concurrent cron job writes to /.cron_log
}

// Store manages persistent shell sessions keyed by an arbitrary ID.
type Store struct {
	mu         sync.Mutex
	entries    map[string]*Entry
	ttl        time.Duration
	maxEntries int
}

// New creates a new session store with the given TTL and max entries.
func New(ttl time.Duration, maxEntries int) *Store {
	st := &Store{
		entries:    make(map[string]*Entry),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
	go st.reap()
	return st
}

// Get returns an existing session entry or creates one.
// Returns (entry, true) on success, or (nil, false) if the max session limit
// would be exceeded by creating a new session.
func (st *Store) Get(id string) (*Entry, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if e, ok := st.entries[id]; ok {
		e.LastUse = time.Now()
		return e, true
	}
	// Enforce max-sessions limit.
	if st.maxEntries > 0 && len(st.entries) >= st.maxEntries {
		return nil, false
	}
	now := time.Now()
	e := &Entry{
		Fs:        afero.NewMemMapFs(),
		Cwd:       "/",
		CreatedAt: now,
		LastUse:   now,
	}
	st.entries[id] = e
	return e, true
}

// Update records the cwd after a request finishes so the next request
// in the same session picks it up. Alias state is persisted in the virtual FS
// via /.memsh_session_aliases, so it does not need a separate field here.
func (st *Store) Update(id, cwd string, rcLoaded bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if e, ok := st.entries[id]; ok {
		e.Cwd = cwd
		if rcLoaded {
			e.RcLoaded = true
		}
	}
}

// Replace creates or overwrites a session with the given filesystem and cwd.
// Used by the snapshot import endpoint.
func (st *Store) Replace(id string, fs afero.Fs, cwd string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now()
	if e, ok := st.entries[id]; ok {
		e.Fs = fs
		e.Cwd = cwd
		e.RcLoaded = false
		e.LastUse = now
		return
	}
	st.entries[id] = &Entry{
		Fs:        fs,
		Cwd:       cwd,
		CreatedAt: now,
		LastUse:   now,
	}
}

// Delete removes and discards a session.
func (st *Store) Delete(id string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.entries, id)
}

// Count returns the number of active sessions.
func (st *Store) Count() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.entries)
}

// Info is one entry in the session list response.
type Info struct {
	ID        string `json:"id"`
	Cwd       string `json:"cwd"`
	CreatedAt string `json:"created_at"`
	LastUse   string `json:"last_use"`
}

// List returns all sessions sorted by last use (most recent first).
func (st *Store) List() []Info {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]Info, 0, len(st.entries))
	for id, e := range st.entries {
		out = append(out, Info{
			ID:        id,
			Cwd:       e.Cwd,
			CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339),
			LastUse:   e.LastUse.UTC().Format(time.RFC3339),
		})
	}

	slices.SortFunc(out, func(a, b Info) int {
		if a.LastUse > b.LastUse {
			return -1
		}
		if a.LastUse < b.LastUse {
			return 1
		}
		return 0
	})
	return out
}

// reap removes sessions that have exceeded the TTL.
func (st *Store) reap() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		st.mu.Lock()
		for id, e := range st.entries {
			if time.Since(e.LastUse) > st.ttl {
				delete(st.entries, id)
			}
		}
		st.mu.Unlock()
	}
}

// Snap is a lightweight copy of a session's key fields for use by the
// cron scheduler without holding the store lock during job execution.
type Snap struct {
	ID     string
	Fs     afero.Fs
	Cwd    string
	CronMu *sync.Mutex
}

// Snapshot returns a point-in-time copy of all active sessions.
func (st *Store) Snapshot() []Snap {
	st.mu.Lock()
	defer st.mu.Unlock()
	result := make([]Snap, 0, len(st.entries))
	for id, e := range st.entries {
		result = append(result, Snap{
			ID:     id,
			Fs:     e.Fs,
			Cwd:    e.Cwd,
			CronMu: &e.CronMu,
		})
	}
	return result
}
