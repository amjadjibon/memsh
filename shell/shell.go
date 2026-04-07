package shell

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/amjadjibon/memsh/shell/plugins"
	"github.com/spf13/afero"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

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

	wasmEnabled          bool
	allowExternalCmds    bool
	inheritEnv           bool

	pluginFilter map[string]struct{}

	rt       wazero.Runtime
	compiled map[string]wazero.CompiledModule

	aliases     map[string]string
	sourceDepth int
	realCwd     string
}

type shellEnviron struct {
	env    map[string]string
	parent expand.Environ
}

func newShellEnviron(env map[string]string, inheritEnv bool) *shellEnviron {
	var parent expand.Environ
	if inheritEnv {
		parent = expand.ListEnviron(os.Environ()...)
	} else {
		parent = expand.ListEnviron()
	}
	return &shellEnviron{env: env, parent: parent}
}

func (e *shellEnviron) Get(name string) expand.Variable {
	if v, ok := e.env[name]; ok {
		return expand.Variable{Exported: true, Kind: expand.String, Str: v}
	}
	return e.parent.Get(name)
}

func (e *shellEnviron) Each(fn func(name string, vr expand.Variable) bool) {
	e.parent.Each(fn)
	for name, v := range e.env {
		if !fn(name, expand.Variable{Exported: true, Kind: expand.String, Str: v}) {
			return
		}
	}
}

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
		aliases:     make(map[string]string),
		wasmEnabled: true,
		inheritEnv:  true,
		realCwd:     "/",
	}

	for _, p := range defaultNativePlugins() {
		s.builtins[p.Name()] = p.Run
	}

	for _, opt := range opts {
		opt(s)
	}

	realCwd := s.cwd
	if _, err := os.Stat(realCwd); err != nil {
		tmp, err := os.MkdirTemp("", "memsh-cwd-*")
		if err != nil {
			return nil, fmt.Errorf("memsh: create temp cwd: %w", err)
		}
		s.realCwd = tmp
	} else {
		s.realCwd = realCwd
	}

	shellEnv := newShellEnviron(s.env, s.inheritEnv)

	runner, err := interp.New(
		interp.StdIO(s.stdin, s.stdout, s.stderr),
		interp.Env(shellEnv),
		interp.Dir(s.realCwd),
		interp.OpenHandler(s.openHandler),
		interp.ReadDirHandler2(s.readDirHandler),
		interp.StatHandler(s.statHandler),
		interp.ExecHandlers(s.execHandler),
		interp.Interactive(true), // enable alias expansion
	)
	if err != nil {
		return nil, err
	}
	s.runner = runner

	if s.wasmEnabled {
		for name, wasm := range defaultPlugins {
			if _, exists := s.plugins[name]; !exists {
				s.plugins[name] = wasm
			}
		}

		if err := s.loadPlugins(); err != nil {
			return nil, err
		}

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

	// Pre-seed aliases from WithAliases option by running alias commands.
	if len(s.aliases) > 0 {
		var initScript strings.Builder
		for name, value := range s.aliases {
			// Use single-quote wrapping; escape any single quotes in value.
			escaped := strings.ReplaceAll(value, "'", `'\''`)
			fmt.Fprintf(&initScript, "alias %s='%s'\n", name, escaped)
		}
		if initErr := s.Run(context.Background(), initScript.String()); initErr != nil {
			return nil, fmt.Errorf("WithAliases init: %w", initErr)
		}
	}

	return s, nil
}

func (s *Shell) Close() error {
	if s.rt != nil {
		return s.rt.Close(context.Background())
	}
	return nil
}

func (s *Shell) Register(p plugins.Plugin) {
	s.builtins[p.Name()] = p.Run
}

func (s *Shell) Cwd() string {
	return s.cwd
}

func (s *Shell) FS() afero.Fs {
	return s.fs
}

func (s *Shell) ListDir(path string) ([]string, error) {
	abs := s.resolvePath(path)
	f, err := s.fs.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Readdirnames(-1)
}

// LoadMemshrc sources /.memshrc from the virtual filesystem if it exists.
// Errors are non-fatal — a missing file is silently ignored.
func (s *Shell) LoadMemshrc(ctx context.Context) error {
	data, err := afero.ReadFile(s.fs, "/.memshrc")
	if err != nil {
		return nil
	}
	return s.Run(ctx, string(data))
}

func (s *Shell) RegisteredPlugins() []string {
	names := make([]string, 0, len(s.plugins))
	for name := range s.plugins {
		names = append(names, name)
	}
	return names
}

func (s *Shell) Run(ctx context.Context, script string) error {
	file, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	if err != nil {
		return err
	}
	err = s.runner.Run(ctx, file)
	s.cwd = s.runner.Dir
	return err
}

func (s *Shell) readDirHandler(ctx context.Context, path string) ([]fs.DirEntry, error) {
	absPath := path
	if !filepath.IsAbs(path) {
		absPath = filepath.Join(s.cwd, path)
	}

	f, err := s.fs.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	infos, err := f.Readdir(-1)
	if err != nil {
		return nil, err
	}

	entries := make([]fs.DirEntry, len(infos))
	for i, info := range infos {
		entries[i] = fs.FileInfoToDirEntry(info)
	}
	return entries, nil
}

func (s *Shell) statHandler(_ context.Context, name string, _ bool) (fs.FileInfo, error) {
	absPath := name
	if !filepath.IsAbs(name) {
		absPath = filepath.Join(s.cwd, name)
	}
	return s.fs.Stat(absPath)
}
