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
