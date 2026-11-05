package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

// gatedCommands are withheld from a default shell because each embeds an
// interpreter that can reach the host OS.
var gatedCommands = []string{"lua", "go", "sqlite3"}

func TestGatedInterpretersOffByDefault(t *testing.T) {
	for _, cmd := range gatedCommands {
		t.Run(cmd+" is not registered by default", func(t *testing.T) {
			var buf strings.Builder
			s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
			// Non-zero exit is expected; the point is the command must not run.
			err := s.Run(context.Background(), cmd+" --version")

			got := buf.String()
			if err != nil {
				got += err.Error()
			}
			if !strings.Contains(got, "disabled by default") {
				t.Errorf("%s: expected a 'disabled by default' message, got %q", cmd, got)
			}
		})
	}

	t.Run("DefaultCommands omits gated interpreters", func(t *testing.T) {
		defaults := make(map[string]bool)
		for _, n := range shell.DefaultCommands() {
			defaults[n] = true
		}
		for _, cmd := range gatedCommands {
			if defaults[cmd] {
				t.Errorf("DefaultCommands includes gated interpreter %q", cmd)
			}
		}
	})

	t.Run("GatedInterpreterNames matches", func(t *testing.T) {
		got := strings.Join(shell.GatedInterpreterNames(), ",")
		if got != "go,lua,sqlite3" {
			t.Errorf("GatedInterpreterNames() = %q, want %q", got, "go,lua,sqlite3")
		}
	})
}

func TestGatedInterpretersEnabledByOption(t *testing.T) {
	for _, cmd := range gatedCommands {
		t.Run(cmd+" runs with WithUnsafeInterpreters", func(t *testing.T) {
			var buf strings.Builder
			s := NewTestShell(t, &buf,
				shell.WithFS(afero.NewMemMapFs()),
				shell.WithUnsafeInterpreters(true),
			)
			err := s.Run(context.Background(), cmd+" --version")

			got := buf.String()
			if err != nil {
				got += err.Error()
			}
			if strings.Contains(got, "disabled by default") {
				t.Errorf("%s: still disabled after WithUnsafeInterpreters(true): %q", cmd, got)
			}
		})
	}

	t.Run("WithDisabledPlugins still wins when applied after", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf,
			shell.WithFS(afero.NewMemMapFs()),
			shell.WithUnsafeInterpreters(true),
			shell.WithDisabledPlugins("lua"),
		)
		err := s.Run(context.Background(), "lua -e 'print(1)'")

		got := buf.String()
		if err != nil {
			got += err.Error()
		}
		if !strings.Contains(got, "lua:") {
			t.Errorf("expected lua to be unavailable, got %q", got)
		}
	})
}

// TestDefaultShellCannotReachHostOS is the regression guard for the escapes
// that motivated gating: by default no command may execute a real OS process
// or write outside the virtual filesystem.
func TestDefaultShellCannotReachHostOS(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "escaped.txt")

	scripts := map[string]string{
		"lua os.execute": `lua -e "os.execute('touch ` + marker + `')"`,
		"lua io.open":    `lua -e "local f=io.open('` + marker + `','w') if f then f:write('x') f:close() end"`,
		"go os/exec":     "printf '%s' \"package main\\nimport \\\"os/exec\\\"\\nfunc main(){ exec.Command(\\\"touch\\\",\\\"" + marker + "\\\").Run() }\" > /p.go\ngo run /p.go",
		"sqlite3 ATTACH": `sqlite3 /v.db "ATTACH DATABASE '` + marker + `' AS ext; CREATE TABLE ext.t(x);"`,
		"sqlite3 VACUUM": `sqlite3 /v.db "CREATE TABLE t(x); VACUUM INTO '` + marker + `';"`,
	}

	for name, script := range scripts {
		t.Run(name, func(t *testing.T) {
			var buf strings.Builder
			s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
			_ = s.Run(context.Background(), script)

			if _, err := os.Stat(marker); err == nil {
				t.Fatalf("SANDBOX ESCAPE: %s created %s on the real filesystem (output: %q)",
					name, marker, buf.String())
			}
		})
	}
}

// TestAwkCannotReachHostOS pins goawk's host-facing features off: system(),
// command pipes, file redirection and getline from files all bypass the
// virtual FS, and ENVIRON would otherwise come from os.Environ().
func TestAwkCannotReachHostOS(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "escaped.txt")
	t.Setenv("MEMSH_HOST_ONLY_SECRET", "leaked")

	scripts := map[string]string{
		"print redirect": `awk 'BEGIN { print "x" > "` + marker + `" }'`,
		"print append":   `awk 'BEGIN { print "x" >> "` + marker + `" }'`,
		"system":         `awk 'BEGIN { system("touch ` + marker + `") }'`,
		"pipe to cmd":    `awk 'BEGIN { print "x" | "tee ` + marker + `" }'`,
		"cmd getline":    `awk 'BEGIN { "touch ` + marker + `" | getline }'`,
	}
	for name, script := range scripts {
		t.Run(name, func(t *testing.T) {
			var buf strings.Builder
			s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
			_ = s.Run(context.Background(), script)
			if _, err := os.Stat(marker); err == nil {
				t.Fatalf("SANDBOX ESCAPE: awk %s created %s (output %q)", name, marker, buf.String())
			}
		})
	}

	t.Run("getline from host file", func(t *testing.T) {
		host := filepath.Join(t.TempDir(), "host.txt")
		if err := os.WriteFile(host, []byte("host-secret\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		_ = s.Run(context.Background(), `awk 'BEGIN { getline line < "`+host+`"; print line }'`)
		if strings.Contains(buf.String(), "host-secret") {
			t.Fatalf("awk read a host file: %q", buf.String())
		}
	})

	t.Run("ENVIRON is the shell env", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()), shell.WithInheritEnv(false))
		_ = s.Run(context.Background(), `awk 'BEGIN { print ENVIRON["MEMSH_HOST_ONLY_SECRET"] }'`)
		if strings.Contains(buf.String(), "leaked") {
			t.Fatalf("awk ENVIRON exposed host env: %q", buf.String())
		}
		buf.Reset()
		_ = s.Run(context.Background(), `export GREETING=hi; awk 'BEGIN { print ENVIRON["GREETING"] }'`)
		if strings.TrimSpace(buf.String()) != "hi" {
			t.Fatalf("awk ENVIRON missing shell export: %q", buf.String())
		}
	})
}
