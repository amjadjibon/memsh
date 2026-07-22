package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/amjadjibon/memsh/internal/server"
	"github.com/amjadjibon/memsh/internal/session"
	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestServerNewDefaults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := session.New(ctx, 30*time.Minute, 0)

	srv, err := server.New(server.Config{
		Addr:         ":0",
		SessionStore: store,
		BaseOpts:     []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)},
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	if srv == nil {
		t.Fatal("server.New returned nil server")
	}
	if srv.Addr != ":0" {
		t.Errorf("Addr = %q, want :0", srv.Addr)
	}
	if srv.Handler == nil {
		t.Fatal("Handler should not be nil")
	}
}

func TestServerNewWithAuthAndCORS(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := session.New(ctx, 30*time.Minute, 0)

	srv, err := server.New(server.Config{
		Addr:         ":0",
		SessionStore: store,
		APIKey:       "sekrit",
		CORSOrigin:   "https://example.com",
		BaseOpts:     []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)},
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	// Unauthenticated request to a protected route should be rejected.
	req, err := http.NewRequest(http.MethodGet, "http://unused/sessions", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated request: status %d, want 401", rr.Code)
	}

	// Health check stays open even with auth enabled.
	req2, _ := http.NewRequest(http.MethodGet, "http://unused/health", nil)
	rr2 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("/health with auth enabled: status %d, want 200", rr2.Code)
	}
}

func TestServerNewZeroTimeoutDefaultsPositive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := session.New(ctx, 30*time.Minute, 0)

	srv, err := server.New(server.Config{
		Addr:         ":0",
		SessionStore: store,
		Timeout:      0,
		BaseOpts:     []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)},
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	if srv.ReadTimeout <= 0 {
		t.Error("ReadTimeout should be positive")
	}
}

func TestStartCronScheduler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := session.New(context.Background(), time.Hour, 0)
	baseOpts := []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)}

	done := make(chan struct{})
	go func() {
		server.StartCronScheduler(ctx, store, baseOpts, time.Second)
		close(done)
	}()

	// Cancel immediately — the scheduler should exit its alignment wait and
	// return without ever needing to wait for a real minute boundary.
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartCronScheduler did not return after context cancellation")
	}
}
