package shell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/spf13/afero"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

var ErrExit = errors.New("exit")

// maxSourceDepth limits recursive `source` calls to prevent stack overflow.
const maxSourceDepth = 16

type Shell struct {
	fs  afero.Fs
	cwd string
	env map[string]string

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	runner        *interp.Runner
	plugins       pluginRegistry
	builtins      map[string]BuiltinFunc
	nativePlugins map[string]plugins.Plugin
	fds           map[uint32]afero.File
	nextFd        uint32
	freeFds       []uint32

	wasmEnabled       bool
	allowExternalCmds bool
	inheritEnv        bool

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
		fs:            afero.NewMemMapFs(),
		cwd:           "/",
		env:           make(map[string]string),
		stdin:         os.Stdin,
		stdout:        os.Stdout,
		stderr:        os.Stderr,
		plugins:       make(pluginRegistry),
		builtins:      make(map[string]BuiltinFunc),
		nativePlugins: make(map[string]plugins.Plugin),
		fds:           make(map[uint32]afero.File),
		nextFd:        3,
		compiled:      make(map[string]wazero.CompiledModule),
		aliases:       make(map[string]string),
		wasmEnabled:   true,
		inheritEnv:    true,
		realCwd:       "/",
	}

	for _, p := range defaultNativePlugins() {
		s.Register(p)
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
		interp.AccessHandler(s.accessHandler),
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

func (s *Shell) execHandler(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		if len(args) == 0 {
			return nil
		}

		ctx = plugins.WithShellContext(ctx, plugins.ShellContext{
			FS:          s.fs,
			Cwd:         s.cwd,
			Env:         func(key string) string { return s.env[key] },
			EnvAll:      func() map[string]string { return maps.Clone(s.env) },
			SetEnv:      func(key, value string) { s.env[key] = value },
			ResolvePath: s.resolvePath,
			SetCwd:      s.changeDir,
			Run:         s.Run,
			Exec:        s.execArgs,
			Exit:        func() error { return ErrExit },
			AliasLookup: func(name string) (string, bool) {
				v, ok := s.aliases[name]
				return v, ok
			},
			CommandInfo:  s.commandInfo,
			CommandNames: s.Commands,
			SourceFile:   s.sourceFile,
		})

		if fn, ok := s.builtins[args[0]]; ok {
			return fn(ctx, args)
		}
		if _, ok := s.plugins[args[0]]; ok {
			return s.runPlugin(ctx, args[0], args)
		}
		if s.allowExternalCmds {
			return next(ctx, args)
		}
		return fmt.Errorf("%s: command not found", args[0])
	}
}

func (s *Shell) execArgs(ctx context.Context, args []string) error {
	notFound := func(_ context.Context, args []string) error {
		return fmt.Errorf("%s: command not found", args[0])
	}
	return s.execHandler(notFound)(ctx, args)
}

func (s *Shell) changeDir(dir string) error {
	target := s.resolvePath(dir)

	info, err := s.fs.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cd: %s: No such file or directory", dir)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("cd: %s: Not a directory", dir)
	}

	s.cwd = target
	s.runner.Dir = target
	return nil
}

func (s *Shell) sourceFile(ctx context.Context, path string) error {
	s.sourceDepth++
	defer func() { s.sourceDepth-- }()
	if s.sourceDepth > maxSourceDepth {
		return fmt.Errorf("source: maximum recursion depth (%d) exceeded", maxSourceDepth)
	}

	var data []byte
	var err error
	if strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://") {
		data, err = sourceURL(ctx, path)
	} else {
		data, err = afero.ReadFile(s.fs, s.resolvePath(path))
	}
	if err != nil {
		return fmt.Errorf("source: %s: %w", path, err)
	}
	return s.Run(ctx, string(data))
}

// sourceURL fetches a script from a URL and returns its contents.
// The response body is capped at 10 MiB to prevent memory exhaustion.
func sourceURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "memsh/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}

	const maxSize = 10 << 20 // 10 MiB
	return io.ReadAll(io.LimitReader(resp.Body, maxSize))
}

func (s *Shell) Register(p plugins.Plugin) {
	s.builtins[p.Name()] = p.Run
	s.nativePlugins[p.Name()] = p
}

func (s *Shell) Cwd() string {
	return s.cwd
}

func (s *Shell) FS() afero.Fs {
	return s.fs
}

// Commands returns all command names known to this shell instance:
// the static builtin list, registered native plugins, and loaded WASM plugins.
func (s *Shell) Commands() []string {
	seen := make(map[string]bool, len(s.builtins)+len(s.compiled))
	var names []string
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	for n := range s.builtins {
		add(n)
	}
	for n := range s.compiled {
		add(n)
	}
	sort.Strings(names)
	return names
}

// DefaultCommands returns all command names available in a default shell
// (static builtins + default native plugins) without creating a Shell instance.
func DefaultCommands() []string {
	seen := make(map[string]bool, len(defaultNativePlugins())+8)
	var names []string
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	for _, p := range defaultNativePlugins() {
		add(p.Name())
	}
	sort.Strings(names)
	return names
}

func (s *Shell) commandInfo(name string) (plugins.CommandInfo, bool) {
	if p, ok := s.nativePlugins[name]; ok {
		info := plugins.CommandInfo{Kind: "native plugin"}
		if pi, ok := p.(plugins.PluginInfo); ok {
			info.Description = pi.Description()
			info.Usage = pi.Usage()
		}
		return info, true
	}
	if _, ok := s.builtins[name]; ok {
		return plugins.CommandInfo{Kind: "native plugin"}, true
	}
	if _, ok := s.plugins[name]; ok {
		return plugins.CommandInfo{Kind: "WASM plugin"}, true
	}
	return plugins.CommandInfo{}, false
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

func (s *Shell) accessHandler(_ context.Context, path string, mode uint32) error {
	absPath := path
	if !filepath.IsAbs(path) {
		absPath = filepath.Join(s.cwd, path)
	}

	info, err := s.fs.Stat(absPath)
	if err != nil {
		return err
	}

	m := info.Mode()
	switch mode {
	case 0x1: // X_OK
		if m&0o111 == 0 {
			return &os.PathError{Op: "access", Path: path, Err: fmt.Errorf("file is not executable")}
		}
	case 0x2: // W_OK
		if m&0o222 == 0 {
			return &os.PathError{Op: "access", Path: path, Err: fmt.Errorf("file is not writable")}
		}
	case 0x4: // R_OK
		if m&0o444 == 0 {
			return &os.PathError{Op: "access", Path: path, Err: fmt.Errorf("file is not readable")}
		}
	}
	return nil
}
