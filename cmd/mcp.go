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

      memsh mcp --transport http --addr :8080

  sse  (HTTP+SSE — MCP 2024-11-05 legacy)
    Client opens a long-lived GET for server→client events and POSTs
    messages to the returned endpoint URL. Each SSE connection is its own
    isolated session.

      memsh mcp --transport sse --addr :8080

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

func runMCP(cmd *cobra.Command, _ []string) error {
	transport, _ := cmd.Flags().GetString("transport")
	addr, _ := cmd.Flags().GetString("addr")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	wasmEnabled, _ := cmd.Flags().GetBool("wasm")

	if timeout < 5*time.Second {
		timeout = 5 * time.Second
	}

	// All log output goes to stderr so it never corrupts the stdio transport.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("memsh-mcp: ")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch transport {
	case "stdio", "":
		return runStdio(ctx, timeout, wasmEnabled)
	case "http":
		return runHTTP(ctx, addr, timeout, wasmEnabled)
	case "sse":
		return runSSE(ctx, addr, timeout, wasmEnabled)
	default:
		return fmt.Errorf("unknown transport %q: must be stdio, http, or sse", transport)
	}
}

// newShellServer creates a fresh MCP server with its own isolated shell.
// Called once per session for HTTP/SSE transports.
func newShellServer(timeout time.Duration, wasmEnabled bool) *mcp.Server {
	var buf strings.Builder
	sh, err := shell.New(
		shell.WithInheritEnv(false),
		shell.WithWASMEnabled(wasmEnabled),
		shell.WithStdIO(strings.NewReader(""), &buf, &buf),
	)
	if err != nil {
		// Shell init failure is non-recoverable for this session.
		log.Printf("session shell init error: %v", err)
		return internalmcp.NewServer(nil, &buf, timeout)
	}
	return internalmcp.NewServer(sh, &buf, timeout)
}

// runStdio runs the MCP server over stdin/stdout (one session, one shell).
func runStdio(ctx context.Context, timeout time.Duration, wasmEnabled bool) error {
	var buf strings.Builder
	sh, err := shell.New(
		shell.WithInheritEnv(false),
		shell.WithWASMEnabled(wasmEnabled),
		shell.WithStdIO(strings.NewReader(""), &buf, &buf),
	)
	if err != nil {
		return fmt.Errorf("mcp: shell init: %w", err)
	}
	defer sh.Close()

	server := internalmcp.NewServer(sh, &buf, timeout)
	log.Printf("stdio started (timeout=%s wasm=%v)", timeout, wasmEnabled)

	err = server.Run(ctx, &mcp.StdioTransport{})
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
func runHTTP(ctx context.Context, addr string, timeout time.Duration, wasmEnabled bool) error {
	handler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			return newShellServer(timeout, wasmEnabled)
		},
		&mcp.StreamableHTTPOptions{
			EventStore: mcp.NewMemoryEventStore(nil), // enables stream resumption
		},
	)

	return serveHTTP(ctx, addr, "/", handler, "http", timeout, wasmEnabled)
}

// runSSE serves MCP over HTTP+SSE (MCP spec 2024-11-05 legacy).
// Each SSE connection gets its own isolated shell.
func runSSE(ctx context.Context, addr string, timeout time.Duration, wasmEnabled bool) error {
	handler := mcp.NewSSEHandler(
		func(r *http.Request) *mcp.Server {
			return newShellServer(timeout, wasmEnabled)
		},
		nil,
	)

	return serveHTTP(ctx, addr, "/", handler, "sse", timeout, wasmEnabled)
}

// serveHTTP starts an HTTP server and shuts it down gracefully on ctx cancel.
func serveHTTP(ctx context.Context, addr, pattern string, handler http.Handler, transport string, timeout time.Duration, wasmEnabled bool) error {
	mux := http.NewServeMux()
	mux.Handle(pattern, handler)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // streaming responses must not have a write deadline
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("%s started on %s (timeout=%s wasm=%v)", transport, addr, timeout, wasmEnabled)
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
	mcpCmd.Flags().String("addr", ":8080", "Listen address for http and sse transports")
	mcpCmd.Flags().Duration("timeout", 30*time.Second, "Per-tool-call execution timeout (minimum 5s)")
	mcpCmd.Flags().Bool("wasm", false, "Enable WASM plugin loading (slower startup)")
	rootCmd.AddCommand(mcpCmd)
}
