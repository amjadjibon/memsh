package shell

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Shell represents the virtual bash session.
type Shell struct {
	fs     afero.Fs
	cwd    string
	env    map[string]string

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	runner  *interp.Runner
	plugins pluginRegistry
	fds     map[uint32]afero.File
}

// New creates a new Shell instance with the provided options.
func New(opts ...Option) (*Shell, error) {
	s := &Shell{
		fs:      afero.NewMemMapFs(),
		cwd:     "/",
		env:     make(map[string]string),
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
		plugins: make(pluginRegistry),
		fds:     make(map[uint32]afero.File),
	}

	for _, opt := range opts {
		opt(s)
	}

	// Initialize the runner
	runnerOpts := []interp.RunnerOption{
		interp.StdIO(s.stdin, s.stdout, s.stderr),
		// We'll set up the custom OpenHandler and ExecHandler next
		interp.OpenHandler(s.openHandler),
		interp.ExecHandlers(s.execHandler),
        // Use a custom directory tracker
        interp.Dir(s.cwd),
	}

    // Set up the environment tracking
    // For mvdan.cc/sh we can provide an Env implementation if needed,
    // or we can use the default and sync it.
    // interp.Env(interp.FuncEnviron(...)) 

	runner, err := interp.New(runnerOpts...)
	if err != nil {
		return nil, err
	}
	s.runner = runner

	// Register built-in plugins (don't overwrite user-supplied ones).
	for name, wasm := range defaultPlugins {
		if _, exists := s.plugins[name]; !exists {
			s.plugins[name] = wasm
		}
	}

	if err := s.loadPlugins(); err != nil {
		return nil, err
	}

	return s, nil
}

// Cwd returns the current working directory of the shell.
func (s *Shell) Cwd() string {
	return s.cwd
}

// Run executes a unified shell script string.
func (s *Shell) Run(ctx context.Context, script string) error {
	file, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	if err != nil {
		return err
	}
	return s.runner.Run(ctx, file)
}
