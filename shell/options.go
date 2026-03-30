package shell

import (
	"context"
	"io"

	"github.com/amjadjibon/memsh/shell/plugins"
	"github.com/spf13/afero"
)

// Option is a configuration function for the Shell.
type Option func(*Shell)

// BuiltinFunc is a native Go command implementation.
// ctx carries the interp.HandlerContext — use interp.HandlerCtx(ctx) for per-command I/O.
// For virtual FS access use shell.ShellCtx(ctx).
type BuiltinFunc func(ctx context.Context, args []string) error

// WithPlugin registers a Plugin as a native shell command.
// Native plugins take priority over WASM plugins with the same name.
func WithPlugin(p plugins.Plugin) Option {
	return func(s *Shell) {
		s.builtins[p.Name()] = p.Run
	}
}

// WithBuiltin registers a raw function as a native shell command.
// Prefer WithPlugin when you want to attach metadata (description, usage).
func WithBuiltin(name string, fn BuiltinFunc) Option {
	return func(s *Shell) {
		s.builtins[name] = fn
	}
}

// WithFS sets the afero Filesystem to use.
func WithFS(fs afero.Fs) Option {
	return func(s *Shell) {
		s.fs = fs
	}
}

// WithCwd sets the initial working directory.
func WithCwd(cwd string) Option {
	return func(s *Shell) {
		s.cwd = cwd
	}
}

// WithEnv sets initial environment variables.
func WithEnv(env map[string]string) Option {
	return func(s *Shell) {
		for k, v := range env {
			s.env[k] = v
		}
	}
}

// WithPluginBytes registers a WASM plugin directly from bytes, without needing
// a file in /memsh/plugins/. The plugin must export command_name() and run().
func WithPluginBytes(name string, wasm []byte) Option {
	return func(s *Shell) {
		s.plugins[name] = wasm
	}
}

// WithStdIO sets the standard input, output, and error streams.
func WithStdIO(in io.Reader, out, err io.Writer) Option {
	return func(s *Shell) {
		s.stdin = in
		s.stdout = out
		s.stderr = err
	}
}

// WithWASMEnabled controls whether the wazero WASM plugin runtime is started.
// Pass false to skip all WASM plugin loading (faster startup, no wazero overhead).
func WithWASMEnabled(enabled bool) Option {
	return func(s *Shell) {
		s.wasmEnabled = enabled
	}
}

// WithPluginFilter sets an allowlist of WASM plugin names to load during
// discovery (/memsh/plugins/ and ~/.memsh/plugins/).
// When the list is non-empty, only plugins whose names appear in it are loaded.
// Plugins registered explicitly via WithPlugin or WithPluginBytes are unaffected.
func WithPluginFilter(names []string) Option {
	return func(s *Shell) {
		s.pluginFilter = make(map[string]struct{}, len(names))
		for _, n := range names {
			s.pluginFilter[n] = struct{}{}
		}
	}
}

// WithDisabledPlugins removes the named plugins from the shell.
// Works for both native (builtin) and WASM plugins.
// Applied after defaults, so it can suppress defaultNativePlugins entries.
func WithDisabledPlugins(names ...string) Option {
	return func(s *Shell) {
		for _, name := range names {
			delete(s.builtins, name)
			delete(s.plugins, name)
		}
	}
}
