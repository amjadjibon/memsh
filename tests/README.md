# memsh integration tests

Integration tests for memsh commands. They run against an in-memory
filesystem with WASM disabled for speed. The convention is one
`<command>_test.go` per command; unit tests for a plugin live next to it in
`pkg/shell/plugins/native/`.

## Running

```bash
go test ./tests -v                # all integration tests
go test ./tests -run TestJq -v    # one command
make test                         # whole repository
```

Python, Ruby and PHP tests skip unless the WASM runtime is installed
(`memsh plugin install python|ruby|php`).

## Helpers (`helper.go`)

| Helper | Use |
| --- | --- |
| `NewTestShell(t, &buf, opts...)` | Default shell; stdout and stderr both go to `buf` |
| `NewInterpreterTestShell(t, &buf, opts...)` | Also registers `lua`, `go` and `sqlite3` (`WithUnsafeInterpreters(true)`) |
| `RealTmpDir(t)` | Real OS temp dir, for `WithCwd` (which needs an existing host path) |
| `WithLoopbackNetwork()` | Lets a test reach a local `httptest` server; the default network policy denies loopback and private ranges |

```go
fs := afero.NewMemMapFs()
_ = afero.WriteFile(fs, "/data.json", []byte(`{"name":"memsh"}`), 0o644)

var buf strings.Builder
s := NewTestShell(t, &buf, shell.WithFS(fs))
_ = s.Run(ctx, `jq -r .name /data.json`)
out := strings.TrimSpace(buf.String())
```

## Sandbox regression tests

These guard the security model; keep them passing and extend them when a
plugin gains host-facing behaviour:

- `unsafe_interpreters_test.go`: lua, go and sqlite3 stay gated; awk cannot run commands or touch host files.
- `git_sandbox_test.go`: git cannot use local, `file://` or ssh remotes.
- `cancellation_test.go`: long-running commands stop when the context is cancelled.
- `cwd_test.go`: commands resolve paths against the directory they run in.
