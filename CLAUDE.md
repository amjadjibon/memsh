# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`memsh` is a **virtual bash shell** implemented in Go. It executes bash-like commands against an `afero.MemMapFs` in-memory filesystem — the real OS filesystem is never touched. Shell parsing/interpretation is delegated to `mvdan.cc/sh/v3`; all commands (built-ins and extensions) are native Go plugins or WASM plugins.

**Security model:** external OS commands are blocked by default. Only registered plugins can run. Opt-in via `WithAllowExternalCommands(true)`.

## Commands

```bash
make build          # build ./bin/memsh
make test           # run all tests
make cover          # tests with coverage
make lint           # vet (go vet ./...)
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

go test ./...                       # full test suite
go test ./tests -v                  # integration tests verbose
go test ./tests -run TestJq -v      # single test suite
go test ./pkg/shell/... -run TestName  # shell package tests

# HTTP server (sessions always enabled, TTL 30m default)
go run . serve
go run . serve --addr :3000 --session-ttl 1h --cors
```

Test suites in `tests/`: `TestAwk`, `TestBase64`, `TestFind`, `TestGrep`, `TestGoja`, `TestJq`, `TestLua`, `TestPhp`, `TestPython`, `TestRuby`, `TestWc`, `TestYq`, `TestGit`, `TestSQLite`, `TestSnapshot`, and more.

## Architecture

```
Shell.Run(ctx, script)
  → syntax.NewParser().Parse()           # mvdan.cc/sh parses bash syntax
  → interp.Runner.Run(ctx, ast)          # mvdan.cc/sh interprets AST
        ↓ interp.OpenHandler             # pkg/shell/fs.go: all file I/O → afero.MemMapFs
        ↓ interp.ExecHandlers
              WithShellContext()         # injects FS+cwd+aliases into ctx
              s.builtins[cmd]            # native Go plugins (each in pkg/shell/plugins/native/<cmd>.go)
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
- `internal/session/store.go` — `Store` holding `afero.Fs` + `cwd` + `RcLoaded` per session ID.
- `internal/session/aliases.go` — `SaveAliases`/`RestoreAliases` persisting aliases to `/.memsh_aliases` in the virtual FS.
- `internal/repl/interactive.go` — interactive REPL (readline, `.memshrc` loading, history).
- `internal/ssh/server.go` — SSH server for `memsh serve --ssh`.
- `web/terminal.html` — single-file browser terminal UI; embedded via `web/embed.go`; served at `GET /`.

## Native plugins (`pkg/shell/plugins/native/`)

Every shell command is a separate file implementing `plugins.Plugin`. All are registered in `defaultNativePlugins()` in `pkg/shell/defaults.go`. There is no central switch statement.

| Plugin file(s) | Command(s) | Library |
|----------------|-----------|---------|
| `cat.go`, `head.go`, `tail.go`, `tee.go` | `cat`, `head`, `tail`, `tee` | stdlib |
| `cp.go`, `mv.go`, `rm.go`, `ln.go` | `cp`, `mv`, `rm`, `ln` | stdlib |
| `mkdir.go`, `rmdir.go`, `touch.go` | `mkdir`, `rmdir`, `touch` | stdlib |
| `ls.go`, `cd.go`, `pwd.go`, `du.go`, `df.go` | `ls`, `cd`, `pwd`, `du`, `df` | stdlib |
| `echo.go`, `printf.go`, `sort.go`, `uniq.go`, `cut.go`, `tr.go`, `sed.go` | `echo`, `printf`, `sort`, `uniq`, `cut`, `tr`, `sed` | stdlib |
| `stat.go`, `diff.go`, `chmod.go`, `wc.go` | `stat`, `diff`, `chmod`, `wc` | stdlib |
| `tee.go`, `xargs.go`, `read.go`, `seq.go`, `date.go`, `sleep.go`, `yes.go` | `tee`, `xargs`, `read`, `seq`, `date`, `sleep`, `yes` | stdlib |
| `env.go`, `printenv.go`, `envsubst.go` | `env`, `printenv`, `envsubst` | stdlib |
| `source.go`, `dot.go` | `source`, `.` | stdlib |
| `exit.go`, `quit.go`, `clear.go`, `reset.go` | `exit`, `quit`, `clear`, `reset` | stdlib |
| `help.go`, `man.go` | `help`, `man` | stdlib |
| `awk.go` | `awk` | `github.com/benhoyt/goawk` |
| `grep.go` | `grep` | stdlib |
| `find.go` | `find` | stdlib |
| `lua.go` | `lua` | `github.com/yuin/gopher-lua` |
| `goja.go` | `goja` | `github.com/dop251/goja` |
| `jq.go` | `jq` | `github.com/itchyny/gojq` |
| `yq.go` | `yq` | `github.com/itchyny/gojq` + `gopkg.in/yaml.v3` |
| `curl.go` | `curl` | stdlib `net/http` |
| `checksum.go` | `md5sum`, `sha1sum`, `sha224sum`, `sha256sum`, `sha384sum`, `sha512sum` | stdlib `crypto/*` |
| `tar.go` | `tar` | stdlib `archive/tar` |
| `gzip.go` | `gzip`, `gunzip` | stdlib `compress/gzip` |
| `bc.go`, `calc.go`, `expr.go` | `bc`, `expr` | stdlib |
| `column.go` | `column` | stdlib |
| `mktemp.go` | `mktemp` | stdlib |
| `hexdump.go` | `xxd`, `hexdump` | stdlib |
| `stty.go` | `tput`, `stty` | stubs for compatibility |
| `less.go` | `less`, `more` | pager UI (web terminal) |
| `ssh.go` | `ssh` | stdlib `net` |
| `crontab.go` | `crontab` | `github.com/robfig/cron/v3` |
| `sqlite.go` | `sqlite3` | `modernc.org/sqlite` |
| `git/git.go` | `git` | `github.com/go-git/go-git/v5` |

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

**Flags:** `--addr` (`:8080`), `--session-ttl` (`30m`), `--timeout` (`30s`), `--cors`.

## Critical implementation rules

- **I/O**: always use `interp.HandlerCtx(ctx).Stdout/.Stdin` — never the `s.stdout` field — so commands work correctly in pipes and redirects.
- **Paths**: `resolvePath` always returns absolute paths with a leading `/`. `aferoSysFS.toAferoPath` prepends `/` because wazero passes paths without it.
- **wazero lifecycle**: one `wazero.Runtime` per `Shell`. Modules compiled once at `New()`; only `InstantiateModule` called per invocation. Always call `shell.Close()`.
- **`cd`**: `CdPlugin` calls `sc.SetCwd(dir)` which calls `s.changeDir`, updating both `s.cwd` and `s.runner.Dir` directly. `mvdan.cc/sh` still intercepts the literal `cd` builtin, so the plugin name `"cd"` works because the exec handler runs first via `interp.Interactive(true)` + alias expansion.
- **External commands**: `next(ctx, args)` (real OS execution) is only called when `s.allowExternalCmds` is true. Default is blocked with `"<cmd>: command not found"`.
- **serve opts slice**: use `make([]shell.Option, len(baseOpts)+N)` + `copy` before appending per-request options — avoids Go slice aliasing across concurrent requests.
- **Env isolation**: use `WithInheritEnv(false)` in server mode to prevent leaking host environment variables to remote users.

## Configuration

`~/.memsh/config.toml` (missing = defaults, not an error):

```toml
[shell]
wasm = true          # false = skip all WASM loading

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
- `github.com/yuin/gopher-lua` — Lua 5.1
- `github.com/dop251/goja` — JavaScript ES2020+
- `github.com/itchyny/gojq` — jq expression engine (used by both `jq` and `yq`)
- `gopkg.in/yaml.v3` — YAML parsing for `yq`
- `modernc.org/sqlite` — SQLite for `sqlite3` plugin
- `github.com/go-git/go-git/v5` — pure Go git implementation
- `github.com/spf13/cobra` — CLI framework
- `github.com/robfig/cron/v3` — Cron expression parsing for `crontab` plugin
