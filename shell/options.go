package shell

import (
	"io"

	"github.com/spf13/afero"
)

// Option is a configuration function for the Shell.
type Option func(*Shell)

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

// WithEnv sets intial environment variables.
func WithEnv(env map[string]string) Option {
	return func(s *Shell) {
		for k, v := range env {
			s.env[k] = v
		}
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
