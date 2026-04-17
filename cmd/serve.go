package cmd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	internalserver "github.com/amjadjibon/memsh/internal/server"
	"github.com/amjadjibon/memsh/internal/session"
	internalssh "github.com/amjadjibon/memsh/internal/ssh"
	"github.com/amjadjibon/memsh/pkg/network"
	"github.com/amjadjibon/memsh/pkg/shell"
)

// minTimeout is the minimum enforced per-request timeout even if --timeout=0.
const minTimeout = 5 * time.Second

var serveNetFlags networkFlagConfig

func newSessionStore(ttl time.Duration, maxEntries int, persistDir string) (*session.Store, error) {
	ctx := context.Background()
	var st *session.Store
	var err error
	if strings.TrimSpace(persistDir) != "" {
		st, err = session.NewPersistent(ctx, ttl, maxEntries, persistDir)
	} else {
		st = session.New(ctx, ttl, maxEntries)
	}
	if err != nil {
		return nil, err
	}
	return st, nil
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

Durability:
	Pass --session-store <dir> to persist each session to disk and restore it on
	server restart.

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
	maxSessionFiles, _ := cmd.Flags().GetInt("session-max-files")
	maxSessionBytes, _ := cmd.Flags().GetInt64("session-max-bytes")
	maxSessionRuntime, _ := cmd.Flags().GetDuration("session-max-runtime")
	sessionStorePath, _ := cmd.Flags().GetString("session-store")
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
	if maxSessionFiles < 0 {
		return fmt.Errorf("invalid --session-max-files: must be >= 0")
	}
	if maxSessionBytes < 0 {
		return fmt.Errorf("invalid --session-max-bytes: must be >= 0")
	}
	if maxSessionRuntime < 0 {
		return fmt.Errorf("invalid --session-max-runtime: must be >= 0")
	}
	limits := session.Limits{
		MaxFiles:   maxSessionFiles,
		MaxBytes:   maxSessionBytes,
		MaxRuntime: maxSessionRuntime,
	}

	// Only enable auth if an API key was explicitly provided.
	if cmd.Flags().Changed("api-key") && apiKey != "" {
		fmt.Fprintln(os.Stderr, "memsh serve: API key authentication enabled")
	}

	cfg, _ := loadConfig()
	baseOpts := buildShellOpts(cfg)
	netPolicy, err := parseNetworkPolicy(serveNetFlags)
	if err != nil {
		return err
	}
	baseOpts = append(baseOpts, shell.WithNetworkPolicy(netPolicy))

	// In server mode, do not inherit the host process's environment
	// to prevent leaking secrets (API keys, DB URLs, etc.) to remote users.
	baseOpts = append(baseOpts, shell.WithInheritEnv(false))

	store, err := newSessionStore(ttl, maxSessions, sessionStorePath)
	if err != nil {
		return err
	}
	srv, err := internalserver.New(internalserver.Config{
		Addr:         addr,
		TTL:          ttl,
		Timeout:      timeout,
		CORSOrigin:   corsOrigin,
		APIKey:       apiKey,
		MaxSessions:  maxSessions,
		SessionStore: store,
		BaseOpts:     baseOpts,
		Limits:       limits,
	})
	if err != nil {
		return fmt.Errorf("memsh serve: %w", err)
	}

	// Graceful shutdown on SIGINT / SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start SSH server if requested.
	var sshSrv *internalssh.Server
	if sshEnabled {
		var sshErr error
		sshSrv, sshErr = internalssh.New(internalssh.Config{
			Addr:        sshAddr,
			APIKey:      apiKey,
			HostKeyFile: sshHostKey,
			Store:       store,
			BaseOpts:    baseOpts,
			Timeout:     timeout,
			MinTimeout:  minTimeout,
			Limits:      limits,
		})
		if sshErr != nil {
			return fmt.Errorf("memsh serve: SSH: %w", sshErr)
		}
		go func() {
			fmt.Fprintf(os.Stderr, "memsh serve: SSH listening on %s\n", sshAddr)
			if err := sshSrv.ListenAndServe(); err != nil && !errors.Is(err, internalssh.ErrServerClosed) {
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
	if sessionStorePath != "" {
		fmt.Fprintf(os.Stderr, "memsh serve: durable session store: %s\n", sessionStorePath)
	}
	if limits.MaxFiles > 0 || limits.MaxBytes > 0 || limits.MaxRuntime > 0 {
		fmt.Fprintf(os.Stderr, "memsh serve: session limits active (max-files=%d, max-bytes=%d, max-runtime=%s)\n",
			limits.MaxFiles, limits.MaxBytes, limits.MaxRuntime)
	}
	if netPolicy.Mode != network.ModeFull || len(netPolicy.AllowDomains) > 0 || len(netPolicy.AllowCIDRs) > 0 || len(netPolicy.AllowPorts) > 0 {
		fmt.Fprintf(os.Stderr, "memsh serve: network policy active (mode=%s domains=%d cidrs=%d ports=%d)\n",
			netPolicy.Mode, len(netPolicy.AllowDomains), len(netPolicy.AllowCIDRs), len(netPolicy.AllowPorts))
	}

	// Start the cron scheduler. It aligns to the next minute boundary and then
	// ticks every minute, running matching jobs for all active sessions.
	cronCtx, cronCancel := context.WithCancel(context.Background())
	defer cronCancel()
	go session.StartScheduler(cronCtx, store, baseOpts, timeout)

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

func init() {
	serveCmd.Flags().StringP("addr", "a", "127.0.0.1:8080", "Address to listen on (default binds to localhost only)")
	serveCmd.Flags().String("session-ttl", "30m", "Idle TTL for sessions")
	serveCmd.Flags().Duration("timeout", 30*time.Second, "Per-request execution timeout (minimum 5s)")
	serveCmd.Flags().String("cors-origin", "", "Allowed CORS origin (e.g. 'https://example.com'). Empty = no CORS headers.")
	serveCmd.Flags().String("api-key", "", "API key for authentication. When set, mutating endpoints require 'Authorization: Bearer <key>'.")
	serveCmd.Flags().Int("max-sessions", 100, "Maximum number of concurrent sessions (0 = unlimited)")
	serveCmd.Flags().String("session-store", "", "Directory path for durable session persistence (empty = in-memory only)")
	serveCmd.Flags().Int("session-max-files", 0, "Maximum files per session filesystem (0 = unlimited)")
	serveCmd.Flags().Int64("session-max-bytes", 0, "Maximum total bytes per session filesystem (0 = unlimited)")
	serveCmd.Flags().Duration("session-max-runtime", 0, "Maximum cumulative command runtime per session (0 = unlimited)")
	serveCmd.Flags().Bool("ssh", false, "Enable SSH server for remote shell access")
	serveCmd.Flags().String("ssh-addr", ":2222", "SSH server listen address (used with --ssh); binds all interfaces so both localhost and 127.0.0.1 work")
	serveCmd.Flags().String("ssh-host-key", "", "Path to persist the SSH host key (default ~/.memsh/ssh_host_key); stable key avoids known_hosts warnings")
	addNetworkFlags(serveCmd.Flags(), &serveNetFlags)
	rootCmd.AddCommand(serveCmd)
}
