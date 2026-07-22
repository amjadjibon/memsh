package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/amjadjibon/memsh/internal/session"
	"github.com/spf13/afero"
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


func TestValidSessionID(t *testing.T) {
	if !session.ValidSessionID("abcdef0123456789") {
		t.Error("expected valid hex id")
	}
	if session.ValidSessionID("to-delete") {
		t.Error("expected invalid weak id")
	}
	if session.ValidSessionID("new") {
		t.Error("new is not a valid client id format")
	}
	if session.ValidSessionID("aaaaaaaaaaaaaaa") { // 15 chars
		t.Error("expected too short")
	}
	if session.ValidSessionID("zzzzzzzzzzzzzzzz") {
		t.Error("non-hex should be invalid")
	}
}

func TestStoreReplaceMaxEntries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := session.New(ctx, time.Hour, 1)
	if !st.Replace("aaaaaaaaaaaaaaaa", nil, "/") {
		t.Fatal("first Replace should succeed")
	}
	if st.Replace("bbbbbbbbbbbbbbbb", nil, "/") {
		t.Fatal("second Replace should fail at max")
	}
	if !st.Replace("aaaaaaaaaaaaaaaa", nil, "/x") {
		t.Fatal("overwrite should succeed")
	}
}

func TestStoreGetExisting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := session.New(ctx, time.Hour, 10)
	if _, ok := st.GetExisting("aaaaaaaaaaaaaaaa"); ok {
		t.Fatal("expected missing")
	}
	st.Get("aaaaaaaaaaaaaaaa")
	if _, ok := st.GetExisting("aaaaaaaaaaaaaaaa"); !ok {
		t.Fatal("expected existing")
	}
}

func TestNewPersistentRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st, err := session.NewPersistent(ctx, time.Hour, 0, dir)
	if err != nil {
		t.Fatalf("NewPersistent: %v", err)
	}
	entry, ok := st.Get("aaaaaaaaaaaaaaaa")
	if !ok {
		t.Fatal("Get failed")
	}
	if err := afero.WriteFile(entry.Fs, "/marker.txt", []byte("survives restart"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Update persists the current FS/cwd snapshot to disk.
	st.Update("aaaaaaaaaaaaaaaa", "/home", true)

	// Simulate a server restart: create a brand new store pointed at the
	// same persistDir and confirm the session comes back.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	st2, err := session.NewPersistent(ctx2, time.Hour, 0, dir)
	if err != nil {
		t.Fatalf("NewPersistent (reload): %v", err)
	}
	restored, ok := st2.GetExisting("aaaaaaaaaaaaaaaa")
	if !ok {
		t.Fatal("expected restored session to exist after reload")
	}
	if restored.Cwd != "/home" {
		t.Errorf("Cwd = %q, want /home", restored.Cwd)
	}
	if !restored.RcLoaded {
		t.Error("RcLoaded should be true after reload")
	}
	data, err := afero.ReadFile(restored.Fs, "/marker.txt")
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(data) != "survives restart" {
		t.Errorf("restored file content = %q", data)
	}
}

func TestNewPersistentExpiredSessionDropped(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st, err := session.NewPersistent(ctx, 10*time.Millisecond, 0, dir)
	if err != nil {
		t.Fatalf("NewPersistent: %v", err)
	}
	st.Get("aaaaaaaaaaaaaaaa")
	st.Update("aaaaaaaaaaaaaaaa", "/", false)

	time.Sleep(20 * time.Millisecond)

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	st2, err := session.NewPersistent(ctx2, 10*time.Millisecond, 0, dir)
	if err != nil {
		t.Fatalf("NewPersistent (reload): %v", err)
	}
	if _, ok := st2.GetExisting("aaaaaaaaaaaaaaaa"); ok {
		t.Error("expired session should not be restored")
	}
}

func TestNewPersistentDeleteRemovesFile(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st, err := session.NewPersistent(ctx, time.Hour, 0, dir)
	if err != nil {
		t.Fatalf("NewPersistent: %v", err)
	}
	st.Get("aaaaaaaaaaaaaaaa")
	st.Update("aaaaaaaaaaaaaaaa", "/", false)
	st.Delete("aaaaaaaaaaaaaaaa")

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	st2, err := session.NewPersistent(ctx2, time.Hour, 0, dir)
	if err != nil {
		t.Fatalf("NewPersistent (reload): %v", err)
	}
	if _, ok := st2.GetExisting("aaaaaaaaaaaaaaaa"); ok {
		t.Error("deleted session should not reappear after reload")
	}
}
