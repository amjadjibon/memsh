package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	internalmcp "github.com/amjadjibon/memsh/internal/mcp"
	"github.com/amjadjibon/memsh/internal/server"
	"github.com/amjadjibon/memsh/pkg/shell"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run an MCP server (stdio, HTTP, or SSE)",
	Long: `Starts a Model Context Protocol server exposing a sandboxed "memsh" bash tool.

Transports:

  stdio (default)
    Reads/writes JSON-RPC over stdin/stdout. Use with Claude Desktop or
    claude mcp add.

      memsh mcp
      memsh mcp --transport stdio

  http  (Streamable HTTP — MCP 2025-03-26+)
    Each POST creates or resumes a session. Sessions are isolated: every
    client connection gets its own in-memory filesystem.

      memsh mcp --transport http                       # 127.0.0.1:8080
      memsh mcp --transport http --addr :8080 --api-key "$KEY"

  sse  (HTTP+SSE — MCP 2024-11-05 legacy)
    Client opens a long-lived GET for server→client events and POSTs
    messages to the returned endpoint URL. Each SSE connection is its own
    isolated session.

      memsh mcp --transport sse --api-key "$KEY"

  http and sse bind to localhost by default. Clients send
  "Authorization: Bearer <key>" when --api-key is set. Each session gets its
  own shell; --max-sessions and --session-ttl bound how many stay alive.
  Outbound network follows the --net-* flags and denies private/loopback
  ranges by default.

Configure stdio in Claude Desktop (claude_desktop_config.json):

    {
      "mcpServers": {
        "memsh": {
          "command": "/usr/local/bin/memsh",
          "args": ["mcp"]
        }
      }
    }

Register stdio with Claude Code:

    claude mcp add memsh -- memsh mcp`,
	RunE: runMCP,
}

// mcpMaxRequestBody caps a single MCP JSON-RPC request body.
const mcpMaxRequestBody = 1 << 20 // 1 MiB

var mcpNetFlags networkFlagConfig

// mcpConfig carries the resolved flags for one `memsh mcp` invocation.
type mcpConfig struct {
	transport   string
	addr        string
	timeout     time.Duration
	wasmEnabled bool
	apiKey      string
	maxSessions int
	sessionTTL  time.Duration
	shellOpts   []shell.Option
}

func runMCP(cmd *cobra.Command, _ []string) error {
	c := mcpConfig{}
	c.transport, _ = cmd.Flags().GetString("transport")
	c.addr, _ = cmd.Flags().GetString("addr")
	c.timeout, _ = cmd.Flags().GetDuration("timeout")
	c.wasmEnabled, _ = cmd.Flags().GetBool("wasm")
	c.apiKey, _ = cmd.Flags().GetString("api-key")
	c.maxSessions, _ = cmd.Flags().GetInt("max-sessions")
	c.sessionTTL, _ = cmd.Flags().GetDuration("session-ttl")
	c.apiKey = strings.TrimSpace(c.apiKey)

	if c.timeout < 5*time.Second {
		c.timeout = 5 * time.Second
	}
	if c.maxSessions < 0 {
		return fmt.Errorf("invalid --max-sessions: must be >= 0")
	}

	// All log output goes to stderr so it never corrupts the stdio transport.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("memsh-mcp: ")

	opts, err := mcpShellOpts(c)
	if err != nil {
		return err
	}
	c.shellOpts = opts

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch c.transport {
	case "stdio", "":
		return runStdio(ctx, c)
	case "http", "sse":
		if !isLoopbackAddr(c.addr) && c.apiKey == "" {
			log.Printf("WARNING: listening on a non-loopback address without --api-key; anyone who can reach %s can run commands", c.addr)
		}
		if c.transport == "http" {
			return runHTTP(ctx, c)
		}
		return runSSE(ctx, c)
	default:
		return fmt.Errorf("unknown transport %q: must be stdio, http, or sse", c.transport)
	}
}

// mcpShellOpts builds the options shared by every MCP shell: no host env,
// the --net-* policy (private ranges denied by default), and the gated
// interpreters only when explicitly enabled.
func mcpShellOpts(c mcpConfig) ([]shell.Option, error) {
	netPolicy, err := parseNetworkPolicy(mcpNetFlags)
	if err != nil {
		return nil, err
	}
	netLimits, err := parseNetworkLimits(mcpNetFlags)
	if err != nil {
		return nil, err
	}
	opts := []shell.Option{
		shell.WithInheritEnv(false),
		shell.WithWASMEnabled(c.wasmEnabled),
		shell.WithNetworkPolicy(netPolicy),
		shell.WithNetworkLimits(netLimits),
	}

	cfg, _ := loadConfig()
	if cfg.Shell.UnsafeInterpreters || unsafeInterps {
		// Over HTTP/SSE every client could run host commands; require auth.
		if c.transport != "stdio" && c.transport != "" && c.apiKey == "" {
			return nil, fmt.Errorf("memsh mcp: --unsafe-interpreters with --transport %s requires --api-key (%s can run host commands)",
				c.transport, strings.Join(shell.GatedInterpreterNames(), ", "))
		}
		log.Printf("WARNING: unsafe interpreters enabled (%s); clients can run real OS commands as this user",
			strings.Join(shell.GatedInterpreterNames(), ", "))
		opts = append(opts, shell.WithUnsafeInterpreters(true))
	}
	if len(cfg.Plugins.Disable) > 0 {
		opts = append(opts, shell.WithDisabledPlugins(cfg.Plugins.Disable...))
	}
	return opts, nil
}

// runStdio runs the MCP server over stdin/stdout (one session, one shell).
func runStdio(ctx context.Context, c mcpConfig) error {
	var buf strings.Builder
	opts := append(append([]shell.Option{}, c.shellOpts...), shell.WithStdIO(strings.NewReader(""), &buf, &buf))
	sh, err := shell.New(opts...)
	if err != nil {
		return fmt.Errorf("mcp: shell init: %w", err)
	}
	defer sh.Close()

	srv := internalmcp.NewServer(sh, &buf, c.timeout)
	log.Printf("stdio started (timeout=%s wasm=%v)", c.timeout, c.wasmEnabled)

	err = srv.Run(ctx, &mcp.StdioTransport{})
	// Clean disconnect (client closed stdin) is not an error.
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		if strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "server is closing") {
			return nil
		}
		return err
	}
	return nil
}

// runHTTP serves MCP over Streamable HTTP (MCP spec 2025-03-26+).
// Each new client session gets its own isolated shell.
func runHTTP(ctx context.Context, c mcpConfig) error {
	pool := internalmcp.NewPool(c.maxSessions, c.timeout, c.shellOpts...)
	go pool.Run(ctx, 30*time.Second)

	handler := mcp.NewStreamableHTTPHandler(pool.GetServer, &mcp.StreamableHTTPOptions{
		EventStore:          mcp.NewMemoryEventStore(nil), // enables stream resumption
		SessionTimeout:      c.sessionTTL,
		MaxRequestBodyBytes: mcpMaxRequestBody,
	})
	return serveHTTP(ctx, c, mcpHandler(handler, c.apiKey))
}

// runSSE serves MCP over HTTP+SSE (MCP spec 2024-11-05 legacy).
// Each SSE connection gets its own isolated shell.
func runSSE(ctx context.Context, c mcpConfig) error {
	pool := internalmcp.NewPool(c.maxSessions, c.timeout, c.shellOpts...)
	go pool.Run(ctx, 30*time.Second)

	handler := mcp.NewSSEHandler(pool.GetServer, nil)
	return serveHTTP(ctx, c, mcpHandler(http.MaxBytesHandler(handler, mcpMaxRequestBody), c.apiKey))
}

// mcpHandler adds bearer-token auth when an API key is configured.
func mcpHandler(h http.Handler, apiKey string) http.Handler {
	if apiKey == "" {
		return h
	}
	return server.APIKeyMiddleware(h, apiKey)
}

// serveHTTP starts an HTTP server and shuts it down gracefully on ctx cancel.
func serveHTTP(ctx context.Context, c mcpConfig, handler http.Handler) error {
	mux := http.NewServeMux()
	mux.Handle("/", handler)

	srv := &http.Server{
		Addr:              c.addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // streaming responses must not have a write deadline
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("%s started on %s (timeout=%s wasm=%v auth=%v max-sessions=%d)",
			c.transport, c.addr, c.timeout, c.wasmEnabled, c.apiKey != "", c.maxSessions)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

func init() {
	mcpCmd.Flags().String("transport", "stdio", "Transport: stdio, http, or sse")
	mcpCmd.Flags().String("addr", "127.0.0.1:8080", "Listen address for http and sse transports (default binds to localhost only)")
	mcpCmd.Flags().Duration("timeout", 30*time.Second, "Per-tool-call execution timeout (minimum 5s)")
	mcpCmd.Flags().Bool("wasm", false, "Enable WASM plugin loading (slower startup)")
	mcpCmd.Flags().String("api-key", "", "Require 'Authorization: Bearer <key>' on http and sse transports")
	mcpCmd.Flags().Int("max-sessions", 100, "Maximum concurrent http/sse sessions, each with its own shell (0 = unlimited)")
	mcpCmd.Flags().Duration("session-ttl", 30*time.Minute, "Close http sessions idle for this long (0 = never)")
	addNetworkFlags(mcpCmd.Flags(), &mcpNetFlags)
	rootCmd.AddCommand(mcpCmd)
}
