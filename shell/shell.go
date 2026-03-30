package shell

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/amjadjibon/memsh/shell/plugins"
	"github.com/spf13/afero"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Shell represents the virtual bash session.
type Shell struct {
	fs  afero.Fs
	cwd string
	env map[string]string

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	runner   *interp.Runner
	plugins  pluginRegistry
	builtins map[string]BuiltinFunc
	fds      map[uint32]afero.File

	// wasmEnabled controls whether the wazero runtime is started.
	// Defaults to true; set via WithWASMEnabled(false) to skip WASM entirely.
	wasmEnabled bool

	// pluginFilter is an optional allowlist for plugin discovery.
	// When non-nil, only plugins whose names are in the map are loaded
	// from /memsh/plugins/ and ~/.memsh/plugins/.
	pluginFilter map[string]struct{}

	// wazero runtime shared across all plugin invocations.
	// CompiledModules are pre-compiled at startup to avoid per-call bytecode compilation.
	rt       wazero.Runtime
	compiled map[string]wazero.CompiledModule
}

// New creates a new Shell instance with the provided options.
func New(opts ...Option) (*Shell, error) {
	s := &Shell{
		fs:          afero.NewMemMapFs(),
		cwd:         "/",
		env:         make(map[string]string),
		stdin:       os.Stdin,
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		plugins:     make(pluginRegistry),
		builtins:    make(map[string]BuiltinFunc),
		fds:         make(map[uint32]afero.File),
		compiled:    make(map[string]wazero.CompiledModule),
		wasmEnabled: true,
	}

	// Register default native plugins. Options applied below may override.
	for _, p := range defaultNativePlugins() {
		s.builtins[p.Name()] = p.Run
	}

	for _, opt := range opts {
		opt(s)
	}

	// Initialize the runner.
	runner, err := interp.New(
		interp.StdIO(s.stdin, s.stdout, s.stderr),
		interp.OpenHandler(s.openHandler),
		interp.ExecHandlers(s.execHandler),
		interp.Dir(s.cwd),
	)
	if err != nil {
		return nil, err
	}
	s.runner = runner

	if s.wasmEnabled {
		// Register built-in WASM plugins (don't overwrite user-supplied ones).
		for name, wasm := range defaultPlugins {
			if _, exists := s.plugins[name]; !exists {
				s.plugins[name] = wasm
			}
		}

		if err := s.loadPlugins(); err != nil {
			return nil, err
		}

		// Only pay the wazero startup cost when there are WASM plugins to run.
		// No plugins → skip runtime init entirely (fast startup).
		if len(s.plugins) > 0 {
			initCtx := context.Background()
			s.rt = wazero.NewRuntime(initCtx)
			wasi_snapshot_preview1.MustInstantiate(initCtx, s.rt)

			for name, wasm := range s.plugins {
				cm, err := s.rt.CompileModule(initCtx, wasm)
				if err != nil {
					_ = s.rt.Close(initCtx)
					return nil, fmt.Errorf("plugin %s: compile: %w", name, err)
				}
				s.compiled[name] = cm
			}
		}
	}

	return s, nil
}

// Close releases wazero resources. Call when the shell is no longer needed.
func (s *Shell) Close() error {
	if s.rt != nil {
		return s.rt.Close(context.Background())
	}
	return nil
}

// Register adds a native Plugin to the shell. If a plugin with the same name
// already exists it is replaced. Safe to call after New().
func (s *Shell) Register(p plugins.Plugin) {
	s.builtins[p.Name()] = p.Run
}

// Cwd returns the current working directory of the shell.
func (s *Shell) Cwd() string {
	return s.cwd
}

// ListDir lists entry names in a directory of the virtual FS.
func (s *Shell) ListDir(path string) ([]string, error) {
	abs := s.resolvePath(path)
	f, err := s.fs.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Readdirnames(-1)
}

// RegisteredPlugins returns the names of all currently registered plugins.
func (s *Shell) RegisteredPlugins() []string {
	names := make([]string, 0, len(s.plugins))
	for name := range s.plugins {
		names = append(names, name)
	}
	return names
}

// Run executes a unified shell script string.
func (s *Shell) Run(ctx context.Context, script string) error {
	file, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	if err != nil {
		return err
	}
	return s.runner.Run(ctx, file)
}
