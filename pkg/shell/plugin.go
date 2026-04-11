package shell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental/sysfs"
	"github.com/tetratelabs/wazero/sys"
	"mvdan.cc/sh/v3/interp"
)

// Plugin loading priority (first registration wins):
//
//  1. WithPluginBytes / WithPlugin options — caller-supplied bytes or compiled modules.
//  2. defaultNativePlugins() — built-in Go plugins registered at Shell construction.
//  3. defaultPlugins map — embedded WASM bytes bundled with the binary (currently empty).
//  4. /memsh/plugins/*.wasm inside the virtual afero.MemMapFs.
//  5. ~/.memsh/plugins/*.wasm on the real OS filesystem (loaded by loadPlugins).
//
// Because the map is checked for existence before inserting, the first source that
// provides a name wins and later sources are silently skipped.

// sysfsConfig casts wazero.FSConfig to sysfs.FSConfig so WithSysFSMount is available.
func sysfsConfig(cfg wazero.FSConfig) sysfs.FSConfig {
	return cfg.(sysfs.FSConfig)
}

// WASMConfig holds WASM plugin bytes and optional runtime tweaks.
type WASMConfig struct {
	// Bytes is the raw WASM module.
	Bytes []byte
	// ExtraArgs are flags to prepend after the command name (e.g. ["-W0"]).
	// Existing flags are checked before insertion to avoid duplicates.
	ExtraArgs []string
	// ExtraEnv are environment variables to set in the WASI module.
	// Format: ["KEY=value", ...]
	ExtraEnv []string
}

// pluginRegistry maps command name → WASM config.
type pluginRegistry map[string]WASMConfig

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
			if !s.pluginAllowed(name) {
				return nil
			}
			if _, exists := s.plugins[name]; !exists {
				data, err := afero.ReadFile(s.fs, path)
				if err != nil {
					return fmt.Errorf("plugin: read %s: %w", path, err)
				}
				s.plugins[name] = wasmConfigForPlugin(name, data)
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
		if !s.pluginAllowed(name) {
			continue
		}
		if _, exists := s.plugins[name]; !exists {
			data, err := os.ReadFile(filepath.Join(realDir, e.Name()))
			if err != nil {
				return fmt.Errorf("plugin: read %s: %w", e.Name(), err)
			}
			s.plugins[name] = wasmConfigForPlugin(name, data)
		}
	}
	return nil
}

// wasmConfigForPlugin returns a WASMConfig with plugin-specific tweaks.
// For most plugins this is just the raw bytes, but some runtimes require
// extra flags or environment variables to suppress warnings.
func wasmConfigForPlugin(name string, bytes []byte) WASMConfig {
	switch name {
	case "ruby":
		return WASMConfig{
			Bytes:     bytes,
			ExtraArgs: []string{"-W0"},
			ExtraEnv:  []string{"RUBYOPT=-W0"},
		}
	default:
		return WASMConfig{Bytes: bytes}
	}
}

// pluginAllowed reports whether a discovered plugin name passes the filter.
// When no filter is set all names are allowed.
func (s *Shell) pluginAllowed(name string) bool {
	if len(s.pluginFilter) == 0 {
		return true
	}
	_, ok := s.pluginFilter[name]
	return ok
}

// realPluginDir returns ~/.memsh/plugins on the real OS filesystem.
func realPluginDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".memsh", "plugins"), nil
}

// runPlugin executes a registered WASM plugin using the shared wazero runtime.
// CompiledModules are pre-compiled at startup; this only instantiates the module.
// If a plugin was added after startup it is compiled on first use and cached.
func (s *Shell) runPlugin(ctx context.Context, name string, args []string) error {
	cm, ok := s.compiled[name]
	if !ok {
		// Plugin added after startup (e.g. dropped into virtual FS at runtime) — compile and cache.
		wasmCfg, exists := s.plugins[name]
		if !exists {
			return fmt.Errorf("plugin %s: not found", name)
		}
		var err error
		cm, err = s.rt.CompileModule(ctx, wasmCfg.Bytes)
		if err != nil {
			return fmt.Errorf("plugin %s: compile: %w", name, err)
		}
		s.compiled[name] = cm
	}

	hc := interp.HandlerCtx(ctx)
	exports := cm.ExportedFunctions()
	_, hasStart := exports["_start"]
	_, hasRun := exports["run"]

	if hasStart && !hasRun {
		return s.runWASIPlugin(ctx, cm, hc, args, name)
	}
	return s.runCustomPlugin(ctx, cm, hc, args, name)
}

// runWASIPlugin runs a standard WASI module (_start is called during Instantiate).
// WASI snapshot support is already registered on s.rt at startup.
//
// We mount the virtual FS directly via the experimental sysfs API so that
// WASI writes go straight into afero.MemMapFs — no temp-dir bridge, no races.
func (s *Shell) runWASIPlugin(ctx context.Context, compiled wazero.CompiledModule, hc interp.HandlerContext, args []string, name string) error {
	cfg, ok := s.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %s: config not found", name)
	}

	// Resolve relative path args to absolute virtual paths.
	// Skip arguments that follow flags like -c (Python), -e (Ruby), etc.
	resolved := make([]string, len(args))
	copy(resolved, args)
	skipNext := false
	for i := 1; i < len(resolved); i++ {
		if skipNext {
			skipNext = false
			continue
		}

		a := resolved[i]
		// Check if this is a flag that takes an argument
		if a == "-c" || a == "-e" || a == "-E" {
			skipNext = true
			continue
		}

		// Don't resolve args that start with - or /
		if len(a) > 0 && a[0] != '-' && a[0] != '/' {
			resolved[i] = path.Join(s.cwd, a)
		}
	}

	// Inject plugin-specific extra args (if not already present).
	if len(cfg.ExtraArgs) > 0 && len(resolved) > 0 {
		for _, extra := range cfg.ExtraArgs {
			// Check if this flag already exists in args
			alreadyPresent := false
			for _, arg := range resolved {
				if arg == extra {
					alreadyPresent = true
					break
				}
				// For flags like -W0, also check -W prefix
				if len(extra) >= 2 && extra[0] == '-' && len(arg) >= 2 && arg[0] == '-' && extra[1] == 'W' && arg[1] == 'W' {
					alreadyPresent = true
					break
				}
			}
			if !alreadyPresent {
				// Insert after the command name (index 0)
				newArgs := make([]string, len(resolved)+1)
				newArgs[0] = resolved[0]
				newArgs[1] = extra
				copy(newArgs[2:], resolved[1:])
				resolved = newArgs
			}
		}
	}

	fsConfig := sysfsConfig(wazero.NewFSConfig()).
		WithSysFSMount(aferoSysFS{vfs: s.fs}, "/")

	modConfig := wazero.NewModuleConfig().
		WithStdin(nopCloser{hc.Stdin}).
		WithStdout(nopWriteCloser{hc.Stdout}).
		WithStderr(nopWriteCloser{hc.Stderr}).
		WithArgs(resolved...).
		WithFSConfig(fsConfig).
		WithEnv("HOME", "/").
		WithEnv("PWD", s.cwd).
		WithEnv("PYTHONDONTWRITEBYTECODE", "1").
		WithName("")

	// Add plugin-specific environment variables.
	for _, envVar := range cfg.ExtraEnv {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) == 2 {
			modConfig = modConfig.WithEnv(parts[0], parts[1])
		}
	}

	_, runErr := s.rt.InstantiateModule(ctx, compiled, modConfig)
	if runErr != nil {
		var exitErr *sys.ExitError
		if errors.As(runErr, &exitErr) {
			if exitErr.ExitCode() != 0 {
				return fmt.Errorf("plugin %s: exit code %d", name, exitErr.ExitCode())
			}
			return nil
		}
		return fmt.Errorf("plugin %s: %w", name, runErr)
	}
	return nil
}

// runCustomPlugin runs a memsh-native plugin (exports run(argc) and uses memsh:: host functions).
// A fresh memsh host module is instantiated per invocation and closed when done,
// so the per-call closures (hc, args) are properly scoped.
func (s *Shell) runCustomPlugin(ctx context.Context, compiled wazero.CompiledModule, hc interp.HandlerContext, args []string, name string) error {
	hostMod, err := s.rt.NewHostModuleBuilder("memsh").
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
	defer hostMod.Close(ctx)

	mod, err := s.rt.InstantiateModule(ctx, compiled,
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

func (s *Shell) allocFd(f afero.File) uint32 {
	var fd uint32
	if n := len(s.freeFds); n > 0 {
		fd = s.freeFds[n-1]
		s.freeFds = s.freeFds[:n-1]
	} else {
		fd = s.nextFd
		s.nextFd++
	}
	s.fds[fd] = f
	return fd
}

func (s *Shell) getFd(fd uint32) afero.File {
	return s.fds[fd]
}

func (s *Shell) closeFd(fd uint32) {
	if f, ok := s.fds[fd]; ok {
		f.Close()
		delete(s.fds, fd)
		s.freeFds = append(s.freeFds, fd)
	}
}
