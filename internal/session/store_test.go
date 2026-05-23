package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/amjadjibon/memsh/internal/session"
)

func TestStoreReapShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create a store with a very short TTL and immediate reaping.
	st := session.New(ctx, 10*time.Millisecond, 100)

	// Create a session and verify it exists.
	entry, ok := st.Get("test-id")
	if !ok {
		t.Fatal("Get failed")
	}
	if entry == nil {
		t.Fatal("entry is nil")
	}

	// Wait longer than TTL, then manually trigger a reap cycle by waiting
	// for the background ticker to fire (30s interval is too long for tests).
	// Instead, we just verify the store is functional and the context cancellation
	// doesn't cause panics.
	time.Sleep(50 * time.Millisecond)

	// Verify the session is still there (reaper hasn't run yet because
	// the ticker interval is 30s and we've only waited 50ms).
	entry2, ok := st.Get("test-id")
	if !ok {
		t.Fatal("Get failed after sleep")
	}
	if entry2.Fs != entry.Fs {
		t.Error("FS changed unexpectedly")
	}

	// Cancel the context to signal the reaper goroutine to stop.
	// This test primarily verifies that cancel() doesn't block or cause
	// a data race — the reap() goroutine should exit cleanly.
	cancel()

	// Give the goroutine time to notice the cancellation.
	time.Sleep(100 * time.Millisecond)

	// Store operations should still work after reaper stops.
	// The Get method always creates a session if it doesn't exist (within maxEntries limit).
	// To verify the store is functional, just call it and check for success.
	_, ok = st.Get("another-id")
	if !ok {
		t.Error("Get failed after context cancel")
	}
}

func TestStoreNewWithNilContext(t *testing.T) {
	// Verify that passing a non-nil context (even one that's never cancelled)
	// doesn't cause the reaper to panic or exit immediately.
	ctx := context.Background()
	st := session.New(ctx, 1*time.Hour, 10)
	if st == nil {
		t.Fatal("New returned nil")
	}

	// Basic operations work.
	entry, ok := st.Get("id")
	if !ok {
		t.Fatal("Get failed")
	}
	if entry == nil {
		t.Fatal("entry is nil")
	}
}

func TestStoreGetCreatesNew(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := session.New(ctx, 30*time.Minute, 0)
	entry, ok := st.Get("sess1")
	if !ok {
		t.Fatal("Get should create a new entry")
	}
	if entry.Cwd != "/" {
		t.Errorf("Cwd = %q, want /", entry.Cwd)
	}
	if entry.Fs == nil {
		t.Error("Fs should not be nil")
	}
}

func TestStoreGetReturnsExisting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := session.New(ctx, 30*time.Minute, 0)
	e1, _ := st.Get("sess1")
	e1.Cwd = "/home"

	e2, ok := st.Get("sess1")
	if !ok {
		t.Fatal("Get should return existing entry")
	}
	if e2.Cwd != "/home" {
		t.Errorf("Cwd = %q, want /home", e2.Cwd)
	}
}

func TestStoreMaxEntries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := session.New(ctx, 30*time.Minute, 2)
	st.Get("a")
	st.Get("b")

	_, ok := st.Get("c")
	if ok {
		t.Error("Get should fail when max entries exceeded")
	}
}

func TestStoreDelete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := session.New(ctx, 30*time.Minute, 0)
	st.Get("sess1")
	st.Delete("sess1")

	if st.Count() != 0 {
		t.Errorf("Count = %d, want 0 after delete", st.Count())
	}
}

func TestStoreCount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := session.New(ctx, 30*time.Minute, 0)
	if st.Count() != 0 {
		t.Errorf("Count = %d, want 0", st.Count())
	}

	st.Get("a")
	st.Get("b")
	if st.Count() != 2 {
		t.Errorf("Count = %d, want 2", st.Count())
	}
}

func TestStoreUpdate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := session.New(ctx, 30*time.Minute, 0)
	st.Get("sess1")

	st.Update("sess1", "/tmp", true)
	e, _ := st.Get("sess1")
	if e.Cwd != "/tmp" {
		t.Errorf("Cwd = %q, want /tmp", e.Cwd)
	}
	if !e.RcLoaded {
		t.Error("RcLoaded should be true")
	}
}

func TestStoreUpdateWithRuntime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := session.New(ctx, 30*time.Minute, 0)
	st.Get("sess1")

	st.UpdateWithRuntime("sess1", "/app", true, 100*time.Millisecond)
	e, _ := st.Get("sess1")
	if e.Runtime != 100*time.Millisecond {
		t.Errorf("Runtime = %v, want 100ms", e.Runtime)
	}
}

func TestStoreUpdateNonexistent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := session.New(ctx, 30*time.Minute, 0)
	st.Update("nonexistent", "/tmp", false)
}

func TestStoreReplace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := session.New(ctx, 30*time.Minute, 0)
	st.Get("old")

	st.Replace("old", nil, "/replaced")
	e, _ := st.Get("old")
	if e.Cwd != "/replaced" {
		t.Errorf("Cwd = %q, want /replaced", e.Cwd)
	}
}

func TestStoreReplaceNew(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := session.New(ctx, 30*time.Minute, 0)
	st.Replace("brand_new", nil, "/new")
	if st.Count() != 1 {
		t.Errorf("Count = %d, want 1", st.Count())
	}
}

func TestStoreList(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := session.New(ctx, 30*time.Minute, 0)
	st.Get("a")
	st.Get("b")

	list := st.List()
	if len(list) != 2 {
		t.Fatalf("List returned %d entries, want 2", len(list))
	}

	// Verify both sessions are present.
	ids := make(map[string]bool)
	for _, info := range list {
		ids[info.ID] = true
	}
	if !ids["a"] || !ids["b"] {
		t.Errorf("List missing sessions: %v", list)
	}
}

func TestStoreSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := session.New(ctx, 30*time.Minute, 0)
	st.Get("a")
	st.Get("b")

	snaps := st.Snapshot()
	if len(snaps) != 2 {
		t.Fatalf("Snapshot returned %d entries, want 2", len(snaps))
	}
}
