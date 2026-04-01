# memsh

A virtual bash shell implemented in Go. memsh executes bash-like commands against an in-memory filesystem — the real OS filesystem is never touched, and external OS commands are blocked by default.

Shell parsing and interpretation is handled by [mvdan.cc/sh/v3](https://github.com/mvdan/sh). Built-in commands, file I/O, and plugins are custom layers on top.

## Features

- **Sandboxed execution** — external OS commands are blocked; only builtins and registered plugins can run
- **In-memory filesystem** — all file operations target `afero.MemMapFs`; nothing touches your disk
- **Bash-like syntax** — pipes, redirects (`>`, `>>`), `&&`, `;`, subshells
- **30+ built-in commands** — `cat`, `ls`, `mkdir`, `rm`, `cp`, `mv`, `grep`, `find`, `awk`, `sort`, `uniq`, `cut`, `tr`, `head`, `tail`, `diff`, `stat`, `wc`, `base64`, and more
- **Combined short flags** — `-rf`, `-la`, `-jrc` etc. work on all commands
- **Scripting languages** — Lua (gopher-lua), JavaScript (goja ES2020+), JSON/YAML processing (jq/yq)
- **WASM plugin system** — extend with WASI-compiled plugins (Go, Python, Ruby runtimes)
- **Native Go plugins** — register custom commands via a simple `Plugin` interface
- **Interactive REPL** — tab completion, command history, familiar prompt
- **Library usage** — embed memsh in Go programs for safe, sandboxed shell scripting

## Quick Start

```bash
# Build
go build ./...

# Interactive REPL
go run .

# Run a script
go run . ./path/to/script.sh

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

## Built-in Commands

| Command | Description |
| --- | --- |
| `cat` | Concatenate and print files |
| `cd` | Change working directory |
| `chmod` | Change file permissions (`-R` recursive) |
| `cp` | Copy files or directories (`-r`) |
| `cut` | Extract fields (`-f`) or characters (`-c`) |
| `diff` | Compare two files line by line (`-u` unified) |
| `echo` | Print arguments (`-n`, `-e`) |
| `find` | Search virtual filesystem (`-name`, `-type`, `-maxdepth`) |
| `grep` | Search file contents (`-i`, `-n`, `-v`, `-r`, `-c`, `-l`, `-w`, `-o`) |
| `head` | Print first N lines (`-n`) or bytes (`-c`) |
| `ls` | List directory contents (`-l`, `-a`, `-R`) |
| `mkdir` | Create directories (`-p`, `-v`, `-m`) |
| `mv` | Move or rename files |
| `pwd` | Print working directory |
| `rm` | Remove files or directories (`-f`, `-r`, `-v`, `-i`, `-d`) |
| `sort` | Sort lines (`-r`, `-u`, `-n`) |
| `stat` | Show file status |
| `tail` | Print last N lines (`-n`) or bytes (`-c`) |
| `tee` | Read stdin; write to stdout and files (`-a`) |
| `touch` | Create or update file timestamps (`-c`, `-r`) |
| `tr` | Translate or delete characters (`-d`, `-s`, `-c`) |
| `uniq` | Filter adjacent duplicate lines (`-c`, `-d`, `-u`) |
| `wc` | Count lines, words, and bytes (`-l`, `-w`, `-c`) |
| `base64` | Encode or decode base64 (`-d`) |
| `sed` | Stream editor (substitution) |
| `awk` | Pattern scanning and processing |
| `jq` | Command-line JSON processor |
| `yq` | Command-line YAML/JSON processor |
| `lua` | Execute Lua code (`-e` for inline) |
| `goja` | Execute JavaScript code (`-e` for inline) |
| `man` | Show help for commands |

## Plugin System

### Native Go Plugins

Implement the `plugins.Plugin` interface:

```go
type HelloPlugin struct{}

func (HelloPlugin) Name() string { return "hello" }

func (HelloPlugin) Run(ctx context.Context, args []string) error {
    hc := interp.HandlerCtx(ctx)  // pipe-aware I/O — always use this
    sc := plugins.ShellCtx(ctx)   // virtual FS, cwd, ResolvePath
    fmt.Fprintf(hc.Stdout, "Hello from %s!\n", sc.Cwd)
    return nil
}
```

Register via `shell.WithPlugin(p)` or add to `defaultNativePlugins()` in `shell/defaults.go`.

### JSON Processing (`jq`)

```bash
echo '{"name":"alice","scores":[10,20,30]}' | jq .name          # "alice"
echo '{"name":"alice"}' | jq -r .name                           # alice (no quotes)
echo '{"items":[1,2,3]}' | jq '.items | length'                 # 3
echo '[1,2,3]' | jq '.[]'                                        # 1\n2\n3
jq -n '{generated: true}'                                        # null input
jq -rc .name data.json                                           # combined flags
```

### YAML/JSON Processing (`yq`)

```bash
echo 'name: alice' | yq .name                                    # alice
echo 'name: alice' | yq -j .                                     # JSON output
printf 'items:\n  - a\n  - b\n' | yq '.items[0]'                # a
yq .host /config.yaml                                            # read from virtual FS
echo '{"host":"localhost"}' | yq .host                           # JSON input works too
yq -jc . data.yaml                                               # compact JSON output
```

### Lua Scripting

```bash
lua -e 'print("hello from lua")'
echo 'for i=1,3 do print(i) end' | lua
lua /script.lua
lua -e 'for i=1,10 do print(i) end' | grep 5
```

Lua has access to the virtual filesystem via `fs_readfile("/path")`.

### JavaScript Scripting

```bash
goja -e 'console.log("hello")'
echo 'console.log("test")' | goja
goja /script.js
goja -e 'const arr=[1,2,3]; console.log(arr.map(x=>x*2).join(","))'
```

JavaScript has access to the virtual filesystem via `fs.readFile("/path")`.

### WASM Plugins

WASM plugins are compiled with `GOOS=wasip1 GOARCH=wasm`. The virtual FS is mounted via WASI so file I/O goes directly into `afero.MemMapFs`.

```bash
make shell/plugins/<name>.wasm   # build a plugin
make                             # build all WASM plugins
go run . plugin install python   # install Python runtime
go run . plugin install ruby     # install Ruby runtime
```

### Plugin Loading Priority

1. `WithPluginBytes(name, wasm)` or `WithPlugin(p)` options
2. Native plugins: `base64`, `wc`, `grep`, `find`, `awk`, `lua`, `goja`, `jq`, `yq`
3. WASM from `defaultPlugins` map (embedded at compile time)
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
| `WithDisabledPlugins(names)` | Exclude specific plugins by name |
| `WithAllowExternalCommands(bool)` | Allow falling back to real OS executables (default: false) |

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

## Testing

```bash
go test ./...                        # full test suite
go test ./tests -v                   # plugin/integration tests verbose
go test ./tests -run TestJq -v       # single suite
go test ./tests -run TestYq -v
go test ./tests -run TestLua -v
go test ./tests -run TestGrep -v
go test ./tests -run TestAwk -v
go test ./tests -run TestGoja -v
```

## Requirements

- Go 1.26+

## License

See [LICENSE](LICENSE).
