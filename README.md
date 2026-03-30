# memsh

A virtual bash shell implemented in Go. memsh executes bash-like commands against an in-memory filesystem — the real OS filesystem is never touched.

Shell parsing and interpretation is handled by [mvdan.cc/sh/v3](https://github.com/mvdan/sh). Built-in commands, file I/O, and WASM plugins are custom layers on top.

## Features

- **In-memory filesystem** — all file operations target `afero.MemMapFs`; nothing touches your disk
- **Bash-like syntax** — pipes, redirects (`>`, `>>`), `&&`, `;`, subshells via `mvdan.cc/sh`
- **25+ built-in commands** — `cat`, `ls`, `mkdir`, `rm`, `cp`, `mv`, `grep`, `find`, `awk`, `sort`, `uniq`, `cut`, `tr`, `head`, `tail`, `diff`, `stat`, `wc`, `base64`, and more
- **WASM plugin system** — extend the shell with WASI-compiled plugins (Go, Python, Ruby runtimes)
- **Native Go plugins** — register custom commands via a simple `Plugin` interface
- **Interactive REPL** — tab completion, command history, familiar prompt
- **Script execution** — run `.sh` files against the virtual FS
- **Library usage** — embed memsh in Go programs for safe shell scripting

## Quick Start

```bash
# Build
go build ./...

# Interactive REPL
go run .

# Run a script
go run . ./scripts/etl-pipeline.sh

# Pipe commands
echo "mkdir /tmp && echo hello > /tmp/f && cat /tmp/f" | go run .
```

## Usage as a Library

memsh is designed to be embedded in Go applications:

```go
package main

import (
    "bytes"
    "context"
    "fmt"
    "log"

    "github.com/spf13/afero"
    "github.com/amjadjibon/memsh/shell"
)

func main() {
    ctx := context.Background()

    var out bytes.Buffer
    sh, err := shell.New(shell.WithStdIO(nil, &out, &out))
    if err != nil {
        log.Fatal(err)
    }

    err = sh.Run(ctx, `
        mkdir -p /home/user/docs
        echo "Hello, memsh!" > /home/user/docs/hello.txt
        cat /home/user/docs/hello.txt
    `)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Print(out.String())
}
```

### Pre-seeding the Virtual Filesystem

```go
fs := afero.NewMemMapFs()
afero.WriteFile(fs, "/var/log/app.log", []byte("[INFO] server started\n[ERROR] timeout\n"), 0644)

var out bytes.Buffer
sh, _ := shell.New(
    shell.WithFS(fs),
    shell.WithStdIO(nil, &out, &out),
)
sh.Run(ctx, "cat /var/log/app.log")
```

## Built-in Commands

| Command | Description |
|---------|-------------|
| `cat` | Concatenate and print files |
| `cd` | Change working directory |
| `chmod` | Change file permissions |
| `cp` | Copy files or directories (`-r` for recursive) |
| `cut` | Extract fields (`-f`) or characters (`-c`) |
| `diff` | Compare two files line by line |
| `echo` | Print arguments |
| `find` | Search virtual filesystem (`-name`, `-type`) |
| `grep` | Search file contents (`-i`, `-n`, `-v`, `-r`) |
| `head` | Print first lines of a file (`-n N`) |
| `ls` | List directory contents |
| `mkdir` | Create directories |
| `mv` | Move or rename files |
| `pwd` | Print working directory |
| `rm` | Remove files or directories |
| `sort` | Sort lines (`-r`, `-u`, `-n`) |
| `stat` | Show file status |
| `tail` | Print last lines of a file (`-n N`) |
| `tee` | Read stdin; write to stdout and files (`-a` to append) |
| `touch` | Create or update file timestamps |
| `tr` | Translate or delete characters (`-d`, `-s`) |
| `uniq` | Filter adjacent duplicate lines (`-c`, `-d`, `-u`) |
| `awk` | Pattern scanning and processing |
| `wc` | Count lines, words, and bytes (`-l`, `-w`, `-c`) |
| `base64` | Encode or decode base64 data (`-d`) |
| `man` | Show help for commands |

## Plugin System

### Native Go Plugins

Implement the `plugins.Plugin` interface and register at startup:

```go
package native

import (
    "context"
    "fmt"
    "github.com/amjadjibon/memsh/shell/plugins"
    "mvdan.cc/sh/v3/interp"
)

type HelloPlugin struct{}

func (HelloPlugin) Name() string { return "hello" }

func (HelloPlugin) Run(ctx context.Context, args []string) error {
    hc := interp.HandlerCtx(ctx)
    sc := plugins.ShellCtx(ctx)
    fmt.Fprintf(hc.Stdout, "Hello from %s!\n", sc.Cwd)
    return nil
}
```

Register via `shell.WithPlugin()` or add to `defaultNativePlugins()` in `shell/defaults.go`.

### WASM Plugins

WASM plugins are compiled with `GOOS=wasip1 GOARCH=wasm`. They use standard `os.Stdin`/`os.Stdout`/`os.Args`. The virtual FS is mounted via WASI so file I/O goes directly into `afero.MemMapFs`.

```bash
# Build a WASM plugin
make shell/plugins/<name>.wasm

# Build all WASM plugins
make

# Install Python/Ruby runtimes
go run . plugin install python
go run . plugin install ruby
```

### Plugin Loading Priority

1. `WithPluginBytes(name, wasm)` or `WithPlugin(p)` options
2. Native plugins from `defaultNativePlugins()` (`base64`, `wc`, `grep`, `find`, `awk`)
3. WASM from `defaultPlugins` map (embedded at compile time)
4. `/memsh/plugins/*.wasm` in the virtual FS
5. `~/.memsh/plugins/*.wasm` on the real OS filesystem

## Configuration

`~/.memsh/config.toml` is loaded at startup (missing file = defaults):

```toml
[shell]
wasm = true          # set false to skip all WASM loading (faster startup)

[plugins]
wasm    = ["python"] # allowlist of ~/.memsh/plugins/*.wasm names; empty = load all
disable = ["wc"]     # exclude specific plugins by name (native or WASM)
```

Session command history is stored in `~/.memsh/history/` as plain text files named by hash.

## Options

| Option | Description |
|--------|-------------|
| `WithFS(fs)` | Set the afero filesystem (default: `afero.NewMemMapFs()`) |
| `WithCwd(path)` | Set initial working directory |
| `WithEnv(env)` | Set initial environment variables |
| `WithStdIO(in, out, err)` | Set standard I/O streams |
| `WithPlugin(p)` | Register a native plugin |
| `WithBuiltin(name, fn)` | Register a raw function as a command |
| `WithPluginBytes(name, wasm)` | Register a WASM plugin from bytes |
| `WithWASMEnabled(bool)` | Enable/disable WASM runtime |
| `WithPluginFilter(names)` | Allowlist for plugin discovery |
| `WithDisabledPlugins(names)` | Exclude specific plugins |

## Project Structure

```
main.go                          → Entry point
cmd/                             → CLI commands (cobra)
  root.go                        → REPL loop + script-file mode
  plugin.go                      → plugin list / plugin install
  history.go                     → history list / history show
  config.go                      → ~/.memsh/config.toml loader
  complete.go                    → Tab completion
  version.go                     → version subcommand
shell/                           → Core library
  shell.go                       → Shell struct, New(), Run()
  options.go                     → Functional options
  builtins.go                    → Built-in command implementations
  fs.go                          → File I/O handler (afero)
  plugin.go                      → WASM plugin registry/loader
  wasi_fs.go                     → afero → wazero sysfs adapter
  defaults.go                    → Default native/WASM plugin registration
  plugins/
    plugin.go                    → Plugin / PluginInfo / ShellContext interfaces
    native/                      → Native Go plugins (base64, wc, grep, find, awk)
example/                         → Standalone usage examples
scripts/                         → Example memsh scripts
```

## Testing

```bash
# Run all tests
go test ./...

# Run a specific test
go test ./shell/... -run TestBuiltins
```

Tests use `afero.NewMemMapFs()` with `bytes.Buffer` for stdout/stderr capture. WASM is disabled in tests for speed via `WithWASMEnabled(false)`.

## Requirements

- Go 1.26+

## License

See [LICENSE](LICENSE).
