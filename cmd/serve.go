package cmd

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/amjadjibon/memsh/shell"
	"github.com/amjadjibon/memsh/shell/plugins/native"
	"github.com/amjadjibon/memsh/web"
)

// maxRequestBodySize limits the size of incoming JSON request bodies (1 MB).
const maxRequestBodySize = 1 << 20

// minTimeout is the minimum enforced per-request timeout even if --timeout=0.
const minTimeout = 5 * time.Second

// runRequest is the JSON body accepted by POST /run.
type runRequest struct {
	Script string `json:"script"`
}

// runResponse is the JSON body returned by POST /run.
type runResponse struct {
	Output string `json:"output"`
	Pager  bool   `json:"pager,omitempty"`
	Cwd    string `json:"cwd"`
	Error  string `json:"error,omitempty"`
}

// healthResponse is returned by GET /health.
type healthResponse struct {
	Status   string `json:"status"`
	Uptime   string `json:"uptime"`
	Sessions int    `json:"sessions"`
}

// sessionInfo is one entry in GET /sessions.
type sessionInfo struct {
	ID        string `json:"id"`
	Cwd       string `json:"cwd"`
	CreatedAt string `json:"created_at"`
	LastUse   string `json:"last_use"`
}

// sessionEntry holds the persistent state (filesystem + cwd) shared across
// requests. Each request still creates its own shell.Shell pointing at the
// session's FS, so per-request I/O capture works correctly.
type sessionEntry struct {
	fs        afero.Fs
	cwd       string
	createdAt time.Time
	lastUse   time.Time
}

// sessionStore manages persistent shell sessions keyed by an arbitrary ID.
type sessionStore struct {
	mu         sync.Mutex
	entries    map[string]*sessionEntry
	ttl        time.Duration
	maxEntries int
}

func newSessionStore(ttl time.Duration, maxEntries int) *sessionStore {
	st := &sessionStore{
		entries:    make(map[string]*sessionEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
	go st.reap()
	return st
}

// get returns an existing session entry or creates one.
// Returns (entry, true) on success, or (nil, false) if the max session limit
// would be exceeded by creating a new session.
func (st *sessionStore) get(id string) (*sessionEntry, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if e, ok := st.entries[id]; ok {
		e.lastUse = time.Now()
		return e, true
	}
	// Enforce max-sessions limit.
	if st.maxEntries > 0 && len(st.entries) >= st.maxEntries {
		return nil, false
	}
	now := time.Now()
	e := &sessionEntry{
		fs:        afero.NewMemMapFs(),
		cwd:       "/",
		createdAt: now,
		lastUse:   now,
	}
	st.entries[id] = e
	return e, true
}

// updateCwd records the cwd after a request finishes so the next request in
// the same session picks it up.
func (st *sessionStore) updateCwd(id, cwd string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if e, ok := st.entries[id]; ok {
		e.cwd = cwd
	}
}

// delete removes and discards a session.
func (st *sessionStore) delete(id string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.entries, id)
}

// count returns the number of active sessions.
func (st *sessionStore) count() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.entries)
}

// list returns all sessions sorted by last use (most recent first).
func (st *sessionStore) list() []sessionInfo {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]sessionInfo, 0, len(st.entries))
	for id, e := range st.entries {
		out = append(out, sessionInfo{
			ID:        id,
			Cwd:       e.cwd,
			CreatedAt: e.createdAt.UTC().Format(time.RFC3339),
			LastUse:   e.lastUse.UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastUse > out[j].LastUse
	})
	return out
}

// reap removes sessions that have exceeded the TTL.
func (st *sessionStore) reap() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		st.mu.Lock()
		for id, e := range st.entries {
			if time.Since(e.lastUse) > st.ttl {
				delete(st.entries, id)
			}
		}
		st.mu.Unlock()
	}
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start an HTTP server exposing the memsh shell over a REST API",
	Long: `Start an HTTP API server that executes shell commands inside memsh.

Endpoints:
  GET  /                  Web terminal UI
  POST /run               Execute a script (JSON body: {"script":"ls /"})
  GET  /sessions          List all active sessions
  DELETE /session/{id}    Destroy a session
  GET  /health            Return server status

Sessions are always enabled. Send X-Session-ID: <id> on POST /run to preserve
virtual filesystem and working directory across requests. Sessions expire after
the idle TTL (--session-ttl).

Authentication:
  Pass --api-key <key> to require authentication on mutating endpoints.
  Clients must send the key via the Authorization header: "Bearer <key>".`,
	RunE: runServe,
}

func runServe(cmd *cobra.Command, _ []string) error {
	addr, _ := cmd.Flags().GetString("addr")
	ttlStr, _ := cmd.Flags().GetString("session-ttl")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	corsOrigin, _ := cmd.Flags().GetString("cors-origin")
	apiKey, _ := cmd.Flags().GetString("api-key")
	maxSessions, _ := cmd.Flags().GetInt("max-sessions")

	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return fmt.Errorf("invalid --session-ttl: %w", err)
	}

	// Enforce a minimum timeout to prevent unbounded execution.
	if timeout <= 0 {
		timeout = minTimeout
	}

	// Only enable auth if an API key was explicitly provided.
	if cmd.Flags().Changed("api-key") && apiKey != "" {
		fmt.Fprintln(os.Stderr, "memsh serve: API key authentication enabled")
	}

	cfg, _ := loadConfig()
	baseOpts := buildShellOpts(cfg)

	// In server mode, do not inherit the host process's environment
	// to prevent leaking secrets (API keys, DB URLs, etc.) to remote users.
	baseOpts = append(baseOpts, shell.WithInheritEnv(false))

	store := newSessionStore(ttl, maxSessions)
	start := time.Now()
	mux := http.NewServeMux()

	// ── GET / ───────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(web.TerminalHTML)
	})

	// ── POST /run ──────────────────────────────────────────────────────────
	mux.HandleFunc("POST /run", func(w http.ResponseWriter, r *http.Request) {
		// Limit request body size to prevent memory exhaustion.
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

		var req runRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, runResponse{Error: "invalid request body"})
			return
		}
		req.Script = strings.TrimSpace(req.Script)
		if req.Script == "" {
			writeJSON(w, http.StatusBadRequest, runResponse{Error: "script is required"})
			return
		}

		ctx := r.Context()
		ctx = native.WithPagerMode(ctx)
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()

		var out strings.Builder
		opts := make([]shell.Option, len(baseOpts)+1)
		copy(opts, baseOpts)
		opts[len(baseOpts)] = shell.WithStdIO(strings.NewReader(""), &out, &out)

		sessionID := r.Header.Get("X-Session-ID")
		if sessionID != "" {
			entry, ok := store.get(sessionID)
			if !ok {
				writeJSON(w, http.StatusTooManyRequests, runResponse{
					Error: "maximum number of sessions reached",
				})
				return
			}
			opts = append(opts, shell.WithFS(entry.fs), shell.WithCwd(entry.cwd))
		}

		sh, err := shell.New(opts...)
		if err != nil {
			log.Printf("memsh serve: shell init error: %v", err)
			writeJSON(w, http.StatusInternalServerError, runResponse{Error: "internal server error"})
			return
		}
		defer sh.Close()

		runErr := sh.Run(ctx, req.Script)
		cwd := sh.Cwd()

		if sessionID != "" {
			store.updateCwd(sessionID, cwd)
		}

		output := out.String()
		pager := false
		if strings.HasPrefix(output, native.PagerSentinel) {
			output = output[len(native.PagerSentinel):]
			pager = true
		}
		resp := runResponse{Output: output, Pager: pager, Cwd: cwd}
		if runErr != nil && !errors.Is(runErr, shell.ErrExit) {
			if errors.Is(runErr, context.DeadlineExceeded) {
				resp.Error = "timeout: execution exceeded " + timeout.String()
			} else {
				resp.Error = runErr.Error()
			}
		}
		writeJSON(w, http.StatusOK, resp)
	})

	// ── GET /sessions ──────────────────────────────────────────────────────
	mux.HandleFunc("GET /sessions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, store.list())
	})

	// ── DELETE /session/{id} ───────────────────────────────────────────────
	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, r *http.Request) {
		store.delete(r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})

	// ── GET /health ────────────────────────────────────────────────────────
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{
			Status:   "ok",
			Uptime:   time.Since(start).Round(time.Second).String(),
			Sessions: store.count(),
		})
	})

	// Build the middleware chain.
	var handler http.Handler = mux

	// API key authentication (applied to all endpoints except GET / and GET /health).
	if apiKey != "" {
		handler = apiKeyMiddleware(handler, apiKey)
	}

	// Security headers (CSP, etc.).
	handler = securityHeadersMiddleware(handler)

	// CORS (only if an explicit origin is configured).
	if corsOrigin != "" {
		handler = corsMiddleware(handler, corsOrigin)
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown on SIGINT / SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nmemsh serve: shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	fmt.Fprintf(os.Stderr, "memsh serve: listening on %s  (session TTL %s, max sessions %d, timeout %s)\n",
		addr, ttl, maxSessions, timeout)

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("memsh serve: %w", err)
	}
	return nil
}

// buildShellOpts converts the loaded config into shell options.
func buildShellOpts(cfg Config) []shell.Option {
	var opts []shell.Option
	if !cfg.Shell.WASM {
		opts = append(opts, shell.WithWASMEnabled(false))
	}
	if len(cfg.Plugins.WASM) > 0 {
		opts = append(opts, shell.WithPluginFilter(cfg.Plugins.WASM))
	}
	if len(cfg.Plugins.Disable) > 0 {
		opts = append(opts, shell.WithDisabledPlugins(cfg.Plugins.Disable...))
	}
	return opts
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// apiKeyMiddleware enforces Bearer token authentication on all endpoints except
// GET / (web terminal static page) and GET /health (status check).
func apiKeyMiddleware(next http.Handler, key string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow unauthenticated access to the static UI and health check.
		if r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "/health") {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
			return
		}

		token := authHeader[len(prefix):]
		if subtle.ConstantTimeCompare([]byte(token), []byte(key)) != 1 {
			http.Error(w, `{"error":"invalid API key"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// securityHeadersMiddleware adds Content-Security-Policy and other security
// headers to all responses.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; font-src 'self' https://fonts.googleapis.com https://fonts.gstatic.com")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers for an explicit allowed origin.
func corsMiddleware(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Session-ID, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// generateAPIKey creates a cryptographically random 32-byte hex-encoded API key.
func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func init() {
	serveCmd.Flags().StringP("addr", "a", "127.0.0.1:8080", "Address to listen on (default binds to localhost only)")
	serveCmd.Flags().String("session-ttl", "30m", "Idle TTL for sessions")
	serveCmd.Flags().Duration("timeout", 30*time.Second, "Per-request execution timeout (minimum 5s)")
	serveCmd.Flags().String("cors-origin", "", "Allowed CORS origin (e.g. 'https://example.com'). Empty = no CORS headers.")
	serveCmd.Flags().String("api-key", "", "API key for authentication. When set, mutating endpoints require 'Authorization: Bearer <key>'.")
	serveCmd.Flags().Int("max-sessions", 100, "Maximum number of concurrent sessions (0 = unlimited)")
	rootCmd.AddCommand(serveCmd)
}
