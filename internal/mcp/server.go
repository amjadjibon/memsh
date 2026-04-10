// Package mcp wires memsh's shell to an MCP server using the official Go SDK.
package mcp

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/amjadjibon/memsh/pkg/shell"
)

// bashInput is the typed input for the "bash" tool.
type bashInput struct {
	Command string `json:"command" jsonschema:"The bash command or script to execute"`
}

// NewServer creates an MCP server with a single "bash" tool backed by sh.
// buf must be the same *strings.Builder passed to shell.WithStdIO so that
// output written by the shell can be read after each Run call.
// timeout is applied per tool call; 0 defaults to 30 s.
func NewServer(sh *shell.Shell, buf *strings.Builder, timeout time.Duration) *mcp.Server {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	var mu sync.Mutex // serialises buf access across concurrent tool calls

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "memsh",
		Version: "1.0.0",
	}, nil)

	mcp.AddTool(server,
		&mcp.Tool{
			Name: "memsh",
			Description: "Execute bash commands in a sandboxed in-memory filesystem. " +
				"The filesystem persists across calls within a session — use it as a " +
				"scratch-pad. The real OS filesystem is never touched and no host " +
				"commands can escape the sandbox. " +
				"Stdin is not available; commands that read input receive EOF.",
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, input bashInput) (*mcp.CallToolResult, any, error) {
			if sh == nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "error: shell failed to initialise"}},
					IsError: true,
				}, nil, nil
			}

			callCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			mu.Lock()
			buf.Reset()
			runErr := sh.Run(callCtx, input.Command)
			output := buf.String()
			cwd := sh.Cwd()
			mu.Unlock()

			text := output + "\nCwd: " + cwd

			// ErrExit is normal termination (the script called exit/quit).
			// Treat it as success so the agent is not confused.
			if runErr != nil && !errors.Is(runErr, shell.ErrExit) {
				if errors.Is(runErr, context.DeadlineExceeded) {
					text += "\nerror: timeout after " + timeout.String()
				} else {
					text += "\nerror: " + runErr.Error()
				}
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: text}},
					IsError: true,
				}, nil, nil
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: text}},
			}, nil, nil
		},
	)

	return server
}
