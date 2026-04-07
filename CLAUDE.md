# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`memsh` is a **virtual bash shell** implemented in Go. It executes bash-like commands against an `afero.MemMapFs` in-memory filesystem — the real OS filesystem is never touched. Shell parsing/interpretation is delegated to `mvdan.cc/sh/v3`; built-in commands, file I/O, and plugins are custom layers on top.

**Security model:** external OS commands are blocked by default. Only builtins and registered plugins can run. Opt-in via `WithAllowExternalCommands(true)`.

## Commands

```bash
make build          # build ./bin/memsh
make test           # run all tests
make cover          # tests with coverage
make lint           # lint
make clean          # remove binaries and .wasm files
make install        # install to /usr/local/bin

go run .                        # interactive REPL
go run . ./scripts/etl-pipeline.sh  # run a script file
echo "ls /" | go run .          # non-interactive pipe

# Plugin management
go run . plugin list            # list installed plugins
go run . plugin install python  # install Python 3.12 WASM runtime
go run . plugin install ruby    # install Ruby 3.2 WASM runtime
go run . plugin install php     # install PHP 8.2.6 WASM runtime
go run . plugin install /path/to/plugin.wasm  # install local WASM plugin

go test ./...                   # full test suite
go test ./tests -v              # plugin/integration tests verbose
go test ./tests -run TestJq -v  # single test suite
go test ./shell/... -run TestName  # shell package tests
```

Test suites in `tests/`: `TestAwk`, `TestBase64`, `TestFind`, `TestGrep`, `TestGoja`, `TestJq`, `TestLua`, `TestPhp`, `TestPython`, `TestRuby`, `TestWc`, `TestYq`.

```bash
# HTTP server (sessions always enabled, TTL 30m default)
go run . serve
go run . serve --addr :3000 --session-ttl 1h --cors
```

## Architecture

```
Shell.Run(ctx, script)
  → syntax.NewParser().Parse()           # mvdan.cc/sh parses bash syntax
  → interp.Runner.Run(ctx, ast)          # mvdan.cc/sh interprets AST
        ↓ interp.OpenHandler             # fs.go: all file I/O → afero.MemMapFs
        ↓ interp.ExecHandlers
              WithShellContext()         # injects FS+cwd into ctx
              switch args[0]             # builtins.go: 30+ hard-coded commands
              s.builtins[cmd]            # native Go plugins (defaults.go)
              s.plugins[cmd]             # WASM plugins via wazero
              blocked (or next())        # external OS commands blocked by default
```

**Key files:**
- `shell/shell.go` — `Shell` struct, `New()`, one wazero runtime per shell, WASM pre-compiled at startup. After `Run`, `s.cwd = s.runner.Dir` syncs cwd.
- `shell/builtins.go` — `execHandler` middleware; all built-in commands as a switch; flag parsing uses combined short-flag loop (e.g. `-rf`, `-la` work on all commands).
- `shell/options.go` — all functional options: `WithFS`, `WithCwd`, `WithEnv`, `WithStdIO`, `WithPlugin`, `WithBuiltin`, `WithPluginBytes`, `WithWASMEnabled`, `WithPluginFilter`, `WithDisabledPlugins`, `WithAllowExternalCommands`.
- `shell/fs.go` — `openHandler` wires all file I/O to afero; `resolvePath` always returns absolute paths.
- `shell/plugin.go` — WASM registry; `runWASIPlugin` (`_start` export) vs `runCustomPlugin` (`run` export).
- `shell/wasi_fs.go` — `aferoSysFS`: implements `experimentalsys.FS` on top of `afero.Fs`, mounted via wazero so WASI modules read/write the virtual FS directly.
- `shell/defaults.go` — `defaultNativePlugins()` and `defaultPlugins` WASM map.
- `shell/plugins/plugin.go` — `Plugin`, `PluginInfo`, `ShellContext` interfaces; `ShellCtx(ctx)`, `WithShellContext()`.
- `cmd/serve.go` — `memsh serve` HTTP server; sessions always enabled; `sessionStore` holds `afero.Fs` + `cwd` per session ID; each request creates a fresh `Shell` with `WithFS(entry.fs)` so I/O capture works per-request while FS mutations persist.
- `web/terminal.html` — single-file browser terminal UI; embedded at compile time via `web/embed.go` (`//go:embed terminal.html`); served at `GET /`.

## Built-in commands

Implemented directly in `shell/builtins.go` (switch statement in `execHandler`):
- **File ops**: `cat`, `cp`, `mv`, `rm`, `touch`, `mkdir`, `rmdir`, `ln`
- **Directory**: `ls`, `cd`, `pwd`, `du`, `df`
- **Text**: `echo`, `printf`, `head`, `tail`, `sort`, `uniq`, `cut`, `tr`, `grep`, `sed`
- **Info/utils**: `stat`, `diff`, `wc`, `chmod`, `tee`, `xargs`, `read`, `seq`, `date`, `sleep`, `yes`, `env`, `which`, `source`, `.`
- **Shell control**: `exit`, `quit`, `clear`, `reset`, `timeout`
- **Help**: `man`, `help`

## Native plugins (`shell/plugins/native/`)

Registered in `defaultNativePlugins()` in `shell/defaults.go`:

| Plugin | Command(s) | Library |
|--------|-----------|---------|
| `AwkPlugin` | `awk` | `github.com/benhoyt/goawk` |
| `Base64Plugin` | `base64` | stdlib |
| `WcPlugin` | `wc` | stdlib |
| `GrepPlugin` | `grep` | stdlib |
| `FindPlugin` | `find` | stdlib |
| `LuaPlugin` | `lua` | `github.com/yuin/gopher-lua` |
| `GojaPlugin` | `goja` | `github.com/dop251/goja` |
| `JqPlugin` | `jq` | `github.com/itchyny/gojq` |
| `YqPlugin` | `yq` | `github.com/itchyny/gojq` + `gopkg.in/yaml.v3` |
| `CurlPlugin` | `curl` | stdlib `net/http` |
| `ChecksumPlugin` | `md5sum`, `sha1sum`, `sha224sum`, `sha256sum`, `sha384sum`, `sha512sum` | stdlib `crypto/*` |
| `TarPlugin` | `tar` | stdlib `archive/tar` |
| `GzipPlugin` | `gzip`, `gunzip` | stdlib `compress/gzip` |
| `ZipPlugin` | `zip`, `unzip` | stdlib `archive/zip` |
| `CalcPlugin` | `bc`, `expr` | `github.com/contiamo/go-generics` |
| `ColumnPlugin` | `column` | stdlib |
| `EnvsubstPlugin` | `envsubst` | stdlib |
| `MktempPlugin` | `mktemp` | stdlib |
| `HexdumpPlugin` | `xxd`, `hexdump` | stdlib |
| `TerminalPlugin` | `tput`, `stty` | stubs for compatibility |
| `LessPlugin` | `less`, `more` | pager UI (web terminal) |
| `SSHPlugin` | `ssh` | stdlib `net` |
| `CrontabPlugin` | `crontab` | cron expression parsing |
| `SQLitePlugin` | `sqlite3` | `modernc.org/sqlite` |
| `GitPlugin` | `git` | pure Go git implementation |

`yq` parses YAML/JSON input, runs a jq expression, outputs YAML by default or JSON with `-j`. It normalises yaml.v3 types through a JSON round-trip so gojq receives plain Go types.

**Adding a native plugin:**
1. Create `shell/plugins/native/<name>.go`, implement `plugins.Plugin` (and optionally `plugins.PluginInfo`).
2. Add to the slice returned by `defaultNativePlugins()` in `shell/defaults.go`.
3. Add a test file `tests/<name>_test.go`.

```go
type MyPlugin struct{}
func (MyPlugin) Name() string { return "mycmd" }
func (MyPlugin) Run(ctx context.Context, args []string) error {
    hc := interp.HandlerCtx(ctx)   // pipe-aware I/O — always use this, never s.stdout
    sc := plugins.ShellCtx(ctx)    // virtual FS, cwd, ResolvePath
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

All builtins and native plugins parse flags with a combined short-flag loop — `-rf`, `-la`, `-jrc` all work. The pattern:
1. `--` stops flag parsing; remaining args are positionals.
2. Long flags (`--recursive`, etc.) handled as explicit `if` checks before the loop.
3. Flags that consume the next argument (`-m`, `-r`, `-d`, `-f`) are handled standalone before the combined loop.
4. Unknown chars in a combined flag return `<cmd>: invalid option -- '<chars>'`.

## Additional features

### Snapshot/restore
- Serialize `afero.MemMapFs` to JSON for session persistence
- Save/load HTTP sessions via `POST /snapshot` and `GET /snapshot/{id}`
- Useful for caching filesystem state between operations

### Remote script sourcing
- `source` / `.` supports HTTP URLs: `source https://example.com/script.sh`
- Fetches scripts into virtual FS then executes them
- Recursion guard prevents infinite loops

### SSH-like remote shell
- `ssh` command connects to another `memsh serve` instance
- Execute commands on remote memsh servers
- Supports interactive and non-interactive modes

### Tab completion (web terminal)
- Server-side completion endpoint: `GET /complete?cmd=<partial>`
- Completes commands, file paths, and options
- Works in the browser terminal UI

### Cron-style scheduler
- `crontab` plugin for time-based command scheduling
- Cron expression parsing with standard syntax
- Runs registered scripts on intervals within a session

### Pager UI
- `less` / `more` commands for scrollable output
- Integrated with web terminal for browser-based viewing
- Search and navigation support

### Configuration files
- `.memshrc` sourced at REPL start and first HTTP/SSH session use
- Supports aliases, environment variables, and startup scripts
- Stored in user's home directory in the virtual FS

### Alias support
- `alias` / `unalias` commands for bash-compatible alias expansion
- Works in interactive mode via `mvdan.cc/sh`
- Persists across sessions in `.memshrc`

## Testing

`tests/helper.go` provides `NewTestShell()` — WASM disabled by default for speed:

```go
var buf strings.Builder
s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
s.Run(ctx, `jq -r .name /data.json`)
out := strings.TrimSpace(buf.String())
```

Pre-seed the filesystem with `afero.WriteFile(fs, "/path", []byte(...), 0644)`.

`WithCwd` requires a real OS path (validated by `mvdan.cc/sh`). Use `os.MkdirTemp` if a non-root cwd is needed.

## HTTP server (`memsh serve`)

Sessions are **always enabled** — no flag needed. Send `X-Session-ID: <id>` on `POST /run` to persist FS state across requests.

| Endpoint | Description |
| --- | --- |
| `GET /` | Web terminal UI (embedded `web/terminal.html`) |
| `POST /run` | `{"script":"..."}` → `{"output":"...","cwd":"...","error":"..."}` |
| `GET /sessions` | List active sessions (sorted by last use) |
| `DELETE /session/{id}` | Destroy a session |
| `GET /health` | `{"status":"ok","uptime":"...","sessions":N}` |
| `GET /complete?cmd=<partial>` | Tab completion for commands/paths |
| `POST /snapshot` | Save current FS state as JSON snapshot |
| `GET /snapshot/{id}` | Load a saved snapshot |

**Session design:** `sessionEntry` stores `afero.Fs` (pointer — mutations persist across requests) + `cwd` string. Each request creates a new `Shell` with `WithFS(entry.fs)` and `WithStdIO(..., &out, &out)` so output is captured per-request while the FS is shared. `sh.Cwd()` is saved back to `entry.cwd` after each run.

**Flags:** `--addr` (`:8080`), `--session-ttl` (`30m`), `--timeout` (`30s`), `--cors`.

## Critical implementation rules

- **I/O**: always use `interp.HandlerCtx(ctx).Stdout/.Stdin` — never field `s.stdout` — so commands work correctly in pipes and redirects.
- **Paths**: `resolvePath` always returns absolute paths with a leading `/`. `aferoSysFS.toAferoPath` prepends `/` because wazero passes paths without it.
- **wazero lifecycle**: one `wazero.Runtime` per `Shell`. Modules compiled once at `New()`; only `InstantiateModule` called per invocation. Always call `shell.Close()`.
- **`cd` limitation**: `mvdan.cc/sh` intercepts `cd` before `execHandler`, so `builtinCd` is unreachable. After `Run`, `s.cwd = s.runner.Dir` (real OS path joined from `realCwd`). When `realCwd = "/"` (default), virtual paths like `/hello` round-trip correctly.
- **External commands**: `next(ctx, args)` (real OS execution) is only called when `s.allowExternalCmds` is true. Default is blocked with `"<cmd>: command not found"`.
- **serve opts slice**: use `make([]shell.Option, len(baseOpts)+N)` + `copy` before appending per-request options — avoids Go slice aliasing across concurrent requests.

## Configuration

`~/.memsh/config.toml` (missing = defaults, not an error):

```toml
[shell]
wasm = true          # false = skip all WASM loading

[plugins]
wasm    = ["python"] # allowlist for ~/.memsh/plugins/*.wasm; empty = load all
disable = ["wc"]     # exclude by name (native or WASM)
```

### WASM Runtime Installation

WASM language runtimes can be installed from VMware Labs releases:

```bash
go run . plugin install python  # Python 3.12.0 (~25 MB)
go run . plugin install ruby    # Ruby 3.2.2 slim (~8 MB)
go run . plugin install php     # PHP 8.2.6 slim (~6 MB)
```

Installed runtimes are stored in `~/.memsh/plugins/*.wasm` and automatically loaded when `wasm = true` in config.

### Configuration Files

- **`~/.memsh/config.toml`** — Shell and plugin configuration
- **`~/.memsh/.memshrc`** — Startup script sourced at REPL initialization (aliases, environment variables, etc.)
- **`~/.memsh/history/`** — Command history per session (plain text files named by hash)
- **`~/.memsh/plugins/`** — User-installed WASM plugins

## Key dependencies

- `mvdan.cc/sh/v3` — shell parser/interpreter
- `github.com/spf13/afero` — in-memory filesystem
- `github.com/tetratelabs/wazero` — WASM runtime
- `github.com/benhoyt/goawk` — AWK
- `github.com/yuin/gopher-lua` — Lua 5.1
- `github.com/dop251/goja` — JavaScript ES2020+
- `github.com/itchyny/gojq` — jq expression engine (used by both `jq` and `yq`)
- `gopkg.in/yaml.v3` — YAML parsing for `yq`
- `modernc.org/sqlite` — SQLite for `sqlite3` plugin
- `github.com/spf13/cobra` — CLI framework
- `github.com/robfig/cron/v3` — Cron expression parsing for `crontab` plugin
