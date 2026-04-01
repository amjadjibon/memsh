# memsh Test Suite

This directory contains integration tests for memsh plugins and commands. All tests run against the virtual in-memory filesystem with WASM disabled for speed.

## Test Files

| File | Command | What's tested |
| --- | --- | --- |
| `awk_test.go` | `awk` | Field extraction, file processing, `-f` program file, NR/NF variables |
| `base64_test.go` | `base64` | Encode from stdin, decode with `-d`, positional args |
| `find_test.go` | `find` | List entries, `-name` glob, `-type f/d` |
| `goja_test.go` | `goja` | Inline `-e`, file execution, stdin, modern JS, `fs.readFile()` |
| `grep_test.go` | `grep` | Pattern match, `-i`, `-v`, `-n`, stdin pipe |
| `jq_test.go` | `jq` | Field selection, `-r`, `-c`, `-n`, array iteration, virtual FS files, combined flags |
| `lua_test.go` | `lua` | Inline `-e`, file execution, stdin, tables, `fs_readfile()` |
| `wc_test.go` | `wc` | `-l`, `-w`, `-c`, multiple files |
| `yq_test.go` | `yq` | YAML field selection, `-j` JSON output, `-jc` compact, `-r`, virtual FS files, JSON input |
| `python_test.go` | `python` | Placeholder — skips when plugin not installed |
| `ruby_test.go` | `ruby` | Placeholder — skips when plugin not installed |

## Running Tests

```bash
go test ./tests -v                   # all tests
go test ./tests -run TestJq -v       # single suite
go test ./tests -run TestYq -v
go test ./tests -run TestLua -v
go test ./tests -run TestGrep -v
go test ./tests -run TestFind -v
go test ./tests -run TestAwk -v
go test ./tests -run TestBase64 -v
go test ./tests -run TestWc -v
go test ./tests -run TestGoja -v
```

## Test Helper

`helper.go` provides `NewTestShell()`:

```go
var buf strings.Builder
s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
s.Run(ctx, `jq -r .name /data.json`)
out := strings.TrimSpace(buf.String())
```

- stdout and stderr both captured in `buf`
- WASM disabled (`WithWASMEnabled(false)`)
- Pass additional `shell.Option` values to customise

Pre-seed files with `afero.WriteFile(fs, "/path", []byte(...), 0644)` before creating the shell.
