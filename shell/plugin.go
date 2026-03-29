package shell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
	"mvdan.cc/sh/v3/interp"
)

// pluginRegistry maps command name → raw WASM bytes.
type pluginRegistry map[string][]byte

// loadPlugins walks /memsh/plugins/ in the MemMapFs and the real ~/.memsh/plugins/
// directory, registering each .wasm file by its filename stem as the command name.
// Non-fatal if either directory is absent.
func (s *Shell) loadPlugins() error {
	const virtualDir = "/memsh/plugins"

	// Virtual FS plugins.
	if info, err := s.fs.Stat(virtualDir); err == nil && info.IsDir() {
		if err := afero.Walk(s.fs, virtualDir, func(path string, info fs.FileInfo, err error) error {
			if err != nil || info.IsDir() || filepath.Ext(path) != ".wasm" {
				return err
			}
			name := strings.TrimSuffix(filepath.Base(path), ".wasm")
			if _, exists := s.plugins[name]; !exists {
				data, err := afero.ReadFile(s.fs, path)
				if err != nil {
					return fmt.Errorf("plugin: read %s: %w", path, err)
				}
				s.plugins[name] = data
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Real FS plugins (~/.memsh/plugins/).
	realDir, err := realPluginDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(realDir)
	if err != nil {
		return nil // directory absent — not an error
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".wasm" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".wasm")
		if _, exists := s.plugins[name]; !exists {
			data, err := os.ReadFile(filepath.Join(realDir, e.Name()))
			if err != nil {
				return fmt.Errorf("plugin: read %s: %w", e.Name(), err)
			}
			s.plugins[name] = data
		}
	}
	return nil
}

// realPluginDir returns ~/.memsh/plugins on the real OS filesystem.
func realPluginDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".memsh", "plugins"), nil
}

// runPlugin executes a registered WASM plugin via a fresh wazero instance.
// It detects whether the module is a standard WASI program (_start export) or a
// custom memsh plugin (run export) and handles each case accordingly.
func (s *Shell) runPlugin(ctx context.Context, name string, args []string) error {
	wasmBytes := s.plugins[name]
	hc := interp.HandlerCtx(ctx)

	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)

	// Compile to inspect exports without executing.
	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("plugin %s: compile: %w", name, err)
	}

	exports := compiled.ExportedFunctions()
	_, hasStart := exports["_start"]
	_, hasRun := exports["run"]

	if hasStart && !hasRun {
		return s.runWASIPlugin(ctx, rt, compiled, hc, args, name)
	}
	return s.runCustomPlugin(ctx, rt, compiled, hc, args, name)
}

// runWASIPlugin runs a standard WASI module (_start is called during Instantiate).
func (s *Shell) runWASIPlugin(ctx context.Context, rt wazero.Runtime, compiled wazero.CompiledModule, hc interp.HandlerContext, args []string, name string) error {
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	modConfig := wazero.NewModuleConfig().
		WithStdin(nopCloser{hc.Stdin}).
		WithStdout(nopWriteCloser{hc.Stdout}).
		WithStderr(nopWriteCloser{hc.Stderr}).
		WithArgs(args...).
		WithName("")

	_, err := rt.InstantiateModule(ctx, compiled, modConfig)
	if err != nil {
		var exitErr *sys.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() != 0 {
				return fmt.Errorf("plugin %s: exit code %d", name, exitErr.ExitCode())
			}
			return nil
		}
		return fmt.Errorf("plugin %s: %w", name, err)
	}
	return nil
}

// runCustomPlugin runs a memsh-native plugin (exports run(argc) and uses memsh:: host functions).
func (s *Shell) runCustomPlugin(ctx context.Context, rt wazero.Runtime, compiled wazero.CompiledModule, hc interp.HandlerContext, args []string, name string) error {
	_, err := rt.NewHostModuleBuilder("memsh").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, ptr, length uint32) int32 {
			buf, ok := m.Memory().Read(ptr, length)
			if !ok {
				return -1
			}
			n, _ := hc.Stdout.Write(buf)
			return int32(n)
		}).Export("write_stdout").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, ptr, length uint32) int32 {
			buf, ok := m.Memory().Read(ptr, length)
			if !ok {
				return -1
			}
			n, _ := hc.Stderr.Write(buf)
			return int32(n)
		}).Export("write_stderr").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, ptr, length uint32) int32 {
			if hc.Stdin == nil {
				return 0
			}
			buf, ok := m.Memory().Read(ptr, length)
			if !ok {
				return -1
			}
			n, _ := hc.Stdin.Read(buf)
			return int32(n)
		}).Export("read_stdin").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, index, ptr, length uint32) int32 {
			if int(index) >= len(args) {
				return -1
			}
			arg := args[index]
			if !m.Memory().WriteString(ptr, arg) {
				return -1
			}
			return int32(len(arg))
		}).Export("arg_get").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, kPtr, kLen, vPtr, vLen uint32) int32 {
			key, ok := m.Memory().Read(kPtr, kLen)
			if !ok {
				return -1
			}
			val := s.env[string(key)]
			if uint32(len(val)) > vLen {
				return -1
			}
			m.Memory().WriteString(vPtr, val)
			return int32(len(val))
		}).Export("env_get").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, ptr, length uint32) int32 {
			cwd := s.cwd
			if uint32(len(cwd)) > length {
				return -1
			}
			m.Memory().WriteString(ptr, cwd)
			return int32(len(cwd))
		}).Export("get_cwd").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, ptr, length uint32) int32 {
			pathBytes, ok := m.Memory().Read(ptr, length)
			if !ok {
				return -1
			}
			f, err := s.fs.Open(s.resolvePath(string(pathBytes)))
			if err != nil {
				return -1
			}
			return int32(s.allocFd(f))
		}).Export("fs_open").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, fd, ptr, length uint32) int32 {
			f := s.getFd(fd)
			if f == nil {
				return -1
			}
			buf, ok := m.Memory().Read(ptr, length)
			if !ok {
				return -1
			}
			n, _ := f.Read(buf)
			return int32(n)
		}).Export("fs_read").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, fd, ptr, length uint32) int32 {
			f := s.getFd(fd)
			if f == nil {
				return -1
			}
			buf, ok := m.Memory().Read(ptr, length)
			if !ok {
				return -1
			}
			n, _ := f.Write(buf)
			return int32(n)
		}).Export("fs_write").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, fd uint32) {
			s.closeFd(fd)
		}).Export("fs_close").
		Instantiate(ctx)
	if err != nil {
		return fmt.Errorf("plugin %s: host module: %w", name, err)
	}

	mod, err := rt.InstantiateModule(ctx, compiled,
		wazero.NewModuleConfig().WithStartFunctions().WithName(""))
	if err != nil {
		return fmt.Errorf("plugin %s: instantiate: %w", name, err)
	}
	defer mod.Close(ctx)

	run := mod.ExportedFunction("run")
	if run == nil {
		return fmt.Errorf("plugin %s: missing export 'run'", name)
	}
	results, err := run.Call(ctx, uint64(len(args)))
	if err != nil {
		if strings.Contains(err.Error(), "exit_code") {
			return nil
		}
		return fmt.Errorf("plugin %s: %w", name, err)
	}
	if len(results) > 0 && results[0] != 0 {
		return fmt.Errorf("plugin %s: exit code %d", name, results[0])
	}
	return nil
}

// nopCloser wraps an io.Reader and swallows Close so wazero cannot close
// the underlying stream (e.g. os.Stdin) when the runtime is torn down.
type nopCloser struct{ io.Reader }

func (nopCloser) Close() error { return nil }

// nopWriteCloser does the same for io.Writer.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// fd table helpers.

func (s *Shell) allocFd(f afero.File) uint32 {
	for i := uint32(3); ; i++ {
		if _, exists := s.fds[i]; !exists {
			s.fds[i] = f
			return i
		}
	}
}

func (s *Shell) getFd(fd uint32) afero.File {
	return s.fds[fd]
}

func (s *Shell) closeFd(fd uint32) {
	if f, ok := s.fds[fd]; ok {
		f.Close()
		delete(s.fds, fd)
	}
}
