package cmd

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	gossh "golang.org/x/crypto/ssh"

	"github.com/amjadjibon/memsh/pkg/cron"
	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/pkg/shell/plugins/native"
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

// completeRequest is the JSON body accepted by POST /complete.
type completeRequest struct {
	Input  string `json:"input"`
	Cursor int    `json:"cursor"` // optional; defaults to len(input) when 0
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
	rcLoaded  bool       // true after /.memshrc has been sourced for this session
	cronMu    sync.Mutex // serialises concurrent cron job writes to /.cron_log
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

// updateSession records the cwd after a request finishes so the next request
// in the same session picks it up. Alias state is persisted in the virtual FS
// via /.memsh_session_aliases, so it does not need a separate field here.
func (st *sessionStore) updateSession(id, cwd string, rcLoaded bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if e, ok := st.entries[id]; ok {
		e.cwd = cwd
		if rcLoaded {
			e.rcLoaded = true
		}
	}
}

// aliasesFile is the virtual-FS path used to persist aliases across requests.
const aliasesFile = "/.memsh_session_aliases"

// saveAliases writes the current alias table to aliasesFile in the virtual FS
// so the next shell can restore it via source.
func saveAliases(ctx context.Context, sh *shell.Shell) {
	_ = sh.Run(ctx, "alias > "+aliasesFile)
}

// restoreAliases sources aliasesFile if it exists.
func restoreAliases(ctx context.Context, sh *shell.Shell, fs afero.Fs) {
	if ok, _ := afero.Exists(fs, aliasesFile); ok {
		_ = sh.Run(ctx, "source "+aliasesFile)
	}
}

// replace creates or overwrites a session with the given filesystem and cwd.
// Used by the snapshot import endpoint.
func (st *sessionStore) replace(id string, fs afero.Fs, cwd string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now()
	if e, ok := st.entries[id]; ok {
		e.fs = fs
		e.cwd = cwd
		e.rcLoaded = false
		e.lastUse = now
		return
	}
	st.entries[id] = &sessionEntry{
		fs:        fs,
		cwd:       cwd,
		createdAt: now,
		lastUse:   now,
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

// sessionSnap is a lightweight copy of a session's key fields for use by the
// cron scheduler without holding the store lock during job execution.
type sessionSnap struct {
	id     string
	fs     afero.Fs
	cwd    string
	cronMu *sync.Mutex
}

// snapshot returns a point-in-time copy of all active sessions.
func (st *sessionStore) snapshot() []sessionSnap {
	st.mu.Lock()
	defer st.mu.Unlock()
	result := make([]sessionSnap, 0, len(st.entries))
	for id, e := range st.entries {
		result = append(result, sessionSnap{
			id:     id,
			fs:     e.fs,
			cwd:    e.cwd,
			cronMu: &e.cronMu,
		})
	}
	return result
}

// startCronScheduler fires cron jobs for every active session once per minute.
// It aligns to the next minute boundary before starting the ticker so that
// CronMatches is evaluated at a consistent wall-clock minute. ctx should be
// cancelled when the server shuts down.
func startCronScheduler(ctx context.Context, store *sessionStore, baseOpts []shell.Option, timeout time.Duration) {
	// Align to the next minute boundary.
	now := time.Now()
	next := now.Truncate(time.Minute).Add(time.Minute)
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Until(next)):
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			for _, ss := range store.snapshot() {
				go runSessionCronJobs(ctx, t, ss, baseOpts, timeout)
			}
		}
	}
}

// runSessionCronJobs reads /.crontab from ss.fs, parses it, and runs any jobs
// whose cron expression matches the given time t.
func runSessionCronJobs(ctx context.Context, t time.Time, ss sessionSnap, baseOpts []shell.Option, timeout time.Duration) {
	data, err := afero.ReadFile(ss.fs, cron.CrontabFile)
	if err != nil {
		// No crontab installed for this session — nothing to do.
		return
	}
	jobs, err := cron.ParseCrontab(string(data))
	if err != nil {
		log.Printf("cron: session %s: parse error: %v", ss.id, err)
		return
	}
	for _, job := range jobs {
		if cron.CronMatches(job.Expr, t) {
			runCronJob(ctx, t, job.Command, ss, baseOpts, timeout)
		}
	}
}

// runCronJob runs a single cron job command inside the session's virtual FS and
// appends a timestamped log entry (including output) to /.cron_log.
func runCronJob(ctx context.Context, t time.Time, command string, ss sessionSnap, baseOpts []shell.Option, timeout time.Duration) {
	var out strings.Builder

	opts := make([]shell.Option, len(baseOpts)+3)
	copy(opts, baseOpts)
	opts[len(baseOpts)] = shell.WithFS(ss.fs)
	opts[len(baseOpts)+1] = shell.WithCwd(ss.cwd)
	opts[len(baseOpts)+2] = shell.WithStdIO(strings.NewReader(""), &out, &out)

	sh, err := shell.New(opts...)
	if err != nil {
		log.Printf("cron: session %s: shell init: %v", ss.id, err)
		return
	}
	defer sh.Close()

	jobCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	runErr := sh.Run(jobCtx, command)

	// Build the log entry regardless of error so users can see when jobs ran.
	stamp := t.Format("2006-01-02 15:04")
	entry := fmt.Sprintf("[%s] %s\n%s\n", stamp, command, out.String())
	if runErr != nil && !errors.Is(runErr, shell.ErrExit) {
		entry += fmt.Sprintf("# error: %v\n", runErr)
	}

	// Serialise log writes for this session.
	ss.cronMu.Lock()
	defer ss.cronMu.Unlock()

	existing, _ := afero.ReadFile(ss.fs, cron.CronLogFile)
	updated := append(existing, []byte(entry)...)
	if writeErr := afero.WriteFile(ss.fs, cron.CronLogFile, updated, 0o644); writeErr != nil {
		log.Printf("cron: session %s: write log: %v", ss.id, writeErr)
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
	sshEnabled, _ := cmd.Flags().GetBool("ssh")
	sshAddr, _ := cmd.Flags().GetString("ssh-addr")
	sshHostKey, _ := cmd.Flags().GetString("ssh-host-key")

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
		var entry *sessionEntry
		if sessionID != "" {
			var ok bool
			entry, ok = store.get(sessionID)
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

		// Restore aliases and .memshrc on first/subsequent requests.
		rcLoaded := false
		if entry != nil {
			restoreAliases(ctx, sh, entry.fs)
			if !entry.rcLoaded {
				_ = sh.LoadMemshrc(ctx)
				rcLoaded = true
			}
		}

		runErr := sh.Run(ctx, req.Script)
		cwd := sh.Cwd()

		if sessionID != "" {
			saveAliases(ctx, sh)
			store.updateSession(sessionID, cwd, entry.rcLoaded || rcLoaded)
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

	// ── GET /session/{id}/snapshot ─────────────────────────────────────────
	// Export the full virtual filesystem + cwd of a session as a JSON snapshot.
	// The response is served as an attachment so browsers download it directly.
	mux.HandleFunc("GET /session/{id}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		entry, ok := store.get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		snap, err := shell.TakeSnapshot(entry.fs, entry.cwd)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		data, err := shell.MarshalSnapshot(snap)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="memsh-snapshot.json"`)
		_, _ = w.Write(data)
	})

	// ── POST /session/{id}/snapshot ────────────────────────────────────────
	// Import a JSON snapshot into an existing or new session.
	// The session's virtual filesystem and cwd are completely replaced.
	// If the id is "new", a fresh session ID is generated and returned.
	mux.HandleFunc("POST /session/{id}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		r.Body = http.MaxBytesReader(w, r.Body, 64<<20) // 64 MB limit
		data, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
			return
		}
		snap, err := shell.UnmarshalSnapshot(data)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		fs, cwd, err := shell.RestoreSnapshot(snap)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}

		// "new" means generate a fresh session ID.
		if id == "new" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = fmt.Sprintf("%x", b)
		}

		store.replace(id, fs, cwd)
		writeJSON(w, http.StatusOK, map[string]string{"session_id": id})
	})

	// ── GET /health ────────────────────────────────────────────────────────
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{
			Status:   "ok",
			Uptime:   time.Since(start).Round(time.Second).String(),
			Sessions: store.count(),
		})
	})

	// ── POST /complete ─────────────────────────────────────────────────────
	// Returns tab-completion candidates for the given input and cursor position.
	// Uses the session's virtual FS (if X-Session-ID is provided) for path
	// completions, and the static default command list for command completions.
	mux.HandleFunc("POST /complete", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		var req completeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if req.Cursor <= 0 {
			req.Cursor = len(req.Input)
		}

		fs := afero.NewMemMapFs()
		cwd := "/"
		if sessionID := r.Header.Get("X-Session-ID"); sessionID != "" {
			if entry, ok := store.get(sessionID); ok {
				fs = entry.fs
				cwd = entry.cwd
			}
		}

		result := shell.Complete(req.Input, req.Cursor, fs, cwd, shell.DefaultCommands())
		writeJSON(w, http.StatusOK, result)
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

	// Start SSH server if requested.
	var sshSrv *gliderssh.Server
	if sshEnabled {
		var sshErr error
		sshSrv, sshErr = buildSSHServer(sshAddr, apiKey, sshHostKey, store, baseOpts, timeout)
		if sshErr != nil {
			return fmt.Errorf("memsh serve: SSH: %w", sshErr)
		}
		go func() {
			fmt.Fprintf(os.Stderr, "memsh serve: SSH listening on %s\n", sshAddr)
			if err := sshSrv.ListenAndServe(); err != nil && !errors.Is(err, gliderssh.ErrServerClosed) {
				log.Printf("memsh serve: SSH: %v", err)
			}
		}()
	}

	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nmemsh serve: shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		if sshSrv != nil {
			_ = sshSrv.Close()
		}
	}()

	fmt.Fprintf(os.Stderr, "memsh serve: listening on %s  (session TTL %s, max sessions %d, timeout %s)\n",
		addr, ttl, maxSessions, timeout)

	// Start the cron scheduler. It aligns to the next minute boundary and then
	// ticks every minute, running matching jobs for all active sessions.
	cronCtx, cronCancel := context.WithCancel(context.Background())
	defer cronCancel()
	go startCronScheduler(cronCtx, store, baseOpts, timeout)

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("memsh serve: %w", err)
	}
	return nil
}

// buildSSHServer creates a gliderlabs SSH server backed by the memsh session
// store.  Each connection is identified by its SSH username, which is used as
// the session ID so that the virtual filesystem persists across reconnects.
func buildSSHServer(addr, apiKey, hostKeyFile string, store *sessionStore, baseOpts []shell.Option, timeout time.Duration) (*gliderssh.Server, error) {
	signer, err := loadOrGenerateHostKey(hostKeyFile)
	if err != nil {
		return nil, err
	}

	srv := &gliderssh.Server{
		Addr:        addr,
		HostSigners: []gliderssh.Signer{signer},
		Handler: func(s gliderssh.Session) {
			handleSSHSession(s, store, baseOpts, timeout)
		},
	}

	if apiKey != "" {
		// Require password == API key.
		srv.PasswordHandler = func(_ gliderssh.Context, password string) bool {
			return subtle.ConstantTimeCompare([]byte(password), []byte(apiKey)) == 1
		}
	}
	// No API key → leave all auth handlers nil; gliderlabs/ssh will set
	// NoClientAuth = true automatically, so no password prompt is shown.

	return srv, nil
}

// handleSSHSession runs a memsh shell for an incoming SSH connection.
//
// The SSH username is used as the session ID so the virtual FS persists across
// reconnects.  If the client supplies a command (non-interactive mode) it is
// run once and the session exits.  Otherwise an interactive REPL is served.
//
// PTY handling: when the SSH client requests a PTY (as the system ssh(1) does
// for interactive sessions), input arrives one raw byte at a time with no line
// buffering.  sshReadLine reads characters, echoes them, and handles backspace /
// Ctrl-C / Ctrl-D so the user gets a proper editing experience.  Command output
// has bare LF translated to CR+LF so it renders correctly in the remote terminal.
func handleSSHSession(s gliderssh.Session, store *sessionStore, baseOpts []shell.Option, timeout time.Duration) {
	sessionID := s.User()
	if sessionID == "" || sessionID == "memsh" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		sessionID = fmt.Sprintf("%x", b)
	}

	entry, ok := store.get(sessionID)
	if !ok {
		fmt.Fprintln(s.Stderr(), "error: maximum number of sessions reached")
		_ = s.Exit(1)
		return
	}

	cmdArgs := s.Command()

	// ── non-interactive: single command ──────────────────────────────────
	if len(cmdArgs) > 0 {
		script := strings.Join(cmdArgs, " ")

		opts := make([]shell.Option, len(baseOpts), len(baseOpts)+3)
		copy(opts, baseOpts)
		opts = append(opts,
			shell.WithFS(entry.fs),
			shell.WithCwd(entry.cwd),
			shell.WithStdIO(s, s, s.Stderr()),
		)

		sh, err := shell.New(opts...)
		if err != nil {
			fmt.Fprintf(s.Stderr(), "error: %v\n", err)
			_ = s.Exit(1)
			return
		}
		defer sh.Close()

		ctx, cancel := context.WithTimeout(s.Context(), timeout)
		defer cancel()

		restoreAliases(ctx, sh, entry.fs)
		rcLoaded := false
		if !entry.rcLoaded {
			_ = sh.LoadMemshrc(ctx)
			rcLoaded = true
		}
		runErr := sh.Run(ctx, script)
		saveAliases(ctx, sh)
		store.updateSession(sessionID, sh.Cwd(), entry.rcLoaded || rcLoaded)

		if runErr != nil && !errors.Is(runErr, shell.ErrExit) {
			_ = s.Exit(1)
		} else {
			_ = s.Exit(0)
		}
		return
	}

	// ── interactive REPL ─────────────────────────────────────────────────
	// Load .memshrc once at session start for interactive mode.
	cwd := entry.cwd
	if !entry.rcLoaded {
		initOpts := make([]shell.Option, len(baseOpts), len(baseOpts)+3)
		copy(initOpts, baseOpts)
		initOpts = append(initOpts,
			shell.WithFS(entry.fs),
			shell.WithCwd(cwd),
			shell.WithStdIO(strings.NewReader(""), io.Discard, io.Discard),
		)
		if rcSh, rcErr := shell.New(initOpts...); rcErr == nil {
			_ = rcSh.LoadMemshrc(s.Context())
			saveAliases(s.Context(), rcSh)
			rcSh.Close()
		}
		store.updateSession(sessionID, cwd, true)
	}

	for {
		// Print prompt — use CR+LF so the cursor returns to column 0.
		fmt.Fprintf(s, "memsh:%s$ ", cwd)

		line, err := sshReadLine(s)
		if err != nil {
			// io.EOF → client pressed Ctrl-D on empty line or disconnected.
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			fmt.Fprint(s, "logout\r\n")
			break
		}

		var cmdOut strings.Builder
		opts := make([]shell.Option, len(baseOpts), len(baseOpts)+3)
		copy(opts, baseOpts)
		opts = append(opts,
			shell.WithFS(entry.fs),
			shell.WithCwd(cwd),
			shell.WithStdIO(strings.NewReader(""), &cmdOut, &cmdOut),
		)

		sh, shErr := shell.New(opts...)
		if shErr != nil {
			fmt.Fprintf(s, "error: %v\r\n", shErr)
			continue
		}

		ctx, cancel := context.WithTimeout(s.Context(), timeout)
		restoreAliases(ctx, sh, entry.fs)
		runErr := sh.Run(ctx, line)
		saveAliases(ctx, sh)
		cancel()
		newCwd := sh.Cwd()
		sh.Close()

		store.updateSession(sessionID, newCwd, true)
		cwd = newCwd

		// Translate bare LF → CR+LF for the remote terminal.
		output := strings.ReplaceAll(cmdOut.String(), "\r\n", "\n") // normalise first
		output = strings.ReplaceAll(output, "\n", "\r\n")
		fmt.Fprint(s, output)

		if runErr != nil && !errors.Is(runErr, shell.ErrExit) {
			fmt.Fprintf(s, "%v\r\n", runErr)
		}
	}
	_ = s.Exit(0)
}

// sshReadLine reads one line of input from an SSH PTY session.
//
// The PTY delivers raw bytes: Enter sends CR (\r), not LF (\n).  Characters
// must be echoed back manually.  Basic editing is supported:
//   - Backspace / DEL (0x7f): erase last character
//   - Ctrl-U (0x15):          kill entire line
//   - Ctrl-C (0x03):          discard line, return empty string (not an error)
//   - Ctrl-D (0x04):          EOF when the line is empty; ignored otherwise
func sshReadLine(r io.Reader) (string, error) {
	var line []byte
	buf := make([]byte, 1)
	for {
		_, err := r.Read(buf)
		if err != nil {
			return string(line), err
		}
		c := buf[0]
		switch {
		case c == '\r' || c == '\n':
			// Echo CR+LF so the cursor moves to the next line before output appears.
			if w, ok := r.(io.Writer); ok {
				_, _ = w.Write([]byte("\r\n"))
			}
			return string(line), nil
		case c == 127 || c == '\b': // DEL / backspace
			if len(line) > 0 {
				line = line[:len(line)-1]
				// Erase character: move back, print space, move back again.
				if w, ok := r.(io.Writer); ok {
					_, _ = w.Write([]byte("\b \b"))
				}
			}
		case c == 21: // Ctrl-U: kill line
			if len(line) > 0 {
				if w, ok := r.(io.Writer); ok {
					_, _ = w.Write([]byte(strings.Repeat("\b", len(line)) +
						strings.Repeat(" ", len(line)) +
						strings.Repeat("\b", len(line))))
				}
				line = line[:0]
			}
		case c == 3: // Ctrl-C: discard line
			if w, ok := r.(io.Writer); ok {
				_, _ = w.Write([]byte("^C\r\n"))
			}
			return "", nil // empty line — caller reprints prompt
		case c == 4: // Ctrl-D: EOF only on empty line
			if len(line) == 0 {
				return "", io.EOF
			}
		case c >= 32 && c < 127: // printable ASCII
			line = append(line, c)
			if w, ok := r.(io.Writer); ok {
				_, _ = w.Write([]byte{c})
			}
		}
	}
}

// loadOrGenerateHostKey loads a persistent Ed25519 host key from keyFile, or
// generates a new one and saves it if the file does not exist.  A stable host
// key prevents "REMOTE HOST IDENTIFICATION HAS CHANGED" warnings on reconnect.
//
// keyFile defaults to ~/.memsh/ssh_host_key when empty.
func loadOrGenerateHostKey(keyFile string) (gliderssh.Signer, error) {
	if keyFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		keyFile = filepath.Join(home, ".memsh", "ssh_host_key")
	}

	// Try to load an existing key.
	if data, err := os.ReadFile(keyFile); err == nil {
		signer, parseErr := gossh.ParsePrivateKey(data)
		if parseErr == nil {
			return signer, nil
		}
		log.Printf("memsh serve: SSH: could not parse host key %s (%v); generating new key", keyFile, parseErr)
	}

	// Generate a fresh Ed25519 key and persist it.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate host key: %w", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("create signer: %w", err)
	}

	pemBlock, err := gossh.MarshalPrivateKey(priv, "memsh ssh host key")
	if err == nil {
		if mkErr := os.MkdirAll(filepath.Dir(keyFile), 0o700); mkErr == nil {
			_ = os.WriteFile(keyFile, pem.EncodeToMemory(pemBlock), 0o600)
			log.Printf("memsh serve: SSH: host key saved to %s", keyFile)
		}
	}

	return signer, nil
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

func init() {
	serveCmd.Flags().StringP("addr", "a", "127.0.0.1:8080", "Address to listen on (default binds to localhost only)")
	serveCmd.Flags().String("session-ttl", "30m", "Idle TTL for sessions")
	serveCmd.Flags().Duration("timeout", 30*time.Second, "Per-request execution timeout (minimum 5s)")
	serveCmd.Flags().String("cors-origin", "", "Allowed CORS origin (e.g. 'https://example.com'). Empty = no CORS headers.")
	serveCmd.Flags().String("api-key", "", "API key for authentication. When set, mutating endpoints require 'Authorization: Bearer <key>'.")
	serveCmd.Flags().Int("max-sessions", 100, "Maximum number of concurrent sessions (0 = unlimited)")
	serveCmd.Flags().Bool("ssh", false, "Enable SSH server for remote shell access")
	serveCmd.Flags().String("ssh-addr", ":2222", "SSH server listen address (used with --ssh); binds all interfaces so both localhost and 127.0.0.1 work")
	serveCmd.Flags().String("ssh-host-key", "", "Path to persist the SSH host key (default ~/.memsh/ssh_host_key); stable key avoids known_hosts warnings")
	rootCmd.AddCommand(serveCmd)
}
