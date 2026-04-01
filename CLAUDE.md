# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`memsh` is a **virtual bash shell** implemented in Go. It executes bash-like commands against an `afero.MemMapFs` in-memory filesystem — the real OS filesystem is never touched. Shell parsing/interpretation is delegated to `mvdan.cc/sh/v3`; built-in commands, file I/O, and WASM plugins are all custom layers on top.

## Commands

```bash
# Build Go binary (output: ./bin/memsh)
make build

# Run (interactive REPL)
go run .

# Run a script file
go run . ./scripts/etl-pipeline.sh

# Run tests
make test
go test ./...

# Run a single test
go test ./shell/... -run TestName

# Run tests with coverage
make cover

# Lint code
make lint

# Build all WASM plugins (requires Go 1.26+, GOOS=wasip1)
make

# Build a specific WASM plugin
make shell/plugins/base64.wasm

# Remove compiled binaries and WASM files
make clean

# Install to /usr/local/bin
make install

# Run the library usage examples
go run ./example/

# Download and install Python/Ruby WASM runtimes to ~/.memsh/plugins/
go run . plugin install python
go run . plugin install ruby

# Pipe commands non-interactively
echo "mkdir /tmp && echo hello > /tmp/f && cat /tmp/f" | go run .
```

## Architecture

```
main.go                          → cmd.Execute()
cmd/root.go                      → REPL loop + script-file mode (cobra)
cmd/plugin.go                    → `plugin list` / `plugin install [python|ruby|<file>]` subcommands
cmd/history.go                   → `history list` / `history show <hash>` — reads ~/.memsh/history/
cmd/config.go                    → loads ~/.memsh/config.toml (TOML via BurntSushi/toml)
cmd/complete.go                  → readline tab-completion (commands + virtual FS paths)
cmd/version.go                   → `version` subcommand
shell/                           → core library (Shell struct, builtins, WASM runtime)
example/                         → standalone Go program showing shell.New() usage patterns
shell/plugins/plugin.go          → Plugin / PluginInfo / ShellContext interfaces (package plugins)
shell/plugins/native/            → native Go plugin implementations (base64, wc)
shell/defaults.go                → registers defaultNativePlugins(); WASM embed hook
shell/wasi_fs.go                 → afero→wazero sysfs adapter (writable WASI FS)
scripts/                         → example memsh scripts (run with `go run . <script>`)
```

**Shell execution flow:**

```
Shell.Run(ctx, script)
  → syntax.NewParser().Parse(script)        # mvdan.cc/sh parses bash syntax
  → interp.Runner.Run(ctx, ast)             # mvdan.cc/sh interprets AST
        ↓ interp.OpenHandler                # fs.go: redirects/pipes → afero.MemMapFs
        ↓ interp.ExecHandlers (middleware)
              plugins.WithShellContext()    # inject FS+cwd into ctx
              s.builtins[cmd]  → native Go  # builtins.go + shell/plugins/native/
              s.plugins[cmd]   → WASM       # plugin.go via wazero
              next()                        # mvdan.cc/sh default handler
```

**Key files:**
- `shell/shell.go` — `Shell` struct and `New()`. Creates the wazero runtime once, calls `wasi_snapshot_preview1.MustInstantiate`, pre-compiles all WASM plugins at startup.
- `shell/options.go` — functional options: `WithFS`, `WithCwd`, `WithEnv`, `WithStdIO`, `WithPlugin`, `WithBuiltin`, `WithPluginBytes`, `WithWASMEnabled`, `WithPluginFilter`, `WithDisabledPlugins`.
- `shell/builtins.go` — `execHandler` middleware; 30+ hard-coded built-in commands implemented as switch statement; injects `ShellContext` into ctx before dispatch.
- `shell/fs.go` — `openHandler` (all file I/O → afero), `resolvePath`.
- `shell/plugin.go` — WASM registry/loader; `runWASIPlugin` (WASI `_start`) vs `runCustomPlugin` (`run` export); lazy-compiles plugins added after startup.
- `shell/wasi_fs.go` — `aferoSysFS` / `aferoSysFile` / `aferoSysDirFile`: implements `experimentalsys.FS` on top of `afero.Fs`. Mounted via `sysfs.FSConfig.WithSysFSMount` so WASI modules write directly into the virtual FS.
- `shell/defaults.go` — `defaultNativePlugins()` returns native Go plugin implementations; `defaultPlugins` map for optional WASM embeds.
- `shell/plugins/plugin.go` — `Plugin`, `PluginInfo`, `ShellContext`, `ShellCtx(ctx)`, `WithShellContext(ctx, sc)`.
- `shell/plugins/native/` — native Go plugin implementations: `base64`, `wc`, `grep`, `find`, `awk`.

## Website Deployment

The project website (`web/index.html`) is automatically deployed to GitHub Pages via GitHub Actions:

- **Workflow**: `.github/workflows/static.yml`
- **Source**: `./web/` directory
- **Trigger**: Push to `main` branch
- **URL**: https://amjadjibon.github.io/memsh/

To update the website:
1. Modify files in `web/` directory
2. Commit and push to `main`
3. GitHub Actions automatically deploys

## Configuration

`~/.memsh/config.toml` is loaded at startup (missing file = defaults, not an error):

```toml
[shell]
wasm = true          # set false to skip all WASM loading (faster startup)

[plugins]
wasm    = ["python"] # allowlist of ~/.memsh/plugins/*.wasm names; empty = load all
disable = ["wc"]     # exclude specific plugins by name (native or WASM)
```

Session command history is stored per-session in `~/.memsh/history/` as plain text files named by a hash. `history list` shows sessions sorted by time; `history show <hash-prefix>` prints numbered commands.

## Built-in commands

The shell has 30+ built-in commands implemented in `shell/builtins.go`:
- **File operations**: `cat`, `cp`, `mv`, `rm`, `touch`, `mkdir`, `rmdir`, `ln`
- **Directory operations**: `ls`, `cd`, `pwd`, `find`, `du`, `df`
- **Text processing**: `echo`, `printf`, `head`, `tail`, `sort`, `uniq`, `cut`, `tr`, `grep`, `sed`
- **File info**: `stat`, `diff`, `wc`, `chmod`
- **Utilities**: `tee`, `xargs`, `read`, `seq`, `date`, `sleep`, `yes`

Native Go plugins in `shell/plugins/native/` provide additional commands:
- `awk` — pattern scanning and processing (via goawk lib)
- `base64` — encode/decode base64
- `wc` — count lines, words, bytes
- `grep` — search file contents
- `find` — search filesystem
- `lua` — execute Lua code (via gopher-lua)

## Plugin system

### Native plugins (Go)

Implement `plugins.Plugin` in `shell/plugins/native/`:

```go
type HelloPlugin struct{}
func (HelloPlugin) Name() string { return "hello" }
func (HelloPlugin) Run(ctx context.Context, args []string) error {
    hc := interp.HandlerCtx(ctx)   // per-invocation I/O — required for pipes
    sc := plugins.ShellCtx(ctx)    // virtual FS, cwd, env, ResolvePath
    fmt.Fprintln(hc.Stdout, "Hello!")
    return nil
}
```

Register at startup via `defaultNativePlugins()` in `shell/defaults.go`, or at call-site with `WithPlugin(p)` / `shell.Register(p)`.

### Lua scripting

memsh includes a Lua interpreter via gopher-lua. Execute Lua code directly:

```bash
# Inline execution
lua -e 'print("hello")'
lua -e 'print(2 + 3)'

# Execute file from virtual FS
lua /script.lua

# Read from stdin
echo 'print("test")' | lua
```

Lua has access to the virtual filesystem via `fs_readfile()`:

```lua
content = fs_readfile("/data.txt")
print(content)
```

The Lua plugin uses gopher-lua (github.com/yuin/gopher-lua) and provides full Lua 5.1 compatibility.

### WASM plugins (WASI)

Standard Go programs compiled with `GOOS=wasip1 GOARCH=wasm`. They use `os.Stdin`/`os.Stdout`/`os.Args` normally. The virtual FS is mounted at `/` via `aferoSysFS`, so WASI file I/O goes directly into `afero.MemMapFs` — no temp-directory sync.

Two WASM module types:
- **WASI** (exports `_start`): standard WASI program; `_start` runs during `Instantiate`.
- **Custom** (exports `run`): imports `memsh::write_stdout`, `memsh::read_stdin`, `memsh::arg_get`, `memsh::fs_open/read/write/close`, `memsh::env_get`, `memsh::get_cwd`.

**Adding a WASM plugin:**
1. Create `plugins/<name>/main.go`
2. `make` → `shell/plugins/<name>.wasm`
3. Restore the `//go:embed plugins/*.wasm` block in `shell/defaults.go` and add the name to `defaultPlugins`.

### Plugin loading priority (first registration wins)
1. `WithPluginBytes(name, wasm)` or `WithPlugin(p)` options
2. Native plugins from `defaultNativePlugins()` (currently: `base64`, `wc`, `grep`, `find`, `awk`)
3. WASM from `defaultPlugins` map (currently empty - can be used for embedded WASM)
4. `/memsh/plugins/*.wasm` in the virtual FS
5. `~/.memsh/plugins/*.wasm` on the real OS filesystem

## Critical implementation rules

**I/O in builtins and native plugins:** always use `interp.HandlerCtx(ctx).Stdout` / `.Stdin` — never `s.stdout` — so the command participates correctly in pipes and redirects.

**Virtual FS paths:** `afero.MemMapFs` stores all paths with a leading `/`. `resolvePath` always returns absolute paths. `aferoSysFS.toAferoPath` prepends `/` for wazero, which passes paths without the leading slash.

**wazero runtime lifecycle:** one `wazero.Runtime` per `Shell`, shared across all WASM invocations. `wasi_snapshot_preview1` is instantiated once. Each WASM module is compiled once at `New()` time (`s.compiled`); `runPlugin` only calls `InstantiateModule` per invocation. Call `shell.Close()` to release wazero resources.

**`cd` limitation:** `mvdan.cc/sh` intercepts `cd` before `execHandler` runs and updates its own real-OS cwd. Our `builtinCd` is unreachable for the `cd` command. Scripts should use absolute virtual paths.

## Testing pattern

**Standard test helper** (from `shell/shell_test.go`):

```go
func newTestShell(t *testing.T, buf *bytes.Buffer, opts ...shell.Option) *shell.Shell {
    t.Helper()
    base := []shell.Option{
        shell.WithStdIO(strings.NewReader(""), buf, buf),
        shell.WithWASMEnabled(false), // skip WASM for speed
    }
    s, err := shell.New(append(base, opts...)...)
    if err != nil {
        t.Fatalf("shell.New: %v", err)
    }
    return s
}
```

**Basic test**:

```go
func TestExample(t *testing.T) {
    var out bytes.Buffer
    sh := newTestShell(t, &out)

    ctx := context.Background()
    err := sh.Run(ctx, "mkdir /tmp && echo hello > /tmp/f && cat /tmp/f")

    if err != nil {
        t.Errorf("Run failed: %v", err)
    }
    if !strings.Contains(out.String(), "hello") {
        t.Errorf("expected output not found")
    }
}
```

**Pre-seeding filesystem**:

```go
func TestWithFixture(t *testing.T) {
    fs := afero.NewMemMapFs()
    afero.WriteFile(fs, "/var/log/app.log", []byte("log line 1\nlog line 2\n"), 0644)

    var out bytes.Buffer
    sh := newTestShell(t, &out, shell.WithFS(fs))

    ctx := context.Background()
    sh.Run(ctx, "cat /var/log/app.log")
    // assert output
}
```

**`WithCwd` requires a real OS path** (validated by `mvdan.cc/sh`). Use `os.MkdirTemp` for tests that need a non-root cwd:

```go
func realTmpDir(t *testing.T) string {
    t.Helper()
    dir, err := os.MkdirTemp("", "shelltest-*")
    if err != nil {
        t.Fatalf("MkdirTemp: %v", err)
    }
    t.Cleanup(func() { os.RemoveAll(dir) })
    return dir
}
```

**WASM is disabled in tests by default** for speed via `WithWASMEnabled(false)`.

## Requirements

- **Go 1.26+** (required for WASI support)
- Dependencies managed via `go.mod`
- Key dependencies:
  - `github.com/tetratelabs/wazero` — WASM runtime
  - `mvdan.cc/sh/v3` — shell parser/interpreter
  - `github.com/spf13/afero` — in-memory filesystem
  - `github.com/benhoyt/goawk` — AWK implementation
  - `github.com/chzyer/readline` — readline support
  - `github.com/spf13/cobra` — CLI framework
  - `github.com/yuin/gopher-lua` — Lua 5.1 interpreter
