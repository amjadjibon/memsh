package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestHeadTailLegacyCount(t *testing.T) {
	cases := map[string]string{
		"head -2 /f":       "1\n2",
		"tail -1 /f":       "4",
		"cat /f | head -3": "1\n2\n3",
		"head -n1 /f":      "1",
	}
	for script, want := range cases {
		t.Run(script, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			_ = afero.WriteFile(fs, "/f", []byte("1\n2\n3\n4\n"), 0o644)
			var buf strings.Builder
			s := NewTestShell(t, &buf, shell.WithFS(fs))
			if err := s.Run(context.Background(), script); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := strings.TrimSpace(buf.String()); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}
