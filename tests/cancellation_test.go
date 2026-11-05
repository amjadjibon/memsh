package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

// TestLongRunningPluginsHonourContext checks that the per-request timeout
// actually stops interpreter-backed plugins instead of leaving them spinning
// after the handler has returned.
func TestLongRunningPluginsHonourContext(t *testing.T) {
	scripts := map[string]string{
		"goja": `goja -e 'while(true){}'`,
		"awk":  `awk 'BEGIN { while (1) {} }'`,
		"jq":   `jq -n '[range(1;infinite)] | length'`,
		"yq":   `echo 'a: 1' | yq '[range(1;infinite)] | length'`,
	}
	for name, script := range scripts {
		t.Run(name, func(t *testing.T) {
			var buf strings.Builder
			s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			done := make(chan error, 1)
			go func() { done <- s.Run(ctx, script) }()
			select {
			case err := <-done:
				if err == nil {
					t.Fatalf("%s returned nil after cancellation; output %q", name, buf.String())
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("%s did not stop after context cancellation", name)
			}
		})
	}
}
