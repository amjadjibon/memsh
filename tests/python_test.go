package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/amjadjibon/memsh/shell"
)

// TestPython is a placeholder for WASM Python plugin tests
// TODO: Implement when Python WASM plugin is available
func TestPython(t *testing.T) {
	ctx := context.Background()

	t.Run("python -c executes inline code", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		err := s.Run(ctx, `python -c 'print("hello from python")'`)
		if err != nil {
			t.Skip("Python WASM plugin not yet implemented")
		}
		// TODO: Add assertions when plugin is available
	})
}

// TestPythonFile executes Python file from virtual FS
func TestPythonFile(t *testing.T) {
	ctx := context.Background()

	t.Run("python executes file from virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/script.py", []byte(`print("hello from file")`), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		err := s.Run(ctx, `python /script.py`)
		if err != nil {
			t.Skip("Python WASM plugin not yet implemented")
		}
		// TODO: Add assertions when plugin is available
	})
}
