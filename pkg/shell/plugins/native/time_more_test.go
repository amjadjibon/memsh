package native_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/tests"
)

// A literal leading "time" is parsed by mvdan.cc/sh as a TimeClause keyword
// and handled internally, never reaching TimePlugin.Run (see TestTime in
// time_test.go, which exercises that keyword path instead). Reach the
// plugin's own code via "xargs", where "time" is just another argv entry.

func TestTimePluginViaXargsRunsCommandAndReportsElapsed(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := tests.NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
	if err := s.Run(ctx, `echo hi | xargs time echo -n`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "real\t") {
		t.Fatalf("output = %q, want real/user/sys timing report", buf.String())
	}
}
