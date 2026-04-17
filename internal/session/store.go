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
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/amjadjibon/memsh/pkg/network"
	"github.com/amjadjibon/memsh/pkg/shell"
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
	RcLoaded  bool // true after /.memshrc has been sourced for this session
	Runtime   time.Duration
	Network   network.Usage
	CronMu    sync.Mutex // serialises concurrent cron job writes to /.cron_log
}

// Store manages persistent shell sessions keyed by an arbitrary ID.
type Store struct {
	mu         sync.Mutex
	entries    map[string]*Entry
	ttl        time.Duration
	maxEntries int
	persistDir string
}

// New creates a new session store with the given TTL and max entries.
// The background reaper goroutine runs until ctx is cancelled, so callers
// should pass a context tied to the server lifetime to avoid goroutine leaks
// on shutdown.
func New(ctx context.Context, ttl time.Duration, maxEntries int) *Store {
	st := &Store{
		entries:    make(map[string]*Entry),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
	go st.reap(ctx)
	return st
}

// NewPersistent creates a new session store with durable persistence.
func NewPersistent(ctx context.Context, ttl time.Duration, maxEntries int, persistDir string) (*Store, error) {
	st := &Store{
		entries:    make(map[string]*Entry),
		ttl:        ttl,
		maxEntries: maxEntries,
		persistDir: strings.TrimSpace(persistDir),
	}
	if st.persistDir != "" {
		if err := os.MkdirAll(st.persistDir, 0o755); err != nil {
			return nil, err
		}
		if err := st.loadPersisted(); err != nil {
			return nil, err
		}
	}
	go st.reap(ctx)
	return st, nil
}

// Get returns an existing session entry or creates one.
// Returns (entry, true) on success, or (nil, false) if the max session limit
// would be exceeded by creating a new session.
func (st *Store) Get(id string) (*Entry, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if e, ok := st.entries[id]; ok {
		e.LastUse = time.Now()
		st.persistLocked(id, e)
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
	st.persistLocked(id, e)
	return e, true
}

// Update records the cwd after a request finishes so the next request
// in the same session picks it up. Alias state is persisted in the virtual FS
// via /.memsh_session_aliases, so it does not need a separate field here.
func (st *Store) Update(id, cwd string, rcLoaded bool) {
	st.UpdateWithRuntime(id, cwd, rcLoaded, 0)
}

// UpdateWithRuntime updates session state and runtime accounting.
func (st *Store) UpdateWithRuntime(id, cwd string, rcLoaded bool, runtimeDelta time.Duration) {
	st.updateLocked(id, cwd, rcLoaded, runtimeDelta, false, network.Usage{})
}

// UpdateWithRuntimeAndNetwork updates session state and cumulative network usage.
func (st *Store) UpdateWithRuntimeAndNetwork(id, cwd string, rcLoaded bool, runtimeDelta time.Duration, netUsage network.Usage) {
	st.updateLocked(id, cwd, rcLoaded, runtimeDelta, true, netUsage)
}

func (st *Store) updateLocked(id, cwd string, rcLoaded bool, runtimeDelta time.Duration, setNetwork bool, netUsage network.Usage) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if e, ok := st.entries[id]; ok {
		e.Cwd = cwd
		if runtimeDelta > 0 {
			e.Runtime += runtimeDelta
		}
		if rcLoaded {
			e.RcLoaded = true
		}
		if setNetwork {
			e.Network = netUsage
		}
		e.LastUse = time.Now()
		st.persistLocked(id, e)
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
		e.Runtime = 0
		e.Network = network.Usage{}
		e.LastUse = now
		st.persistLocked(id, e)
		return
	}
	st.entries[id] = &Entry{
		Fs:        fs,
		Cwd:       cwd,
		CreatedAt: now,
		LastUse:   now,
	}
	st.persistLocked(id, st.entries[id])
}

// Delete removes and discards a session.
func (st *Store) Delete(id string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.entries, id)
	st.deletePersistedLocked(id)
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
	RuntimeMS int64  `json:"runtime_ms"`
	NetReqs   int    `json:"network_requests"`
	NetSent   int64  `json:"network_bytes_sent"`
	NetRecv   int64  `json:"network_bytes_received"`
	NetRunMS  int64  `json:"network_runtime_ms"`
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
			RuntimeMS: e.Runtime.Milliseconds(),
			NetReqs:   e.Network.Requests,
			NetSent:   e.Network.BytesSent,
			NetRecv:   e.Network.BytesReceived,
			NetRunMS:  e.Network.Runtime.Milliseconds(),
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
// It stops when ctx is cancelled, preventing a goroutine leak on server shutdown.
func (st *Store) reap(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			st.mu.Lock()
			for id, e := range st.entries {
				if time.Since(e.LastUse) > st.ttl {
					delete(st.entries, id)
					st.deletePersistedLocked(id)
				}
			}
			st.mu.Unlock()
		}
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

func (st *Store) persistLocked(id string, e *Entry) {
	if st.persistDir == "" {
		return
	}
	snap, err := shell.TakeSnapshot(e.Fs, e.Cwd)
	if err != nil {
		log.Printf("session persistence snapshot %s: %v", id, err)
		return
	}
	if err := writePersistedSession(st.persistDir, persistedSession{
		ID:        id,
		Cwd:       e.Cwd,
		CreatedAt: e.CreatedAt,
		LastUse:   e.LastUse,
		RcLoaded:  e.RcLoaded,
		RuntimeNS: e.Runtime.Nanoseconds(),
		Network:   e.Network,
		Snapshot:  snap,
	}); err != nil {
		log.Printf("session persistence write %s: %v", id, err)
	}
}

func (st *Store) deletePersistedLocked(id string) {
	if st.persistDir == "" {
		return
	}
	if err := removePersistedSession(st.persistDir, id); err != nil {
		log.Printf("session persistence remove %s: %v", id, err)
	}
}

func (st *Store) loadPersisted() error {
	entries, err := os.ReadDir(st.persistDir)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, de := range entries {
		if de.IsDir() || filepath.Ext(de.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(st.persistDir, de.Name()))
		if err != nil {
			log.Printf("session persistence read %s: %v", de.Name(), err)
			continue
		}
		var rec persistedSession
		if err := json.Unmarshal(data, &rec); err != nil {
			log.Printf("session persistence decode %s: %v", de.Name(), err)
			continue
		}
		if rec.ID == "" || rec.Snapshot == nil {
			continue
		}
		if st.maxEntries > 0 && len(st.entries) >= st.maxEntries {
			break
		}
		if st.ttl > 0 && now.Sub(rec.LastUse) > st.ttl {
			_ = os.Remove(filepath.Join(st.persistDir, de.Name()))
			continue
		}
		fs, cwd, err := shell.RestoreSnapshot(rec.Snapshot)
		if err != nil {
			log.Printf("session persistence restore %s: %v", rec.ID, err)
			continue
		}
		if cwd == "" {
			cwd = rec.Cwd
		}
		if cwd == "" {
			cwd = "/"
		}
		createdAt := rec.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}
		lastUse := rec.LastUse
		if lastUse.IsZero() {
			lastUse = now
		}
		st.entries[rec.ID] = &Entry{
			Fs:        fs,
			Cwd:       cwd,
			CreatedAt: createdAt,
			LastUse:   lastUse,
			RcLoaded:  rec.RcLoaded,
			Runtime:   time.Duration(rec.RuntimeNS),
			Network:   rec.Network,
		}
	}
	return nil
}
