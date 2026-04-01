package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/amjadjibon/memsh/shell"
)

// TestRuby is a placeholder for WASM Ruby plugin tests
// TODO: Implement when Ruby WASM plugin is available
func TestRuby(t *testing.T) {
	ctx := context.Background()

	t.Run("ruby -e executes inline code", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		err := s.Run(ctx, `ruby -e 'puts "hello from ruby"'`)
		if err != nil {
			t.Skip("Ruby WASM plugin not yet implemented")
		}
		// TODO: Add assertions when plugin is available
	})
}

// TestRubyFile executes Ruby file from virtual FS
func TestRubyFile(t *testing.T) {
	ctx := context.Background()

	t.Run("ruby executes file from virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/script.rb", []byte(`puts "hello from file"`), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		err := s.Run(ctx, `ruby /script.rb`)
		if err != nil {
			t.Skip("Ruby WASM plugin not yet implemented")
		}
		// TODO: Add assertions when plugin is available
	})
}
