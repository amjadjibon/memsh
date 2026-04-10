---
name: memsh
description: Expert guide for the memsh virtual bash shell — a Go project that runs bash-like commands against an in-memory filesystem with no real OS access. Use this skill whenever you need to: add a new shell command (native plugin), write tests, run or configure the HTTP/SSH/MCP servers, use memsh as an AI sandbox, embed memsh in Go programs, extend WASM plugins, or understand memsh architecture. Trigger on any task involving memsh development, plugin creation, shell sandboxing, AI tool integration, or in-memory filesystem scripting.
---

# memsh Developer Guide

`memsh` is a virtual bash shell in Go. It parses real bash syntax (`mvdan.cc/sh/v3`) and executes commands against an `afero.MemMapFs` — the real OS filesystem is never touched. Every shell command is a Go plugin registered at startup.

---

## Quick reference: memsh binary commands

```bash
# Build
make build              # Build ./bin/memsh
make install            # Install to /usr/local/bin

# Interactive REPL (shell)
memsh                # Start interactive shell
memsh ./script.sh    # Run a script file
echo "ls /" | memsh  # Pipe commands

# Servers
memsh serve                    # HTTP API + web terminal (localhost:8080)
memsh serve --addr :3000       # Custom address
memsh serve --ssh              # Enable SSH server on :2222
memsh serve --api-key xyz      # Require authentication

# MCP server (for AI agents)
memsh mcp                       # stdio transport (Claude Desktop)
memsh mcp --transport http      # HTTP transport
memsh mcp --wasm                # Enable WASM plugins

# Plugin management
memsh plugin list               # List installed plugins
memsh plugin install python     # Install Python 3.12 WASM runtime
memsh plugin install /path/to/plugin.wasm

# Testing
make test              # Run all tests
go test ./tests -v     # Integration tests only

# During development, you can also use:
go run . serve         # Runs without building first
go run . mcp           # MCP without building
```

---

## Architecture in 30 seconds

```
Shell.Run(script)
  → mvdan.cc/sh parses bash syntax into an AST
  → interp.Runner executes AST
       ↓ OpenHandler      → all file I/O goes to afero.MemMapFs
       ↓ ExecHandler      → looks up command:
            s.builtins[cmd]   native Go plugins
            s.plugins[cmd]    WASM plugins via wazero
            blocked           external OS commands (default)
```

Key files:
- `pkg/shell/shell.go` — `Shell` struct, `New()`, exec handler
- `pkg/shell/plugins/plugin.go` — `Plugin`, `PluginInfo`, `ShellContext` interfaces
- `pkg/shell/defaults.go` — `defaultNativePlugins()` registration list
- `pkg/shell/options.go` — all `With*` functional options
- `internal/server/server.go` — HTTP server and route handlers
- `tests/helper.go` — `NewTestShell()` test utility

---

## Adding a native plugin (most common task)

### Step 1 — Create `pkg/shell/plugins/native/<name>.go`

```go
package native

import (
    "context"
    "fmt"

    "mvdan.cc/sh/v3/interp"
    "github.com/amjadjibon/memsh/pkg/shell/plugins"
)

type MyPlugin struct{}

func (MyPlugin) Name() string        { return "mycmd" }
func (MyPlugin) Description() string { return "one-line description" }
func (MyPlugin) Usage() string       { return "mycmd [-f] [args...]" }

func (MyPlugin) Run(ctx context.Context, args []string) error {
    hc := interp.HandlerCtx(ctx) // pipe-aware I/O — always use this
    sc := plugins.ShellCtx(ctx)  // virtual FS, cwd, env

    // Parse flags (see Flag parsing section below)
    // Do work using sc.FS, sc.ResolvePath, sc.Env, etc.
    fmt.Fprintln(hc.Stdout, "hello")
    return nil
}

// Compile-time check that MyPlugin implements PluginInfo
var _ plugins.PluginInfo = MyPlugin{}
```

### Step 2 — Register in `pkg/shell/defaults.go`

Add to the slice returned by `defaultNativePlugins()`:

```go
MyPlugin{},
```

### Step 3 — Write a test in `tests/<name>_test.go`

```go
package tests

import (
    "context"
    "strings"
    "testing"

    "github.com/spf13/afero"
    "github.com/amjadjibon/memsh/pkg/shell"
)

func TestMyPlugin(t *testing.T) {
    ctx := context.Background()
    var buf strings.Builder
    fs := afero.NewMemMapFs()

    s := NewTestShell(t, &buf, shell.WithFS(fs))

    if err := s.Run(ctx, `mycmd`); err != nil {
        t.Fatal(err)
    }

    got := strings.TrimSpace(buf.String())
    if got != "hello" {
        t.Errorf("got %q, want %q", got, "hello")
    }
}
```

---

## ShellContext API

`sc := plugins.ShellCtx(ctx)` gives you:

| Field / Method | What it does |
|---|---|
| `sc.FS` | `afero.Fs` — the virtual in-memory filesystem |
| `sc.Cwd` | Current working directory (always absolute) |
| `sc.ResolvePath(p)` | Converts relative path → absolute virtual path |
| `sc.Env(key)` | Get environment variable |
| `sc.SetEnv(key, val)` | Set environment variable |
| `sc.SetCwd(path)` | Change working directory |
| `sc.Run(ctx, script)` | Execute a script in the current shell |
| `sc.Exec(ctx, args)` | Execute a command through the shell resolver |
| `sc.CommandInfo(name)` | Look up metadata for any registered command |
| `sc.CommandNames()` | List all known command names |

Always use `hc := interp.HandlerCtx(ctx)` for I/O:
- `hc.Stdout` — write output here (works in pipes and redirects)
- `hc.Stdin` — read stdin from here
- `hc.Stderr` — write errors here

Never write to a stored `s.stdout` field — it won't work in pipes.

---

## Flag parsing convention

All native plugins use a combined short-flag loop so `-rf`, `-la` etc. work:

```go
var flagForce, flagRecursive bool
var flagMsg string
var positional []string

endOfFlags := false
for i := 1; i < len(args); i++ {
    a := args[i]
    if endOfFlags || a == "" || a[0] != '-' {
        positional = append(positional, a)
        continue
    }
    if a == "--" { endOfFlags = true; continue }

    // Long flags
    switch a {
    case "--force":     flagForce = true; continue
    case "--message":   i++; flagMsg = args[i]; continue
    }

    // Combined short flags: -rf, -m <val>
    unknown := ""
    for j, c := range a[1:] {
        switch c {
        case 'f': flagForce = true
        case 'r': flagRecursive = true
        case 'm': flagMsg = args[i][2+j:]; i++ // consume next arg if empty
        default:  unknown += string(c)
        }
    }
    if unknown != "" {
        return fmt.Errorf("mycmd: invalid option -- '%s'", unknown)
    }
}
```

---

## Testing patterns

`tests/helper.go` provides `NewTestShell()` — WASM is disabled by default for speed:

```go
// Basic test
var buf strings.Builder
s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
_ = s.Run(ctx, `echo hello`)
out := strings.TrimSpace(buf.String())

// Pre-seed the virtual filesystem
fs := afero.NewMemMapFs()
afero.WriteFile(fs, "/data.json", []byte(`{"name":"alice"}`), 0644)
s := NewTestShell(t, &buf, shell.WithFS(fs))
_ = s.Run(ctx, `jq -r .name /data.json`)

// Test that a file was created
_, err := fs.Stat("/output.txt")
if err != nil { t.Error("file not created") }

// Non-root cwd (mvdan.cc/sh requires a real OS path for cwd validation)
dir := tests.RealTmpDir(t) // wraps os.MkdirTemp, cleaned up by t.Cleanup
s := NewTestShell(t, &buf, shell.WithFS(fs), shell.WithCwd(dir))
```

Run tests:
```bash
go test ./tests -run TestMyPlugin -v    # single plugin
go test ./tests -v                      # all integration tests
go test ./pkg/shell/... -v             # shell package tests
```

---

## HTTP server

Start with `memsh serve` (sessions always enabled, TTL 30m).

| Endpoint | Request | Response |
|---|---|---|
| `GET /` | — | Web terminal UI |
| `POST /run` | `{"script":"ls /"}` | `{"output":"...","cwd":"...","pager":bool,"error":"..."}` |
| `GET /sessions` | — | List active sessions |
| `DELETE /session/{id}` | — | Destroy session |
| `GET /health` | — | `{"status":"ok","uptime":"...","sessions":N}` |
| `GET /session/{id}/snapshot` | — | Export FS as JSON |
| `POST /session/{id}/snapshot` | snapshot JSON | Import FS (use `"new"` as id to create) |

Sessions persist FS state across requests. Send `X-Session-ID: <id>` header:

```bash
# Create a file in one request, see it in the next
curl -s -X POST http://localhost:8080/run \
  -H "X-Session-ID: mysession" \
  -H "Content-Type: application/json" \
  -d '{"script":"echo hello > /tmp/f.txt"}'

curl -s -X POST http://localhost:8080/run \
  -H "X-Session-ID: mysession" \
  -H "Content-Type: application/json" \
  -d '{"script":"cat /tmp/f.txt"}'
```

---

## Creating a Shell programmatically

```go
import (
    "github.com/amjadjibon/memsh/pkg/shell"
    "github.com/spf13/afero"
)

fs := afero.NewMemMapFs()
s, err := shell.New(
    shell.WithFS(fs),
    shell.WithCwd("/"),
    shell.WithEnv(map[string]string{"HOME": "/root"}),
    shell.WithStdIO(os.Stdin, &buf, os.Stderr),
    shell.WithAllowExternalCommands(false), // default
    shell.WithInheritEnv(false),            // isolate from host env
)
defer s.Close()

err = s.Run(ctx, `mkdir /data && echo "hi" > /data/f.txt`)
```

---

## WASM plugins

Standard Go programs compiled for `GOOS=wasip1 GOARCH=wasm`. The virtual FS is mounted at `/` so WASI file I/O goes directly into `afero.MemMapFs`.

Two types:
- **WASI** (exports `_start`): runs like a normal CLI program
- **Custom** (exports `run`): imports `memsh::write_stdout`, `memsh::read_stdin`, `memsh::arg_get`, `memsh::fs_*`, etc.

Install plugins:
```bash
memsh plugin list
memsh plugin install python   # Python 3.12 WASM runtime
memsh plugin install ruby     # Ruby 3.2 WASM runtime
memsh plugin install php      # PHP 8.2 WASM runtime
memsh plugin install /path/to/plugin.wasm
```

Enable WASM in tests (disabled by default for speed):
```go
s := NewTestShell(t, &buf, shell.WithFS(fs), shell.WithWASMEnabled(true))
```

---

## Running memsh as a server

### HTTP server (REST API + Web Terminal)

```bash
# Basic HTTP server (sessions always enabled, 30m TTL)
memsh serve

# Custom address and session TTL
memsh serve --addr :3000 --session-ttl 1h

# Enable CORS for browser access
memsh serve --cors-origin https://example.com

# Require API key authentication
memsh serve --api-key mysecretkey
# Clients then send: Authorization: Bearer mysecretkey

# Set per-request timeout (default 30s, minimum 5s)
memsh serve --timeout 1m
```

The HTTP server exposes a REST API and a browser-based terminal UI. Sessions persist the virtual filesystem across requests — each session ID gets its own isolated `afero.MemMapFs`.

### SSH server (remote shell access)

```bash
# Start both HTTP and SSH servers
memsh serve --ssh --ssh-addr :2222

# SSH with password authentication (uses --api-key as password)
memsh serve --ssh --api-key mypassword --ssh-addr :2222

# Custom SSH host key path (default ~/.memsh/ssh_host_key)
memsh serve --ssh --ssh-host-key /custom/path/host_key
```

Connect via SSH:
```bash
ssh -p 2222 user@localhost
# Password: mypassword (if --api-key set)
# No password if --api-key omitted
```

The SSH server provides an interactive PTY session with full readline support. The virtual filesystem persists across SSH sessions using the same session store as HTTP.

### MCP server (Model Context Protocol)

memsh can run as an MCP server, exposing a `memsh` tool that allows AI agents to execute bash commands in a sandboxed in-memory filesystem. This is ideal for giving LLMs a safe shell environment.

```bash
# stdio transport (most common for Claude Desktop)
memsh mcp

# HTTP transport
memsh mcp --transport http --addr :8080

# SSE (Server-Sent Events) transport
memsh mcp --transport sse --addr :8080

# Enable WASM plugins (slower startup)
memsh mcp --wasm

# Custom per-tool timeout (default 30s)
memsh mcp --timeout 1m
```

**Installing in Claude Desktop** (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "memsh": {
      "command": "/path/to/memsh",
      "args": ["mcp"]
    }
  }
}
```

Or via the Claude CLI:
```bash
claude mcp add memsh -- /path/to/memsh mcp
```

**MCP tool behavior:**
- The `memsh` tool executes bash commands in a persistent in-memory filesystem
- Filesystem persists across tool calls within a session — use it as a scratchpad
- Real OS filesystem is never touched; no host commands can escape the sandbox
- Stdin is not available; commands that read input receive EOF
- Returns output + current working directory after each command

Example MCP usage:
```python
# Agent calls the memsh tool
tool_call = {"command": "mkdir /data && echo 'hello' > /data/f.txt"}
# Result: "hello\nCwd: /"

tool_call = {"command": "cat /data/f.txt"}
# Result: "hello\nCwd: /"
```

---

## Using memsh as an AI sandbox

memsh is designed as a safe execution environment for AI agents. Unlike running bash directly on the host OS:

- **Isolation**: All file I/O goes to `afero.MemMapFs` — the real OS filesystem is never touched
- **No escape**: External OS commands are blocked by default. Only registered plugins can run.
- **Persistent state**: Within a session, the virtual filesystem persists across requests
- **Reset anytime**: Destroy the session or create a new shell to start fresh

### Common sandbox patterns

```bash
# Run a script safely
echo "rm -rf /" | memsh  # Does nothing in the virtual FS

# Pipe data through memsh tools
cat large.json | memsh jq '.data | select(.value > 10)'

# Test unsafe operations
memsh --args "rm -rf /"  # Safe — only affects virtual FS
```

### Session-based workflows

```bash
# Create isolated environments per user/task
SESSION_ID="user1-task-$(date +%s)"

# Execute multiple commands in the same session
curl -X POST http://localhost:8080/run \
  -H "X-Session-ID: $SESSION_ID" \
  -H "Content-Type: application/json" \
  -d '{"script":"git clone repo /src"}'

curl -X POST http://localhost:8080/run \
  -H "X-Session-ID: $SESSION_ID" \
  -H "Content-Type: application/json" \
  -d '{"script":"cd /src && make test"}'

# Clean up when done
curl -X DELETE http://localhost:8080/session/$SESSION_ID
```

---

## Common pitfalls

**I/O via HandlerCtx only** — always `interp.HandlerCtx(ctx).Stdout`, never a stored writer field. Pipes and redirects depend on this.

**Paths are always absolute** — `sc.ResolvePath("relative/path")` always returns `/absolute/path`. Pass all user-provided paths through `ResolvePath` before calling `sc.FS.*`.

**Slice aliasing in server mode** — when building per-request shell options, always `copy` the base slice before appending:
```go
opts := make([]shell.Option, len(baseOpts))
copy(opts, baseOpts)
opts = append(opts, shell.WithStdIO(...))
```

**`cd` works** because the exec handler runs before `mvdan.cc/sh`'s built-in `cd` intercept — `CdPlugin` calls `sc.SetCwd()` which updates both `s.cwd` and `s.runner.Dir`.

**wazero lifecycle** — one `wazero.Runtime` per `Shell`. Always call `shell.Close()` when done.

---

## Configuration (`~/.memsh/config.toml`)

```toml
[shell]
wasm = true           # false = skip all WASM loading

[plugins]
wasm    = ["python"]  # allowlist; empty = load all
disable = ["wc"]      # disable by name (native or WASM)
```

Startup script: `~/.memsh/.memshrc` (sourced at REPL start and first HTTP/SSH session use).
