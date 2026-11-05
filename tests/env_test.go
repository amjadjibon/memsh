package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

// TestEnvSeesExportedVariables checks that env/printenv read the environment
// the command runs with, not only the shell's starting environment.
func TestEnvSeesExportedVariables(t *testing.T) {
	cases := []struct {
		name, script, want string
	}{
		{"printenv exported", `export FOO=bar; printenv FOO`, "bar"},
		{"env exported", `export FOO=bar; env | grep '^FOO='`, "FOO=bar"},
		{"prefix assignment", `X=9 printenv X`, "9"},
		{"unexported hidden", `BAZ=1; printenv BAZ; echo rc=$?`, "rc=1"},
		{"printenv several", `export A=1 B=2; printenv A B`, "1\n2"},
		{"export in subshell", `(export S=sub; printenv S)`, "sub"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
			_ = s.Run(context.Background(), tc.script)
			if got := strings.TrimSpace(buf.String()); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
