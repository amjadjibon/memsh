# memsh

A virtual bash shell implemented in Go. memsh executes bash-like commands against an in-memory filesystem — the real OS filesystem is never touched, and external OS commands are blocked by default.

Shell parsing and interpretation is handled by [mvdan.cc/sh/v3](https://github.com/mvdan/sh). Every command is a native Go plugin or WASM plugin — there is no reliance on host system binaries.

## Features

- **Sandboxed execution** — external OS commands are blocked; only registered plugins can run
- **In-memory filesystem** — all file operations target `afero.MemMapFs`; nothing touches your disk
- **Bash-like syntax** — pipes, redirects (`>`, `>>`), `&&`, `;`, subshells, aliases
- **60+ built-in commands** — file ops, text processing, archiving, networking, scripting, and more
- **Combined short flags** — `-rf`, `-la`, `-jrc` etc. work on all commands
- **Scripting languages** — Lua (gopher-lua), JavaScript (goja ES2020+), JSON/YAML (jq/yq), SQLite
- **WASM plugin system** — extend with WASI-compiled plugins (Python, Ruby, PHP runtimes)
- **Native Go plugins** — register custom commands via a simple `Plugin` interface
- **Interactive REPL** — tab completion, command history, `.memshrc` startup script
- **HTTP server** — expose the shell over HTTP with session-scoped virtual filesystems
- **Network egress policy** — control outbound networking with domain/CIDR/port allowlists
- **Library usage** — embed memsh in Go programs for safe, sandboxed shell scripting

## Installation

### Homebrew

```bash
brew tap amjadjibon/memsh
brew install memsh
```

### Go Install

```bash
go install github.com/amjadjibon/memsh@latest
```

### Pre-built Binaries

Download pre-built binaries from the [GitHub Releases](https://github.com/amjadjibon/memsh/releases) page.

**Linux (amd64):**

```bash
curl -Lo memsh.tar.gz https://github.com/amjadjibon/memsh/releases/latest/download/memsh_linux_amd64.tar.gz
tar xzf memsh.tar.gz
sudo mv memsh /usr/local/bin/
```

**macOS (Apple Silicon):**

```bash
curl -Lo memsh.tar.gz https://github.com/amjadjibon/memsh/releases/latest/download/memsh_darwin_arm64.tar.gz
tar xzf memsh.tar.gz
sudo mv memsh /usr/local/bin/
```

**macOS (Intel):**

```bash
curl -Lo memsh.tar.gz https://github.com/amjadjibon/memsh/releases/latest/download/memsh_darwin_amd64.tar.gz
tar xzf memsh.tar.gz
sudo mv memsh /usr/local/bin/
```

**Windows (amd64):**

Download [`memsh_windows_amd64.zip`](https://github.com/amjadjibon/memsh/releases/latest/download/memsh_windows_amd64.zip), extract, and add `memsh.exe` to your `PATH`.

### Build from Source

```bash
git clone https://github.com/amjadjibon/memsh.git
cd memsh
go build -o memsh .
```

### Verify Installation

```bash
memsh --help
```

## Quick Start

```bash
# Interactive REPL
memsh

# Run a script
memsh ./path/to/script.sh

# Pipe commands
echo "mkdir /tmp && echo hello > /tmp/f && cat /tmp/f" | memsh

# HTTP server
memsh serve
memsh serve --addr :3000 --session-ttl 1h --cors-origin https://app.example.com

# Network-restricted shell
memsh --net-mode allowlist \
  --net-allow-domain 'httpbin.org' \
  --net-allow-port 443 \
  -c 'curl https://httpbin.org/get'
```

## Usage as a Library

```go
package main

import (
    "bytes"
    "context"
    "fmt"
    "log"

    "github.com/amjadjibon/memsh/pkg/shell"
)

func main() {
    ctx := context.Background()

    var out bytes.Buffer
    sh, err := shell.New(shell.WithStdIO(nil, &out, &out))
    if err != nil {
        log.Fatal(err)
    }
    defer sh.Close()

    err = sh.Run(ctx, `
        mkdir -p /home/user/docs
        echo '{"name":"alice","role":"admin"}' > /home/user/docs/user.json
        jq -r .name /home/user/docs/user.json
    `)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Print(out.String()) // alice
}
```

### Pre-seeding the Virtual Filesystem

```go
fs := afero.NewMemMapFs()
afero.WriteFile(fs, "/config.yaml", []byte("host: localhost\nport: 8080\n"), 0644)

var out bytes.Buffer
sh, _ := shell.New(
    shell.WithFS(fs),
    shell.WithStdIO(nil, &out, &out),
)
sh.Run(ctx, "yq .host /config.yaml") // localhost
```

## Commands

| Command | Description |
| --- | --- |
| `cat` | Concatenate and print files |
| `cd` | Change working directory |
| `chmod` | Change file permissions (`-R` recursive) |
| `cp` | Copy files or directories (`-r`) |
| `cut` | Extract fields (`-f`) or characters (`-c`) |
| `date` | Print current date and time |
| `df` | Report filesystem disk space usage |
| `diff` | Compare two files line by line (`-u` unified) |
| `du` | Estimate file space usage |
| `echo` | Print arguments (`-n`, `-e`) |
| `env` | Print or set environment variables |
| `find` | Search virtual filesystem (`-name`, `-type`, `-maxdepth`) |
| `grep` | Search file contents (`-i`, `-n`, `-v`, `-r`, `-c`, `-l`, `-w`, `-o`) |
| `head` | Print first N lines (`-n`) or bytes (`-c`) |
| `ln` | Create hard or symbolic links (`-s`, `-f`) |
| `ls` | List directory contents (`-l`, `-a`, `-R`) |
| `mkdir` | Create directories (`-p`, `-v`, `-m`) |
| `mv` | Move or rename files |
| `printf` | Format and print data |
| `pwd` | Print working directory |
| `read` | Read a line from stdin into a variable |
| `rm` | Remove files or directories (`-f`, `-r`, `-v`) |
| `rmdir` | Remove empty directories |
| `sed` | Stream editor (substitution) |
| `seq` | Print a sequence of numbers |
| `sleep` | Delay for a specified amount of time |
| `sort` | Sort lines (`-r`, `-u`, `-n`) |
| `stat` | Show file status |
| `tail` | Print last N lines (`-n`) or bytes (`-c`) |
| `tee` | Read stdin; write to stdout and files (`-a`) |
| `timeout` | Run a command with a time limit |
| `touch` | Create or update file timestamps |
| `tr` | Translate or delete characters (`-d`, `-s`, `-c`) |
| `uniq` | Filter adjacent duplicate lines (`-c`, `-d`, `-u`) |
| `wc` | Count lines, words, and bytes (`-l`, `-w`, `-c`) |
| `which` | Locate a command |
| `xargs` | Build and execute command lines from stdin |
| `yes` | Repeatedly output a string |
| `awk` | Pattern scanning and processing |
| `base64` | Encode or decode base64 (`-d`) |
| `bc`, `expr` | Arbitrary precision calculator / expression evaluator |
| `column` | Columnate output |
| `crontab` | Schedule commands with cron expressions |
| `curl` | Transfer data from URLs |
| `envsubst` | Substitute environment variables in strings |
| `goja` | Execute JavaScript (ES2020+) code |
| `git` | Pure-Go git implementation |
| `gzip`, `gunzip` | Compress/decompress gzip files |
| `hexdump`, `xxd` | Hex dump of files |
| `jq` | Command-line JSON processor |
| `less`, `more` | Scrollable pager (web terminal) |
| `ln` | Create links |
| `lua` | Execute Lua 5.1 code |
| `man`, `help` | Show help for commands |
| `md5sum`, `sha256sum`, … | File checksum (md5, sha1, sha224, sha256, sha384, sha512) |
| `mktemp` | Create a temporary file or directory |
| `sqlite3` | SQLite database shell |
| `ssh` | Connect to a remote memsh server |
| `tar` | Archive files |
| `tput`, `stty` | Terminal control stubs |
| `yq` | Command-line YAML/JSON processor |
| `zip`, `unzip` | Compress/decompress zip files |

## Plugin System

### Native Go Plugins

Implement the `Plugin` interface from `pkg/shell/plugins`:

```go
import (
    "github.com/amjadjibon/memsh/pkg/shell/plugins"
    "mvdan.cc/sh/v3/interp"
)

type HelloPlugin struct{}

func (HelloPlugin) Name() string        { return "hello" }
func (HelloPlugin) Description() string { return "greet the user" }
func (HelloPlugin) Usage() string       { return "hello [name]" }

func (HelloPlugin) Run(ctx context.Context, args []string) error {
    hc := interp.HandlerCtx(ctx)  // pipe-aware I/O — always use this
    sc := plugins.ShellCtx(ctx)   // virtual FS, cwd, ResolvePath, SetEnv, …
    fmt.Fprintf(hc.Stdout, "Hello from %s!\n", sc.Cwd)
    return nil
}
```

Register at shell creation time:

```go
sh, _ := shell.New(shell.WithPlugin(HelloPlugin{}))
```

Or add to `defaultNativePlugins()` in `pkg/shell/defaults.go` to include it in every shell instance.

### JSON Processing (`jq`)

```bash
echo '{"name":"alice","scores":[10,20,30]}' | jq .name          # "alice"
echo '{"name":"alice"}' | jq -r .name                           # alice (no quotes)
echo '{"items":[1,2,3]}' | jq '.items | length'                 # 3
jq -n '{generated: true}'                                        # null input
jq -rc .name data.json                                           # combined flags
```

### YAML/JSON Processing (`yq`)

```bash
echo 'name: alice' | yq .name                                    # alice
echo 'name: alice' | yq -j .                                     # JSON output
printf 'items:\n  - a\n  - b\n' | yq '.items[0]'                # a
yq .host /config.yaml                                            # read from virtual FS
yq -jc . data.yaml                                               # compact JSON output
```

### Lua Scripting

```bash
lua -e 'print("hello from lua")'
echo 'for i=1,3 do print(i) end' | lua
lua /script.lua
```

### JavaScript Scripting

```bash
goja -e 'console.log("hello")'
echo 'console.log("test")' | goja
goja /script.js
goja -e 'const arr=[1,2,3]; console.log(arr.map(x=>x*2).join(","))'
```

### WASM Plugins

WASM plugins are compiled with `GOOS=wasip1 GOARCH=wasm`. The virtual FS is mounted via WASI so file I/O goes directly into `afero.MemMapFs`.

```bash
go run . plugin install python   # Python 3.12.0 (~25 MB)
go run . plugin install ruby     # Ruby 3.2.2 slim (~8 MB)
go run . plugin install php      # PHP 8.2.6 slim (~6 MB)
go run . plugin install /path/to/plugin.wasm  # local file
go run . plugin list             # list installed plugins
```

Installed runtimes are stored in `~/.memsh/plugins/*.wasm`.

### Plugin Loading Priority

1. `WithPlugin(p)` or `WithPluginBytes(name, wasm)` options
2. Native Go plugins registered in `defaultNativePlugins()`
3. Embedded WASM from `defaultPlugins` map (currently empty)
4. `/memsh/plugins/*.wasm` in the virtual FS
5. `~/.memsh/plugins/*.wasm` on the real OS filesystem

## Options

| Option | Description |
| --- | --- |
| `WithFS(fs)` | Set the afero filesystem (default: `afero.NewMemMapFs()`) |
| `WithCwd(path)` | Set initial working directory |
| `WithEnv(env)` | Set initial environment variables |
| `WithStdIO(in, out, err)` | Set standard I/O streams |
| `WithPlugin(p)` | Register a native plugin |
| `WithBuiltin(name, fn)` | Register a raw function as a command |
| `WithPluginBytes(name, wasm)` | Register a WASM plugin from bytes |
| `WithWASMEnabled(bool)` | Enable/disable WASM runtime (default: true) |
| `WithPluginFilter(names)` | Allowlist for WASM plugin discovery |
| `WithDisabledPlugins(names...)` | Exclude specific plugins by name |
| `WithAllowExternalCommands(bool)` | Allow falling back to real OS executables (default: false) |
| `WithInheritEnv(bool)` | Inherit parent process environment (default: true; use false in server mode) |
| `WithAliases(map)` | Pre-seed the alias table |
| `WithNetworkPolicy(policy)` | Set outbound network policy (`off`, `allowlist`, `full`) |
| `WithNetworkLimits(limits)` | Set network request/bytes/runtime limits |

## HTTP Server

```bash
go run . serve                          # listen on :8080
go run . serve --addr :3000 --cors-origin https://app.example.com
go run . serve --session-ttl 1h --timeout 30s
```

Sessions are always enabled. Send `X-Session-ID: <id>` on `POST /run` to persist the virtual filesystem across requests.

| Endpoint | Description |
| --- | --- |
| `GET /` | Web terminal UI |
| `POST /run` | `{"script":"..."}` → `{"output":"...","cwd":"...","error":"..."}` |
| `GET /sessions` | List active sessions |
| `DELETE /session/{id}` | Destroy a session |
| `GET /health` | `{"status":"ok","uptime":"...","sessions":N}` |
| `POST /complete` | `{"input":"...","cursor":N}` → tab completion |
| `GET /session/{id}/snapshot` | Export session filesystem as JSON |
| `POST /session/{id}/snapshot` | Import a snapshot (use `"new"` as id to create) |

### Networking Policy Flags

These flags work for both local `memsh` and `memsh serve`:

```bash
--net-mode off|allowlist|full
--net-allow-domain <domain>    # repeatable, supports *.example.com
--net-allow-cidr <cidr>        # repeatable, e.g. 203.0.113.0/24
--net-allow-port <port>        # repeatable, e.g. 443
```

Examples:

```bash
# Block all outbound networking
memsh --net-mode off -c 'curl https://example.com'

# Allow only HTTPS to httpbin.org
memsh --net-mode allowlist \
  --net-allow-domain 'httpbin.org' \
  --net-allow-port 443 \
  -c 'curl https://httpbin.org/get'
```

If DNS fails (`lookup <host>: no such host`), that is environment/network resolution, not a policy deny. A policy deny returns explicit errors like `network disabled by policy` or `destination port ... is not allowed`.

## LLM Integration

memsh has two modes for connecting LLMs: an **MCP server** (any MCP-compatible client) and a built-in **agent** (interactive ReAct loop).

### MCP Server (`memsh mcp`)

The MCP server exposes a single `memsh` tool that lets any LLM execute bash commands in a sandboxed in-memory filesystem. The real OS is never touched.

**Transports:**

| Transport | Command | Use case |
|---|---|---|
| stdio (default) | `memsh mcp` | Claude Desktop, Claude Code CLI |
| HTTP (MCP 2025-03-26+) | `memsh mcp --transport http --addr :8080` | Programmatic / multi-session |
| SSE (legacy) | `memsh mcp --transport sse --addr :8080` | Legacy MCP clients |

**Claude Desktop** (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "memsh": {
      "command": "/usr/local/bin/memsh",
      "args": ["mcp"]
    }
  }
}
```

**Claude Code CLI:**

```bash
claude mcp add memsh -- memsh mcp
```

**Other MCP clients** — start the HTTP transport and point your client at the endpoint:

```bash
memsh mcp --transport http --addr :8080
# Connect to: http://localhost:8080/
```

**Tool behaviour:**
- Tool name: `memsh`, input field: `command` (string)
- The virtual filesystem **persists across calls** within a session — use it as a scratchpad
- `exit` / `quit` are treated as success, not errors
- Stdin is not available; commands that read stdin receive EOF
- Returns command output + current working directory (`Cwd: /path`)
- Per-call timeout (default 30 s, minimum 5 s): `memsh mcp --timeout 1m`
- WASM plugins (Python/Ruby/PHP) disabled by default for fast startup: `memsh mcp --wasm`

**Example tool call result:**
```
/home/user/data
file1.txt  file2.txt

Cwd: /home/user/data
```

### Built-in Agent (`memsh agent`)

`memsh agent` runs a ReAct loop: the LLM thinks, calls the `memsh` tool, observes results, and repeats until the task is done. After each response it pauses for your review.

Provider is inferred from the model name:

| Model prefix | Provider | API key env var |
|---|---|---|
| `gpt-*` | OpenAI | `OPENAI_API_KEY` |
| `claude-*` | Anthropic | `ANTHROPIC_API_KEY` |
| `gemini-*` | Google | `GOOGLE_API_KEY` |
| `grok-*` | xAI | `XAI_API_KEY` |
| any | OpenAI-compatible | `--base-url` + `--api-key` |

```bash
# Interactive TUI (human-in-the-loop)
memsh agent --model claude-opus-4-5
memsh agent --model gpt-4o
memsh agent --model gemini-2.0-flash
memsh agent --model grok-3

# Single query, non-interactive
memsh agent --model claude-opus-4-5 \
  --query "create a CSV of 10 random users and compute average age with awk"

# With WASM plugins (Python/Ruby/PHP)
memsh agent --model gpt-4o --wasm

# Explicit API key and base URL (any OpenAI-compatible endpoint)
memsh agent --model my-model --api-key sk-xxx --base-url https://my-provider/v1
```

The agent uses an isolated `afero.MemMapFs` — nothing written during the session touches your real filesystem.

## Configuration

`~/.memsh/config.toml` is loaded at startup (missing file = defaults):

```toml
[shell]
wasm = true          # set false to skip all WASM loading (faster startup)

[plugins]
wasm    = ["python"] # allowlist of ~/.memsh/plugins/*.wasm names; empty = load all
disable = ["wc"]     # exclude specific plugins by name (native or WASM)
```

Configuration files:

- `~/.memsh/config.toml` — shell and plugin configuration
- `~/.memsh/.memshrc` — startup script (sourced at REPL start and first HTTP session)
- `~/.memsh/history/` — per-session command history
- `~/.memsh/plugins/` — user-installed WASM plugins

## Testing

```bash
go test ./...                        # full test suite
go test ./tests -v                   # integration tests verbose
go test ./tests -run TestJq -v       # single suite
go test ./pkg/shell/... -run TestName  # shell package tests
```

## Development

```bash
# Build
make build

# Run tests
make test

# Run coverage report
make cover

# Lint
make lint

# Clean build artifacts
make clean

# View all available commands
make help
```

### Creating a Release

The project uses [GoReleaser](https://goreleaser.com/) for automated releases and Homebrew cask generation.

```bash
# 1. Test the release process (dry-run)
make release-dry-run TAG=v1.0.0

# 2. Create the actual release
make release TAG=v1.0.0
```

The `make release` command will:
1. **Commit and push any uncommitted changes** (prepares for release)
2. **Clean the `dist/` directory** (removes old build artifacts)
3. Clean build artifacts (bin/ and *.wasm files)
4. Create and push a git tag
5. Build binaries for all platforms (Linux, macOS, Windows × AMD64, ARM64)
6. Create a GitHub Release with all binaries
7. **Generate and push the Homebrew cask** automatically via goreleaser to [`homebrew-memsh`](https://github.com/amjadjibon/homebrew-memsh)

After release, users can install via:
```bash
brew tap amjadjibon/memsh
brew install memsh
```

**Note:** Ensure `GITHUB_TOKEN` is set for goreleaser to create releases and push to repositories.

## Requirements

- Go 1.26+

## License

See [LICENSE](LICENSE).
