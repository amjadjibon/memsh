package shell_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

// The tests in this file exercise pkg/shell/plugin.go's WASM execution path
// (loadPlugins, runWASIPlugin, runCustomPlugin, and the plugin file-descriptor
// table) without requiring network access. The only tests that otherwise
// touch this code (python/ruby/php) download a runtime and are skipped by
// default (CI_PYTHON_TEST=1 etc.), so this path normally has zero coverage.
//
// Two kinds of fixtures are used:
//   - A hand-assembled "custom ABI" WASM module (raw bytes, built at the byte
//     level below) that exports run(argc) and imports memsh:: host
//     functions directly. This is necessary because a plain `go build
//     GOOS=wasip1` binary's exported "run" cannot be called without first
//     running "_start" to initialize the Go runtime, which is exactly what
//     runCustomPlugin's InstantiateModule(..., WithStartFunctions()) (no
//     functions) avoids calling.
//   - A real WASI binary, compiled on the fly via `go build GOOS=wasip1
//     GOARCH=wasm`, to exercise runWASIPlugin.

// --- byte-level WASM module builder ---

func uleb128(v uint32) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			break
		}
	}
	return out
}

func sleb128(v int32) []byte {
	var out []byte
	more := true
	for more {
		b := byte(v & 0x7f)
		v >>= 7
		if (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0) {
			more = false
		} else {
			b |= 0x80
		}
		out = append(out, b)
	}
	return out
}

func wasmSection(id byte, payload []byte) []byte {
	var buf bytes.Buffer
	buf.WriteByte(id)
	buf.Write(uleb128(uint32(len(payload))))
	buf.Write(payload)
	return buf.Bytes()
}

func wasmVec(items ...[]byte) []byte {
	var buf bytes.Buffer
	buf.Write(uleb128(uint32(len(items))))
	for _, it := range items {
		buf.Write(it)
	}
	return buf.Bytes()
}

func wasmName(s string) []byte {
	var buf bytes.Buffer
	buf.Write(uleb128(uint32(len(s))))
	buf.WriteString(s)
	return buf.Bytes()
}

// buildCustomPluginModule builds a minimal custom-ABI memsh WASM module that
// imports memsh::write_stdout and exports run(argc) which writes msg to
// stdout and returns 0.
func buildCustomPluginModule(msg string) []byte {
	var mod bytes.Buffer
	mod.Write([]byte{0x00, 0x61, 0x73, 0x6d}) // magic
	mod.Write([]byte{0x01, 0x00, 0x00, 0x00}) // version

	// T0: (i32)->(i32) [run]; T1: (i32,i32)->(i32) [write_stdout]
	t0 := []byte{0x60, 0x01, 0x7f, 0x01, 0x7f}
	t1 := []byte{0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f}
	mod.Write(wasmSection(1, wasmVec(t0, t1)))

	imp := append(append(wasmName("memsh"), wasmName("write_stdout")...), 0x00, 0x01)
	mod.Write(wasmSection(2, wasmVec(imp)))

	mod.Write(wasmSection(3, wasmVec([]byte{0x00}))) // one local func, type 0

	mod.Write(wasmSection(5, wasmVec([]byte{0x00, 0x01}))) // memory: 1 page

	runExport := append(wasmName("run"), 0x00, 0x01) // func idx 1 (after 1 import)
	mod.Write(wasmSection(7, wasmVec(runExport)))

	var body bytes.Buffer
	body.WriteByte(0x00) // no locals
	body.WriteByte(0x41) // i32.const 0 (ptr)
	body.Write(sleb128(0))
	body.WriteByte(0x41) // i32.const len(msg)
	body.Write(sleb128(int32(len(msg))))
	body.WriteByte(0x10) // call write_stdout (idx 0)
	body.Write(uleb128(0))
	body.WriteByte(0x1a) // drop
	body.WriteByte(0x41) // i32.const 0 (return value)
	body.Write(sleb128(0))
	body.WriteByte(0x0b) // end
	funcEntry := append(uleb128(uint32(body.Len())), body.Bytes()...)
	mod.Write(wasmSection(10, wasmVec(funcEntry)))

	var data bytes.Buffer
	data.WriteByte(0x00) // active, memory 0
	data.WriteByte(0x41)
	data.Write(sleb128(0))
	data.WriteByte(0x0b)
	data.Write(uleb128(uint32(len(msg))))
	data.WriteString(msg)
	mod.Write(wasmSection(11, wasmVec(data.Bytes())))

	return mod.Bytes()
}

// buildCustomPluginModuleFSRead builds a custom-ABI module that opens the
// path passed as args[1] via fs_open, reads it via fs_read, closes it via
// fs_close, and echoes what it read via write_stdout. This exercises the
// plugin file-descriptor table (allocFd/getFd/closeFd in plugin.go).
func buildCustomPluginModuleFSRead() []byte {
	var mod bytes.Buffer
	mod.Write([]byte{0x00, 0x61, 0x73, 0x6d})
	mod.Write([]byte{0x01, 0x00, 0x00, 0x00})

	// T0 (i32)->(i32) [run]; T1 (i32,i32,i32)->(i32) [arg_get/fs_read];
	// T2 (i32,i32)->(i32) [fs_open/write_stdout]; T3 (i32)->() [fs_close]
	t0 := []byte{0x60, 0x01, 0x7f, 0x01, 0x7f}
	t1 := []byte{0x60, 0x03, 0x7f, 0x7f, 0x7f, 0x01, 0x7f}
	t2 := []byte{0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f}
	t3 := []byte{0x60, 0x01, 0x7f, 0x00}
	mod.Write(wasmSection(1, wasmVec(t0, t1, t2, t3)))

	impArgGet := append(append(wasmName("memsh"), wasmName("arg_get")...), 0x00, 0x01)
	impFsOpen := append(append(wasmName("memsh"), wasmName("fs_open")...), 0x00, 0x02)
	impFsRead := append(append(wasmName("memsh"), wasmName("fs_read")...), 0x00, 0x01)
	impFsClose := append(append(wasmName("memsh"), wasmName("fs_close")...), 0x00, 0x03)
	impWriteStdout := append(append(wasmName("memsh"), wasmName("write_stdout")...), 0x00, 0x02)
	mod.Write(wasmSection(2, wasmVec(impArgGet, impFsOpen, impFsRead, impFsClose, impWriteStdout)))

	mod.Write(wasmSection(3, wasmVec([]byte{0x00}))) // one local func "run", type 0
	mod.Write(wasmSection(5, wasmVec([]byte{0x00, 0x01})))

	mod.Write(wasmSection(7, wasmVec(append(wasmName("run"), 0x00, 0x05)))) // func idx 5 (after 5 imports)

	var body bytes.Buffer
	body.Write(uleb128(1)) // 1 local-decl entry
	body.Write(uleb128(3)) // count=3
	body.WriteByte(0x7f)   // i32 x3: pathLen, fd, n

	constI32 := func(v int32) { body.WriteByte(0x41); body.Write(sleb128(v)) }
	call := func(idx uint32) { body.WriteByte(0x10); body.Write(uleb128(idx)) }
	localGet := func(idx uint32) { body.WriteByte(0x20); body.Write(uleb128(idx)) }
	localSet := func(idx uint32) { body.WriteByte(0x21); body.Write(uleb128(idx)) }

	// arg_get(1, 0, 64) -> local1 (pathLen)
	constI32(1)
	constI32(0)
	constI32(64)
	call(0)
	localSet(1)

	// fs_open(0, pathLen) -> local2 (fd)
	constI32(0)
	localGet(1)
	call(1)
	localSet(2)

	// fs_read(fd, 128, 128) -> local3 (n)
	localGet(2)
	constI32(128)
	constI32(128)
	call(2)
	localSet(3)

	// fs_close(fd)
	localGet(2)
	call(3)

	// write_stdout(128, n)
	constI32(128)
	localGet(3)
	call(4)
	body.WriteByte(0x1a) // drop

	constI32(0)
	body.WriteByte(0x0b) // end

	funcEntry := append(uleb128(uint32(body.Len())), body.Bytes()...)
	mod.Write(wasmSection(10, wasmVec(funcEntry)))

	return mod.Bytes()
}

func TestCustomPluginWritesStdout(t *testing.T) {
	var buf bytes.Buffer
	s := newTestShell(t, &buf,
		shell.WithWASMEnabled(true),
		shell.WithPluginBytes("mycmd", buildCustomPluginModule("hello from custom plugin\n")),
	)
	defer s.Close()

	if err := s.Run(context.Background(), "mycmd"); err != nil {
		t.Fatalf("mycmd: %v", err)
	}
	if got := buf.String(); got != "hello from custom plugin\n" {
		t.Errorf("output = %q", got)
	}
}

func TestCustomPluginDiscoveredFromVirtualFS(t *testing.T) {
	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/memsh/plugins", 0o755); err != nil {
		t.Fatal(err)
	}
	wasmBytes := buildCustomPluginModule("discovered via virtual fs\n")
	if err := afero.WriteFile(fs, "/memsh/plugins/discovered.wasm", wasmBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	s := newTestShell(t, &buf, shell.WithWASMEnabled(true), shell.WithFS(fs))
	defer s.Close()

	if err := s.Run(context.Background(), "discovered"); err != nil {
		t.Fatalf("discovered: %v", err)
	}
	if got := buf.String(); got != "discovered via virtual fs\n" {
		t.Errorf("output = %q", got)
	}
}

func TestCustomPluginFileDescriptorLifecycle(t *testing.T) {
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, "/data.txt", []byte("fd table round trip"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	s := newTestShell(t, &buf,
		shell.WithWASMEnabled(true),
		shell.WithFS(fs),
		shell.WithPluginBytes("readfile", buildCustomPluginModuleFSRead()),
	)
	defer s.Close()

	if err := s.Run(context.Background(), "readfile /data.txt"); err != nil {
		t.Fatalf("readfile: %v", err)
	}
	if got := buf.String(); got != "fd table round trip" {
		t.Errorf("output = %q", got)
	}
}

// --- WASI fixture, compiled on the fly ---

var (
	wasiFixtureOnce  sync.Once
	wasiFixtureBytes []byte
	wasiFixtureErr   error
)

// compileWASIFixture builds a tiny wasip1 Go program once per test binary
// run and caches the resulting bytes. It requires only the local Go
// toolchain's bundled wasip1 stdlib support — no network access.
func compileWASIFixture(t *testing.T) []byte {
	t.Helper()
	wasiFixtureOnce.Do(func() {
		if _, err := exec.LookPath("go"); err != nil {
			wasiFixtureErr = err
			return
		}
		dir := t.TempDir()
		src := `package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: fixture <path>")
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "read error:", err)
		os.Exit(1)
	}
	if err := os.WriteFile("/output.txt", []byte("processed: "+string(data)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write error:", err)
		os.Exit(1)
	}
	fmt.Print(string(data))
}
`
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
			wasiFixtureErr = err
			return
		}
		gomod := "module wasifixture\n\ngo 1.23\n"
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
			wasiFixtureErr = err
			return
		}
		out := filepath.Join(dir, "fixture.wasm")
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
		if outBytes, err := cmd.CombinedOutput(); err != nil {
			wasiFixtureErr = fmt.Errorf("build wasip1 fixture: %w: %s", err, outBytes)
			return
		}
		data, err := os.ReadFile(out)
		if err != nil {
			wasiFixtureErr = err
			return
		}
		wasiFixtureBytes = data
	})
	if wasiFixtureErr != nil {
		t.Skipf("skipping: could not build wasip1 fixture: %v", wasiFixtureErr)
	}
	return wasiFixtureBytes
}

func TestWASIPluginRunsAndTouchesVirtualFS(t *testing.T) {
	wasmBytes := compileWASIFixture(t)

	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, "/input.txt", []byte("wasi content"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	s := newTestShell(t, &buf,
		shell.WithWASMEnabled(true),
		shell.WithFS(fs),
		shell.WithPluginBytes("wasicmd", wasmBytes),
	)
	defer s.Close()

	if err := s.Run(context.Background(), "wasicmd /input.txt"); err != nil {
		t.Fatalf("wasicmd: %v", err)
	}
	if got := buf.String(); got != "wasi content" {
		t.Errorf("stdout = %q, want %q", got, "wasi content")
	}

	written, err := afero.ReadFile(fs, "/output.txt")
	if err != nil {
		t.Fatalf("reading /output.txt written by wasm: %v", err)
	}
	if string(written) != "processed: wasi content" {
		t.Errorf("/output.txt = %q", written)
	}
}

func TestWASIPluginResolvesRelativePathArg(t *testing.T) {
	wasmBytes := compileWASIFixture(t)

	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, "/input.txt", []byte("relative path content"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	s := newTestShell(t, &buf,
		shell.WithWASMEnabled(true),
		shell.WithFS(fs),
		shell.WithCwd("/"),
		shell.WithPluginBytes("wasicmd", wasmBytes),
	)
	defer s.Close()

	// Relative arg should be resolved against cwd by runWASIPlugin.
	if err := s.Run(context.Background(), "wasicmd input.txt"); err != nil {
		t.Fatalf("wasicmd: %v", err)
	}
	if got := buf.String(); got != "relative path content" {
		t.Errorf("stdout = %q, want %q", got, "relative path content")
	}
}

func TestWASIPluginNonZeroExitReturnsError(t *testing.T) {
	wasmBytes := compileWASIFixture(t)

	var buf bytes.Buffer
	s := newTestShell(t, &buf,
		shell.WithWASMEnabled(true),
		shell.WithFS(afero.NewMemMapFs()),
		shell.WithPluginBytes("wasicmd", wasmBytes),
	)
	defer s.Close()

	err := s.Run(context.Background(), "wasicmd /nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for missing input file")
	}
	if !strings.Contains(err.Error(), "exit code") {
		t.Errorf("error = %v, want it to mention exit code", err)
	}
}
