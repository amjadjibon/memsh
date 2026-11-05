# AGENTS.md

This file provides guidance to AGENTS when working with code in this repository.

## Project Overview

`memsh` is a **virtual bash shell** implemented in Go. It executes bash-like commands against an `afero.MemMapFs` in-memory filesystem — with the default command set the real OS filesystem is not touched. Shell parsing/interpretation is delegated to `mvdan.cc/sh/v3`; all commands (built-ins and extensions) are native Go plugins or WASM plugins.

**Security model:** external OS commands are blocked by default — only registered plugins can run. Opt-in via `WithAllowExternalCommands(true)`.

This is a *soft* boundary, not a sandbox. It holds for native plugins written
against the virtual FS and for WASM plugins (wazero grants a module no ambient
authority: WASI preview1 has no exec, no socket-open, and the only mount is the
afero FS). It does **not** hold for plugins that embed a third-party
interpreter, because those arrive pre-wired to the host OS. Three are therefore
withheld unless `WithUnsafeInterpreters(true)` / `--unsafe-interpreters` is set:

| Command | Host access it carries |
| --- | --- |
| `lua` | gopher-lua opens the full Lua stdlib: `os.execute` runs real commands, `io.open` reads/writes real files |
| `go` | mvm binds the whole Go stdlib including `os/exec` and `syscall` |
| `sqlite3` | operates on a real OS temp file, so `ATTACH DATABASE` / `VACUUM INTO` reach arbitrary real paths |

`memsh serve` refuses `--unsafe-interpreters` without `--api-key`. For untrusted
input, leave them off and prefer the WASM plugin path.

## Repository Guidelines (Contributor Quick Guide)

### Project Structure
- `cmd/`: CLI subcommands and Cobra wiring.
- `pkg/shell/`: core runtime, virtual FS integration, and command/plugin execution.
- `pkg/shell/plugins/native/`: built-in command implementations.
- `internal/`: session, server, REPL, config, and agent internals.
- `tests/`: integration tests using in-memory FS helpers.
- `web/`: static web terminal assets.

### Build, Test, and Lint
- `make build` builds `./bin/memsh`.
- `make plugins` compiles `plugins/*/main.go` to WASM (no `plugins/` source dir exists today, so it is a no-op).
- `make test` runs `go test ./... -v -count=1`.
- `make cover` writes `coverage.out` and `coverage.html`.
- `make lint` runs `go vet -stdmethods=false ./...`.
- `make release TAG=vX.Y.Z` / `make release-dry-run TAG=vX.Y.Z` run `scripts/release.sh` (goreleaser + homebrew tap).

### Style and Naming
- Use `gofmt` formatting; prefer `goimports` for import cleanup.
- Keep package names lowercase; exported identifiers in `CamelCase`.
- Use descriptive, feature-based filenames (for example `snapshot.go`, `server.go`).
- For commands/plugins, keep names lowercase and shell-like (`jq`, `grep`, `mktemp`).

### Tests and PRs
- Add/extend `*_test.go` files near changed code or in `tests/` for command behavior.
- Prefer `tests.NewTestShell(...)` to test commands consistently.
- Follow Conventional Commit prefixes: `feat:`, `fix:`, `refactor:`, `chore:`, `docs:`.
- PRs should include a concise summary, linked issue(s), and test evidence (`make test`).

## Commands

```bash
make build          # build ./bin/memsh
make test           # run all tests
make cover          # tests with coverage
make lint           # go vet -stdmethods=false ./...
make clean          # remove binaries and .wasm files
make install        # install to /usr/local/bin

go run .                        # interactive REPL
go run . ./scripts/etl-pipeline.sh  # run a script file
echo "ls /" | go run .          # non-interactive pipe
go run . -c 'echo hi | wc -c'   # run a command string
go run . --session-store ./s --session-id dev   # persist local session FS across runs

# Plugin management
go run . plugin list            # list installed plugins
go run . plugin install python  # install Python 3.12 WASM runtime
go run . plugin install ruby    # install Ruby 3.2 WASM runtime
go run . plugin install php     # install PHP 8.2.6 WASM runtime
go run . plugin install /path/to/plugin.wasm  # install local WASM plugin

go test ./...                       # full test suite
go test ./tests -v                  # integration tests verbose
go test ./tests -run TestJq -v      # single test suite
go test ./pkg/shell/... -run TestName  # shell package tests
go test ./pkg/shell/plugins/native -run TestSed  # per-plugin unit tests

# HTTP server (sessions always enabled, TTL 30m default, binds 127.0.0.1:8080)
go run . serve
go run . serve --addr :3000 --session-ttl 1h --cors-origin https://example.com --api-key secret
go run . serve --ssh            # also start SSH server on 127.0.0.1:2222

# MCP server exposing a sandboxed "memsh" tool (one isolated FS per client session)
go run . mcp                    # stdio (default)
go run . mcp --transport http --api-key secret   # or --transport sse; binds 127.0.0.1:8080
# mcp http/sse flags: --api-key, --max-sessions (100), --session-ttl (30m), --net-* (private ranges denied)
```

Tests live in two places: integration tests in `tests/` (one `<cmd>_test.go` per command, via `NewTestShell`) and unit tests next to each plugin in `pkg/shell/plugins/native/`. Python/Ruby/PHP tests skip when the WASM runtime is not installed.

## Architecture

```
Shell.Run(ctx, script)
  → syntax.NewParser().Parse()           # mvdan.cc/sh parses bash syntax
  → interp.Runner.Run(ctx, ast)          # mvdan.cc/sh interprets AST
        ↓ interp.OpenHandler             # pkg/shell/fs.go: all file I/O → afero.MemMapFs
        ↓ interp.ExecHandlers
              WithShellContext()         # injects FS+cwd+aliases into ctx
              s.builtins[cmd]            # native Go plugins (each in pkg/shell/plugins/native/<cmd>.go)
              #                          # lua/go/sqlite3 only if WithUnsafeInterpreters(true)
              s.plugins[cmd]             # WASM plugins via wazero
              blocked (or next())        # external OS commands blocked by default
```

**Key files:**
- `pkg/shell/shell.go` — `Shell` struct, `New()`, `execHandler`, `changeDir`, `sourceFile`, one wazero runtime per shell, WASM pre-compiled at startup. After `Run`, `s.cwd = s.runner.Dir`.
- `pkg/shell/options.go` — all functional options: `WithFS`, `WithCwd`, `WithEnv`, `WithStdIO`, `WithPlugin`, `WithBuiltin`, `WithPluginBytes`, `WithWASMEnabled`, `WithPluginFilter`, `WithDisabledPlugins`, `WithAllowExternalCommands`, `WithInheritEnv`, `WithAliases`.
- `pkg/shell/fs.go` — `openHandler` wires all file I/O to afero; `resolvePath` always returns absolute paths.
- `pkg/shell/plugin.go` — WASM registry; `runWASIPlugin` (`_start` export) vs `runCustomPlugin` (`run` export).
- `pkg/shell/wasi_fs.go` — `aferoSysFS`: implements `experimentalsys.FS` on top of `afero.Fs`, mounted via wazero so WASI modules read/write the virtual FS directly.
- `pkg/shell/defaults.go` — `defaultNativePlugins()` slice and `defaultPlugins` WASM map.
- `pkg/shell/plugins/plugin.go` — `Plugin`, `PluginInfo`, `ShellContext` interfaces; `ShellCtx(ctx)`, `WithShellContext()`.
- `pkg/shell/snapshot.go` — `TakeSnapshot`/`RestoreSnapshot` for serializing `afero.MemMapFs` to JSON.
- `internal/server/server.go` — HTTP handler (`Handler`), all route registrations and request logic.
- `internal/server/middleware.go` — API-key auth, CORS, rate limiting (`--trust-proxy` for `X-Forwarded-For`).
- `internal/session/store.go` — `Store` holding `afero.Fs` + `cwd` + `RcLoaded` per session ID.
- `internal/session/persistence.go` — durable sessions on disk (`--session-store`): snapshot + cwd + runtime/network usage per session, file name is sha256 of the ID.
- `internal/session/limits.go` — per-session caps (`--session-max-files`, `--session-max-bytes` (default 256 MiB), `--session-max-runtime`). `internal/session/quota.go` (`Limits.WrapFS` → `QuotaFs`) enforces the FS caps on every write; `output.go` (`LimitedBuffer`) caps buffered command output at 10 MiB.
- `internal/session/aliases.go` — `SaveAliases`/`RestoreAliases` persisting aliases to `/.memsh_aliases` in the virtual FS.
- `internal/mcp/server.go` + `cmd/mcp.go` — MCP server (stdio / streamable HTTP / SSE) built on `github.com/modelcontextprotocol/go-sdk`; `internal/mcp/pool.go` gives each http/sse session its own shell, caps them, and closes shells whose session ended.
- `pkg/network/` — outbound network policy, metered dialer, and usage budgets held per `Shell` (`networkPolicy`/`networkDialer` in `shell.go`) so network plugins share one enforcement point; configured by `--net-mode off|allowlist|full`, `--net-allow-domain/-cidr/-port`, and `--net-max-*` budgets (`cmd/flags.go`).
- `pkg/cron/` + `internal/session/cron.go` — cron scheduling backing the `crontab` plugin.
- `internal/repl/interactive.go` — interactive REPL (readline, `.memshrc` loading, history).
- `internal/ssh/server.go` — SSH server for `memsh serve --ssh`.
- `web/terminal.html` — single-file browser terminal UI; embedded via `web/embed.go`; served at `GET /`.

## Native plugins (`pkg/shell/plugins/native/`)

Every shell command is a separate file implementing `plugins.Plugin`. All are registered in `defaultNativePlugins()` in `pkg/shell/defaults.go`. There is no central switch statement.

The authoritative command list is `defaultNativePlugins()` (grouped by category: filesystem, text processing, data tools, scripting, archive, ...); `lua`, `go` and `sqlite3` live in `gatedInterpreterPlugins()` and are only registered with `WithUnsafeInterpreters(true)` (see security model). Most commands are stdlib-only; notable library-backed ones: `awk` (goawk), `go` (`mvm-sh/mvm/interp` — `go run/test/fmt`), `lua` (gopher-lua), `goja` (JavaScript), `jq`/`yq` (gojq + yaml.v3), `sqlite3` (modernc.org/sqlite), `crontab` (robfig/cron), and `git` (go-git, in the `native/git/` subpackage). Shared flag/IO helpers live in `native/helpers.go`.

`yq` parses YAML/JSON input, runs a jq expression, outputs YAML by default or JSON with `-j`. It normalises `yaml.v3` types through a JSON round-trip so gojq receives plain Go types.

**Adding a native plugin:**
1. Create `pkg/shell/plugins/native/<name>.go`, implement `plugins.Plugin` (and optionally `plugins.PluginInfo`).
2. Add to the slice returned by `defaultNativePlugins()` in `pkg/shell/defaults.go`.
3. Add a test file `tests/<name>_test.go`.

```go
type MyPlugin struct{}
func (MyPlugin) Name() string        { return "mycmd" }
func (MyPlugin) Description() string { return "one-line description" }
func (MyPlugin) Usage() string       { return "mycmd [-f] [args...]" }
func (MyPlugin) Run(ctx context.Context, args []string) error {
    hc := interp.HandlerCtx(ctx)   // pipe-aware I/O — always use this, never s.stdout
    sc := plugins.ShellCtx(ctx)    // virtual FS, cwd, ResolvePath, SetEnv, etc.
    fmt.Fprintln(hc.Stdout, "hello")
    return nil
}
var _ plugins.PluginInfo = MyPlugin{} // optional compile-time check
```

## WASM plugins

Standard Go programs compiled with `GOOS=wasip1 GOARCH=wasm`. The virtual FS is mounted at `/` so WASI file I/O goes directly into `afero.MemMapFs`.

- WASI type (exports `_start`): runs during `Instantiate`.
- Custom type (exports `run`): imports `memsh::write_stdout`, `memsh::read_stdin`, `memsh::arg_get`, `memsh::fs_*`, `memsh::env_get`, `memsh::get_cwd`.

Plugin loading priority (first registration wins):
1. `WithPluginBytes` / `WithPlugin` options
2. `defaultNativePlugins()` — native Go
3. `defaultPlugins` map — embedded WASM (currently empty)
4. `/memsh/plugins/*.wasm` in the virtual FS
5. `~/.memsh/plugins/*.wasm` on the real OS filesystem

## Flag parsing convention

All native plugins parse flags with a combined short-flag loop — `-rf`, `-la`, `-jrc` all work. The pattern:
1. `--` stops flag parsing; remaining args are positionals.
2. Long flags (`--recursive`, etc.) handled as explicit `if` checks before the loop.
3. Flags that consume the next argument (`-m`, `-r`, `-d`, `-f`) are handled standalone before the combined loop.
4. Unknown chars in a combined flag return `<cmd>: invalid option -- '<chars>'`.

## Testing

`tests/helper.go` provides `NewTestShell()` — WASM disabled by default for speed:

```go
var buf strings.Builder
s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
_ = s.Run(ctx, `jq -r .name /data.json`)
out := strings.TrimSpace(buf.String())
```

Pre-seed the filesystem with `afero.WriteFile(fs, "/path", []byte(...), 0644)`.

`WithCwd` requires a real OS path (validated by `mvdan.cc/sh`). Use `tests.RealTmpDir(t)` (wraps `os.MkdirTemp`) if a non-root cwd is needed.

## HTTP server (`memsh serve`)

Sessions are **always enabled** — no flag needed. Send `X-Session-ID: <id>` on `POST /run` to persist FS state across requests.

| Endpoint | Description |
| --- | --- |
| `GET /` | Web terminal UI (embedded `web/terminal.html`) |
| `POST /run` | `{"script":"..."}` → `{"output":"...","cwd":"...","pager":bool,"error":"..."}` |
| `GET /sessions` | List active sessions (sorted by last use) |
| `DELETE /session/{id}` | Destroy a session |
| `GET /health` | `{"status":"ok","uptime":"...","sessions":N}` |
| `POST /complete` | `{"input":"...","cursor":N}` → tab completion result |
| `GET /session/{id}/snapshot` | Export session FS as JSON snapshot |
| `POST /session/{id}/snapshot` | Import JSON snapshot into session (use `"new"` as id to create) |

**Session design:** `session.Entry` (`internal/session/store.go`) stores `afero.Fs` (pointer — mutations persist across requests) + `cwd` + `RcLoaded` bool. Each request creates a new `Shell` with `WithFS(entry.Fs)` and `WithStdIO(...)` so output is captured per-request while the FS is shared. Aliases are serialised to `/.memsh_aliases` in the virtual FS between requests via `session.SaveAliases`/`RestoreAliases`.

**Flags:** `--addr` (`127.0.0.1:8080`), `--session-ttl` (`30m`), `--timeout` (`30s`, min 5s), `--cors-origin`, `--api-key` (Bearer auth on mutating endpoints), `--trust-proxy`, `--max-sessions` (`100`), `--session-store`, `--session-max-{files,bytes,runtime}`, `--ssh`/`--ssh-addr`/`--ssh-host-key`, plus the `--net-*` network flags. See `cmd/serve.go`.

## Critical implementation rules

- **I/O**: always use `interp.HandlerCtx(ctx).Stdout/.Stdin` — never the `s.stdout` field — so commands work correctly in pipes and redirects.
- **Paths**: `resolvePath` always returns absolute paths with a leading `/`. `aferoSysFS.toAferoPath` prepends `/` because wazero passes paths without it.
- **Plugin cwd**: inside a plugin use `sc.Cwd` / `sc.ResolvePath` (set per command from `interp.HandlerCtx(ctx).Dir`), never `s.cwd` — `s.cwd` only advances when `Run` returns, so it is stale after a `cd` earlier in the same script or inside a subshell.
- **wazero lifecycle**: one `wazero.Runtime` per `Shell`. Modules compiled once at `New()`; only `InstantiateModule` called per invocation. Always call `shell.Close()`.
- **`cd`**: `CdPlugin` calls `sc.SetCwd(dir)` which calls `s.changeDir`, updating both `s.cwd` and `s.runner.Dir` directly. `mvdan.cc/sh` still intercepts the literal `cd` builtin, so the plugin name `"cd"` works because the exec handler runs first via `interp.Interactive(true)` + alias expansion.
- **External commands**: `next(ctx, args)` (real OS execution) is only called when `s.allowExternalCmds` is true. Default is blocked with `"<cmd>: command not found"`.
- **No host access from plugins**: a default plugin must only touch `sc.FS`, `sc.NetworkDialContext` and the shell env. Embedded libraries often reach the host by default — goawk (`system()`, pipes, file redirects, `ENVIRON`: run with `NoExec`/`NoFileWrites`/`NoFileReads` and an explicit `Environ`), go-git (file/ssh/git transports: only http(s) remotes, other protocols swapped for a refusing stub), gopher-lua/mvm/sqlite (gated behind `WithUnsafeInterpreters`). Never fall back to `os/user`, `os.Hostname`, `os.Getenv` or `net.Lookup*`; use the virtual identity constants in `native/helpers.go`. Regression tests: `tests/unsafe_interpreters_test.go`, `tests/git_sandbox_test.go`.
- **Cancellation**: long-running plugins must stop when `ctx` is done (goja `Interrupt`, gojq `RunWithContext`, goawk `ExecuteContext`; wazero runtime uses `WithCloseOnContextDone`). `tests/cancellation_test.go` checks each.
- **Network default**: `network.DefaultPolicy()` denies loopback/private/reserved ranges. Tests that hit a local server pass `tests.WithLoopbackNetwork()`.
- **Server limits**: wrap a session FS with `Limits.WrapFS` (enforces bytes/files on write) and buffer output with `session.NewLimitedBuffer`; serve/mcp default `--session-max-bytes` is 256 MiB. The HTTP chain includes `OriginGuardMiddleware` (rejects cross-site `Origin`, and non-loopback `Host` when bound to loopback).
- **serve opts slice**: use `make([]shell.Option, len(baseOpts)+N)` + `copy` before appending per-request options — avoids Go slice aliasing across concurrent requests.
- **Env isolation**: use `WithInheritEnv(false)` in server mode to prevent leaking host environment variables to remote users.

## Configuration

`~/.memsh/config.toml` (missing = defaults, not an error):

```toml
[shell]
wasm = true                 # false = skip all WASM loading
unsafe_interpreters = false # true = also register lua, go, sqlite3

[plugins]
wasm    = ["python"] # allowlist for ~/.memsh/plugins/*.wasm; empty = load all
disable = ["wc"]     # exclude by name (native or WASM)
```

Configuration files:
- **`~/.memsh/config.toml`** — Shell and plugin configuration
- **`~/.memsh/.memshrc`** — Startup script (sourced at REPL start and first HTTP/SSH session use)
- **`~/.memsh/history/`** — Command history per session
- **`~/.memsh/plugins/`** — User-installed WASM plugins

WASM language runtimes installable from VMware Labs:
```bash
go run . plugin install python  # Python 3.12.0 (~25 MB)
go run . plugin install ruby    # Ruby 3.2.2 slim (~8 MB)
go run . plugin install php     # PHP 8.2.6 slim (~6 MB)
```

## Key dependencies

- `mvdan.cc/sh/v3` — shell parser/interpreter
- `github.com/spf13/afero` — in-memory filesystem
- `github.com/tetratelabs/wazero` — WASM runtime
- `github.com/benhoyt/goawk` — AWK
- `github.com/mvm-sh/mvm/interp` — Go interpreter for the `go` plugin (`go run`, `go test`, `go fmt`)
- `github.com/yuin/gopher-lua` — Lua 5.1
- `github.com/dop251/goja` — JavaScript ES2020+
- `github.com/itchyny/gojq` — jq expression engine (used by both `jq` and `yq`)
- `gopkg.in/yaml.v3` — YAML parsing for `yq`
- `modernc.org/sqlite` — SQLite for `sqlite3` plugin
- `github.com/go-git/go-git/v5` — pure Go git implementation
- `github.com/spf13/cobra` — CLI framework
- `github.com/robfig/cron/v3` — Cron expression parsing for `crontab` plugin
