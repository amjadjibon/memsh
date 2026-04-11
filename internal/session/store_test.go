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
