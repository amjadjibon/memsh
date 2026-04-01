# memsh Test Suite

This directory contains comprehensive tests for memsh plugins and commands.

## Test Files

### Implemented Plugins

- **lua_test.go** - Tests for Lua (gopher-lua interpreter)
  - Inline execution: `lua -e 'print("hello")'`
  - File execution: `lua /script.lua`
  - Stdin input: `echo 'code' | lua`
  - Math operations, tables, error handling

- **grep_test.go** - Tests for grep command
  - Pattern matching with -i (case-insensitive)
  - Invert matching with -v
  - Line numbers with -n
  - Stdin pipe support

- **find_test.go** - Tests for find command
  - List all entries under path
  - Filter by name glob patterns
  - Filter by type (-f for files, -d for directories)

- **awk_test.go** - Tests for AWK command (goawk)
  - Field extraction: `awk '{print $2}'`
  - File processing
  - Program file: `awk -f /prog.awk`
  - Built-in variables (NR, NF, etc.)

- **base64_test.go** - Tests for base64 command
  - Encode from stdin
  - Decode with -d flag
  - File encoding via cat pipe
  - Positional argument encoding

- **wc_test.go** - Tests for wc command
  - Line counting with -l
  - Word counting with -w
  - Byte counting with -c
  - Multiple file handling

- **goja_test.go** - Tests for JavaScript (goja interpreter)
  - Inline execution: `goja -e 'console.log("hello")'`
  - File execution: `goja /script.js`
  - Stdin input: `echo 'code' | goja`
  - Math operations, modern JS features (arrow functions, array methods)
  - Filesystem access via fs.readFile()
  - Error handling for syntax errors

### Placeholder Tests (Future WASM Plugins)

- **python_test.go** - Placeholder for Python WASM plugin
- **ruby_test.go** - Placeholder for Ruby WASM plugin

These tests will skip when the plugins are not yet implemented.

## Running Tests

```bash
# Run all tests in tests directory
go test ./tests -v

# Run specific test suite
go test ./tests -run TestLua -v
go test ./tests -run TestGrep -v
go test ./tests -run TestFind -v
go test ./tests -run TestAwk -v
go test ./tests -run TestBase64 -v
go test ./tests -run TestWc -v
go test ./tests -run TestGoja -v
```

## Test Helpers

The `helper.go` file provides `NewTestShell()` which creates a shell instance with:
- stdout/stderr wired to a strings.Builder
- WASM disabled for faster test execution
- Customizable via shell.Option parameters

## Test Organization

Each test file focuses on a single plugin/command, making it easy to:
- Find tests for specific functionality
- Add new test cases
- Maintain clear separation of concerns

All tests use the `tests` package and import `github.com/amjadjibon/memsh/shell` to test the shell as a black box.
