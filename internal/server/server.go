// Package server provides HTTP server implementation for memsh,
// including request handlers, middleware, and server configuration.
//
// The server supports:
//   - RESTful API endpoints for shell execution
//   - Session management with persistent virtual filesystems
//   - Tab completion via POST /complete
//   - Session snapshot import/export
//   - Health check endpoint
//   - Web terminal UI
package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/internal/session"
	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/pkg/shell/plugins/native"
	"github.com/amjadjibon/memsh/web"
)

const (
	// MaxRequestBodySize limits the size of incoming JSON request bodies (1 MB).
	MaxRequestBodySize = 1 << 20
	// MinTimeout is the minimum enforced per-request timeout even if --timeout=0.
	MinTimeout = 5 * time.Second
)

// Handler dependencies.
type Handler struct {
	Store     *session.Store
	BaseOpts  []shell.Option
	Timeout   time.Duration
	StartTime time.Time
}

// NewHandler creates a new HTTP handler with the given dependencies.
func NewHandler(store *session.Store, baseOpts []shell.Option, timeout time.Duration) *Handler {
	return &Handler{
		Store:     store,
		BaseOpts:  baseOpts,
		Timeout:   timeout,
		StartTime: time.Now(),
	}
}

// RegisterRoutes registers all HTTP routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// GET / - Web terminal UI
	mux.HandleFunc("GET /", h.handleIndex)

	// POST /run - Execute a script
	mux.HandleFunc("POST /run", h.handleRun)

	// GET /sessions - List all active sessions
	mux.HandleFunc("GET /sessions", h.handleSessionsList)

	// DELETE /session/{id} - Destroy a session
	mux.HandleFunc("DELETE /session/{id}", h.handleSessionDelete)

	// GET /session/{id}/snapshot - Export session snapshot
	mux.HandleFunc("GET /session/{id}/snapshot", h.handleSnapshotGet)

	// POST /session/{id}/snapshot - Import session snapshot
	mux.HandleFunc("POST /session/{id}/snapshot", h.handleSnapshotPost)

	// GET /health - Health check
	mux.HandleFunc("GET /health", h.handleHealth)

	// POST /complete - Tab completion
	mux.HandleFunc("POST /complete", h.handleComplete)
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(web.TerminalHTML)
}

func (h *Handler) handleRun(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)

	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSON(w, http.StatusBadRequest, runResponse{Error: "invalid request body"})
		return
	}
	req.Script = strings.TrimSpace(req.Script)
	if req.Script == "" {
		WriteJSON(w, http.StatusBadRequest, runResponse{Error: "script is required"})
		return
	}

	ctx := r.Context()
	ctx = native.WithPagerMode(ctx)
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, h.Timeout)
	defer cancel()

	var out strings.Builder
	opts := make([]shell.Option, len(h.BaseOpts)+1)
	copy(opts, h.BaseOpts)
	opts[len(h.BaseOpts)] = shell.WithStdIO(strings.NewReader(""), &out, &out)

	sessionID := r.Header.Get("X-Session-ID")
	var entry *session.Entry
	if sessionID != "" {
		var ok bool
		entry, ok = h.Store.Get(sessionID)
		if !ok {
			WriteJSON(w, http.StatusTooManyRequests, runResponse{
				Error: "maximum number of sessions reached",
			})
			return
		}
		opts = append(opts, shell.WithFS(entry.Fs), shell.WithCwd(entry.Cwd))
	}

	sh, err := shell.New(opts...)
	if err != nil {
		log.Printf("memsh serve: shell init error: %v", err)
		WriteJSON(w, http.StatusInternalServerError, runResponse{Error: "internal server error"})
		return
	}
	defer sh.Close()

	// Restore aliases and .memshrc on first/subsequent requests.
	rcLoaded := false
	if entry != nil {
		session.RestoreAliases(ctx, sh, entry.Fs)
		if !entry.RcLoaded {
			_ = sh.LoadMemshrc(ctx)
			rcLoaded = true
		}
	}

	runErr := sh.Run(ctx, req.Script)
	cwd := sh.Cwd()

	if sessionID != "" {
		session.SaveAliases(ctx, sh)
		h.Store.Update(sessionID, cwd, entry.RcLoaded || rcLoaded)
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
			resp.Error = "timeout: execution exceeded " + h.Timeout.String()
		} else {
			resp.Error = runErr.Error()
		}
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleSessionsList(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, h.Store.List())
}

func (h *Handler) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	h.Store.Delete(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleSnapshotGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	entry, ok := h.Store.Get(id)
	if !ok {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	snap, err := shell.TakeSnapshot(entry.Fs, entry.Cwd)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	data, err := shell.MarshalSnapshot(snap)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="memsh-snapshot.json"`)
	_, _ = w.Write(data)
}

func (h *Handler) handleSnapshotPost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20) // 64 MB limit
	data, err := io.ReadAll(r.Body)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	snap, err := shell.UnmarshalSnapshot(data)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	fs, cwd, err := shell.RestoreSnapshot(snap)
	if err != nil {
		WriteJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}

	// "new" means generate a fresh session ID.
	if id == "new" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		id = fmt.Sprintf("%x", b)
	}

	h.Store.Replace(id, fs, cwd)
	WriteJSON(w, http.StatusOK, map[string]string{"session_id": id})
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, healthResponse{
		Status:   "ok",
		Uptime:   time.Since(h.StartTime).Round(time.Second).String(),
		Sessions: h.Store.Count(),
	})
}

func (h *Handler) handleComplete(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)
	var req completeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.Cursor <= 0 {
		req.Cursor = len(req.Input)
	}

	var fs afero.Fs = afero.NewMemMapFs()
	cwd := "/"
	if sessionID := r.Header.Get("X-Session-ID"); sessionID != "" {
		if entry, ok := h.Store.Get(sessionID); ok {
			fs = entry.Fs
			cwd = entry.Cwd
		}
	}

	result := shell.Complete(req.Input, req.Cursor, fs, cwd, shell.DefaultCommands())
	WriteJSON(w, http.StatusOK, result)
}

// Request/response types.

type runRequest struct {
	Script string `json:"script"`
}

type completeRequest struct {
	Input  string `json:"input"`
	Cursor int    `json:"cursor"` // optional; defaults to len(input) when 0
}

type runResponse struct {
	Output string `json:"output"`
	Pager  bool   `json:"pager,omitempty"`
	Cwd    string `json:"cwd"`
	Error  string `json:"error,omitempty"`
}

type healthResponse struct {
	Status   string `json:"status"`
	Uptime   string `json:"uptime"`
	Sessions int    `json:"sessions"`
}
