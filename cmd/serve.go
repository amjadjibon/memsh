package cmd

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/amjadjibon/memsh/web"
)

// runRequest is the JSON body accepted by POST /run.
type runRequest struct {
	Script string `json:"script"`
}

// runResponse is the JSON body returned by POST /run.
type runResponse struct {
	Output string `json:"output"`
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
	mu      sync.Mutex
	entries map[string]*sessionEntry
	ttl     time.Duration
}

func newSessionStore(ttl time.Duration) *sessionStore {
	st := &sessionStore{
		entries: make(map[string]*sessionEntry),
		ttl:     ttl,
	}
	go st.reap()
	return st
}

// get returns an existing session entry or creates one.
func (st *sessionStore) get(id string) *sessionEntry {
	st.mu.Lock()
	defer st.mu.Unlock()
	if e, ok := st.entries[id]; ok {
		e.lastUse = time.Now()
		return e
	}
	now := time.Now()
	e := &sessionEntry{
		fs:        afero.NewMemMapFs(),
		cwd:       "/",
		createdAt: now,
		lastUse:   now,
	}
	st.entries[id] = e
	return e
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
the idle TTL (--session-ttl).`,
	RunE: runServe,
}

func runServe(cmd *cobra.Command, _ []string) error {
	addr, _ := cmd.Flags().GetString("addr")
	ttlStr, _ := cmd.Flags().GetString("session-ttl")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	corsEnabled, _ := cmd.Flags().GetBool("cors")

	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return fmt.Errorf("invalid --session-ttl: %w", err)
	}

	cfg, _ := loadConfig()
	baseOpts := buildShellOpts(cfg)

	store := newSessionStore(ttl)
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
		var req runRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, runResponse{Error: "invalid JSON: " + err.Error()})
			return
		}
		req.Script = strings.TrimSpace(req.Script)
		if req.Script == "" {
			writeJSON(w, http.StatusBadRequest, runResponse{Error: "script is required"})
			return
		}

		ctx := r.Context()
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		var out strings.Builder
		opts := make([]shell.Option, len(baseOpts)+1)
		copy(opts, baseOpts)
		opts[len(baseOpts)] = shell.WithStdIO(strings.NewReader(""), &out, &out)

		sessionID := r.Header.Get("X-Session-ID")
		if sessionID != "" {
			entry := store.get(sessionID)
			opts = append(opts, shell.WithFS(entry.fs), shell.WithCwd(entry.cwd))
		}

		sh, err := shell.New(opts...)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, runResponse{Error: "failed to start shell: " + err.Error()})
			return
		}
		defer sh.Close()

		runErr := sh.Run(ctx, req.Script)
		cwd := sh.Cwd()

		if sessionID != "" {
			store.updateCwd(sessionID, cwd)
		}

		resp := runResponse{Output: out.String(), Cwd: cwd}
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

	var handler http.Handler = mux
	if corsEnabled {
		handler = corsMiddleware(mux)
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

	fmt.Fprintf(os.Stderr, "memsh serve: listening on %s  (session TTL %s)\n", addr, ttl)

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

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Session-ID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func init() {
	serveCmd.Flags().StringP("addr", "a", ":8080", "Address to listen on")
	serveCmd.Flags().String("session-ttl", "30m", "Idle TTL for sessions")
	serveCmd.Flags().Duration("timeout", 30*time.Second, "Per-request execution timeout (0 = no timeout)")
	serveCmd.Flags().Bool("cors", false, "Add CORS headers (Access-Control-Allow-Origin: *)")
	rootCmd.AddCommand(serveCmd)
}
