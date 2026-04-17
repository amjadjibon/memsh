package shell

import (
	"context"
	"io"
	"maps"

	"github.com/amjadjibon/memsh/pkg/network"
	"github.com/amjadjibon/memsh/pkg/shell/plugins"
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
		s.nativePlugins[p.Name()] = p
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
		maps.Copy(s.env, env)
	}
}

// WithPluginBytes registers a WASM plugin directly from bytes, without needing
// a file in /memsh/plugins/. The plugin must export command_name() and run().
func WithPluginBytes(name string, wasm []byte) Option {
	return func(s *Shell) {
		s.plugins[name] = wasmConfigForPlugin(name, wasm)
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
//
// Precedence note: WithPluginFilter only gates filesystem discovery, which runs
// after all options are applied. WithDisabledPlugins removes already-registered
// native plugins during option application but does not prevent a same-named WASM
// plugin from being loaded later by discovery. If both options name the same plugin
// and you need to suppress it entirely, use WithDisabledPlugins and do NOT include
// that name in the filter list (i.e. leave it out of the allowlist).
func WithPluginFilter(names []string) Option {
	return func(s *Shell) {
		s.pluginFilter = make(map[string]struct{}, len(names))
		for _, n := range names {
			s.pluginFilter[n] = struct{}{}
		}
	}
}

// WithAllowExternalCommands permits falling back to real OS executables when a
// command is not found among builtins or plugins. By default this is false,
// which keeps all execution inside the virtual sandbox.
func WithAllowExternalCommands(allow bool) Option {
	return func(s *Shell) {
		s.allowExternalCmds = allow
	}
}

// WithInheritEnv controls whether the shell inherits the parent process's
// environment variables. When false, only explicitly set variables (via WithEnv)
// are available. Defaults to true for CLI use; should be false in server mode
// to prevent leaking host secrets to remote users.
func WithInheritEnv(inherit bool) Option {
	return func(s *Shell) {
		s.inheritEnv = inherit
	}
}

// WithAliases pre-seeds the alias table (e.g. loaded from a config file).
func WithAliases(aliases map[string]string) Option {
	return func(s *Shell) {
		maps.Copy(s.aliases, aliases)
	}
}

// WithNetworkPolicy sets outbound networking policy used by builtins/plugins
// that issue network requests (for example curl and source URL).
func WithNetworkPolicy(policy network.Policy) Option {
	return func(s *Shell) {
		s.networkPolicy = policy
		s.networkDialer = network.NewDialer(network.DialerConfig{
			Policy: policy,
			Meter:  s.networkMeter,
		})
	}
}

// WithNetworkLimits sets per-shell network usage limits.
func WithNetworkLimits(limits network.Limits) Option {
	return func(s *Shell) {
		s.networkMeter = network.NewMeter(limits)
		s.networkDialer = network.NewDialer(network.DialerConfig{
			Policy: s.networkPolicy,
			Meter:  s.networkMeter,
		})
	}
}

// WithDisabledPlugins removes the named plugins from the shell.
// Works for both native (builtin) and WASM plugins that are already registered
// at the time the option is applied (i.e. defaultNativePlugins entries).
//
// Precedence note: WASM plugins loaded from the filesystem during discovery
// (which happens after all options are applied) are NOT suppressed by this
// option. To exclude a discovered WASM plugin, omit its name from the
// WithPluginFilter allowlist instead of relying on WithDisabledPlugins.
func WithDisabledPlugins(names ...string) Option {
	return func(s *Shell) {
		for _, name := range names {
			delete(s.builtins, name)
			delete(s.nativePlugins, name)
			delete(s.plugins, name)
		}
	}
}
