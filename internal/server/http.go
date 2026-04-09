package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/amjadjibon/memsh/internal/config"
	"github.com/amjadjibon/memsh/internal/session"
	"github.com/amjadjibon/memsh/pkg/shell"
)

// BaseOpts are shell options that should always be applied in server mode.
var BaseOpts = []shell.Option{
	shell.WithInheritEnv(false),
}

// Config holds the HTTP server configuration.
type Config struct {
	Addr         string         // TCP address to listen on (e.g., ":8080")
	TTL          time.Duration  // Session idle timeout before reaping
	Timeout      time.Duration  // Per-request execution timeout
	CORSOrigin   string         // CORS allowed origin (empty = no CORS)
	APIKey       string         // API key for Bearer authentication (empty = no auth)
	MaxSessions  int            // Maximum concurrent sessions (0 = unlimited)
	SessionStore *session.Store // Session store for persistence
}

// New creates a new HTTP server with the given configuration.
//
// The server includes:
//   - Middleware chain (auth, security headers, CORS)
//   - All route handlers registered
//   - Proper timeouts configured
//
// Returns an error if shell config loading fails.
func New(cfg Config) (*http.Server, error) {
	// Enforce a minimum timeout to prevent unbounded execution.
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// Load shell config and build base options.
	shellCfg, _ := config.Load()
	baseOpts := config.BuildShellOpts(shellCfg)
	baseOpts = append(baseOpts, shell.WithInheritEnv(false))

	// Only enable auth if an API key was explicitly provided.
	if cfg.APIKey != "" && cfg.APIKey != "default" {
		fmt.Fprintln(os.Stderr, "memsh serve: API key authentication enabled")
	}

	// Create handler and register routes.
	handler := NewHandler(cfg.SessionStore, baseOpts, timeout)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Build the middleware chain.
	var h http.Handler = mux

	// API key authentication (applied to all endpoints except GET / and GET /health).
	if cfg.APIKey != "" {
		h = APIKeyMiddleware(h, cfg.APIKey, "/", "/health")
	}

	// Security headers (CSP, etc.).
	h = SecurityHeadersMiddleware(h)

	// CORS (only if an explicit origin is configured).
	if cfg.CORSOrigin != "" {
		h = CORSMiddleware(h, cfg.CORSOrigin)
	}

	return &http.Server{
		Addr:         cfg.Addr,
		Handler:      h,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}, nil
}

// StartCronScheduler starts the cron scheduler for the session store.
// It runs in a background goroutine and should be cancelled via the context
// when the server shuts down. The scheduler fires every minute for all active
// sessions, executing cron jobs from their respective /.crontab files.
func StartCronScheduler(ctx context.Context, store *session.Store, baseOpts []shell.Option, timeout time.Duration) {
	session.StartScheduler(ctx, store, baseOpts, timeout)
}
