// Package plugins defines the Plugin interface and shell context helpers shared
// by the shell runtime and all native plugin implementations.
package plugins

import (
	"context"

	"github.com/spf13/afero"
)

// Plugin is the interface for registering native Go commands with memsh.
//
// Example — a minimal "hello" command:
//
//	type HelloPlugin struct{}
//
//	func (HelloPlugin) Name() string { return "hello" }
//	func (HelloPlugin) Run(ctx context.Context, args []string) error {
//	    hc := interp.HandlerCtx(ctx)          // per-invocation I/O (works in pipes)
//	    sc := plugins.ShellCtx(ctx)            // virtual FS + cwd
//	    fmt.Fprintln(hc.Stdout, "Hello!")
//	    return nil
//	}
//
//	sh, _ := shell.New(shell.WithPlugin(HelloPlugin{}))
type Plugin interface {
	// Name returns the command name used to invoke the plugin (e.g. "jq").
	Name() string
	// Run executes the command. Use interp.HandlerCtx(ctx) for per-invocation
	// stdin/stdout/stderr, and plugins.ShellCtx(ctx) for virtual FS access.
	Run(ctx context.Context, args []string) error
}

// PluginInfo optionally extends Plugin with metadata shown by `plugin list`.
type PluginInfo interface {
	Plugin
	// Description returns a one-line summary shown in `plugin list`.
	Description() string
	// Usage returns a usage string, e.g. "base64 [-d] [data...]".
	Usage() string
}

// ShellContext provides access to shell-level state inside a Plugin.Run call.
// Retrieve it with ShellCtx(ctx).
type ShellContext struct {
	// FS is the virtual in-memory filesystem (afero.MemMapFs by default).
	FS afero.Fs
	// Cwd is the shell's current working directory.
	Cwd string
	// Env looks up a shell environment variable by key.
	Env func(key string) string
	// ResolvePath converts a possibly-relative path to an absolute virtual path.
	ResolvePath func(path string) string
}

type ctxKey struct{}

// ShellCtx extracts the ShellContext injected by the shell's exec handler.
// Returns a zero ShellContext if called outside a shell invocation.
func ShellCtx(ctx context.Context) ShellContext {
	v, _ := ctx.Value(ctxKey{}).(ShellContext)
	return v
}

// WithShellContext returns a new context carrying sc.
// This is called by the shell's exec handler and is not typically needed
// by plugin authors.
func WithShellContext(ctx context.Context, sc ShellContext) context.Context {
	return context.WithValue(ctx, ctxKey{}, sc)
}
