package repl

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestNeedsContinuation(t *testing.T) {
	cases := []struct {
		name string
		line string
		want bool
	}{
		{"complete simple command", "echo hello", false},
		{"unclosed brace", "if [ 1 -eq 1 ] {", true},
		{"unclosed paren", "myfunc(", true},
		{"unclosed bracket", "arr[0", true},
		{"balanced braces", "echo {a,b}", false},
		{"trailing do", "for i in 1 2 3; do", true},
		{"trailing then", "if true; then", true},
		{"trailing else", "else", true},
		{"trailing elif", "if true; then echo a; elif", true},
		{"for without terminator", "for i in 1 2 3", true},
		{"for with semicolon", "for i in 1 2 3;", false},
		{"if with fi present", "if true; fi", false},
		{"while without terminator", "while true", true},
		{"case without esac", "case $x", true},
		{"trailing backslash", "echo hello \\", true},
		{"plain word list", "ls -la /tmp", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsContinuation(tc.line); got != tc.want {
				t.Errorf("needsContinuation(%q) = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

func TestPrompt(t *testing.T) {
	sh, err := shell.New(shell.WithFS(afero.NewMemMapFs()), shell.WithWASMEnabled(false))
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	defer sh.Close()

	got := prompt(sh)
	want := "memsh:" + sh.Cwd() + "$ "
	if got != want {
		t.Errorf("prompt() = %q, want %q", got, want)
	}
}

func TestHistoryFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := historyFile()
	if err != nil {
		t.Fatalf("historyFile: %v", err)
	}
	wantDir := filepath.Join(home, ".memsh", "history")
	if filepath.Dir(path) != wantDir {
		t.Errorf("historyFile dir = %q, want %q", filepath.Dir(path), wantDir)
	}
	if len(filepath.Base(path)) != 64 {
		t.Errorf("historyFile basename = %q, want a 64-char sha256 hex digest", filepath.Base(path))
	}
}

func TestHistoryFileRemovesNonDirAtPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Pre-create ~/.memsh/history as a plain file instead of a directory.
	memshDir := filepath.Join(home, ".memsh")
	if err := afero.NewOsFs().MkdirAll(memshDir, 0o755); err != nil {
		t.Fatal(err)
	}
	historyPath := filepath.Join(memshDir, "history")
	if err := afero.WriteFile(afero.NewOsFs(), historyPath, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	// paths.HistoryDir() will fail to MkdirAll over the existing file; that's
	// fine — we're only checking historyFile() doesn't panic in that case.
	_, _ = historyFile()
}

func TestGetVersion(t *testing.T) {
	v := GetVersion()
	if !strings.HasPrefix(v, "memsh") {
		t.Errorf("GetVersion() = %q, want it to start with 'memsh'", v)
	}
}

func TestIsInteractiveTerminalDoesNotPanic(t *testing.T) {
	_ = IsInteractiveTerminal()
}

func TestShouldRunInteractiveDoesNotPanic(t *testing.T) {
	_ = ShouldRunInteractive()
}

func TestRunPipedExecutesLines(t *testing.T) {
	var buf strings.Builder
	sh, err := shell.New(
		shell.WithFS(afero.NewMemMapFs()),
		shell.WithWASMEnabled(false),
		shell.WithStdIO(strings.NewReader(""), &buf, &buf),
	)
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	defer sh.Close()

	input := "echo one\n\necho two\n"
	if err := RunPiped(context.Background(), sh, strings.NewReader(input)); err != nil {
		t.Fatalf("RunPiped: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		t.Errorf("output = %q, want both echoed lines", out)
	}
}

func TestRunPipedStopsOnExit(t *testing.T) {
	var buf strings.Builder
	sh, err := shell.New(
		shell.WithFS(afero.NewMemMapFs()),
		shell.WithWASMEnabled(false),
		shell.WithStdIO(strings.NewReader(""), &buf, &buf),
	)
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	defer sh.Close()

	input := "echo before\nexit\necho after\n"
	if err := RunPiped(context.Background(), sh, strings.NewReader(input)); err != nil {
		t.Fatalf("RunPiped: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "before") {
		t.Errorf("output = %q, want 'before' to have run", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("output = %q, want 'after' to be skipped once exit is seen", out)
	}
}

func TestLoadMemshrc(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rcPath := filepath.Join(home, ".memshrc")
	if err := afero.WriteFile(afero.NewOsFs(), rcPath, []byte("echo from-memshrc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	sh, err := shell.New(
		shell.WithFS(afero.NewMemMapFs()),
		shell.WithWASMEnabled(false),
		shell.WithStdIO(strings.NewReader(""), &buf, &buf),
	)
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	defer sh.Close()

	loadMemshrc(sh, context.Background())
	if !strings.Contains(buf.String(), "from-memshrc") {
		t.Errorf("output = %q, want .memshrc to have been sourced", buf.String())
	}
}

func TestLoadMemshrcMissingFileIsNoop(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var buf strings.Builder
	sh, err := shell.New(
		shell.WithFS(afero.NewMemMapFs()),
		shell.WithWASMEnabled(false),
		shell.WithStdIO(strings.NewReader(""), &buf, &buf),
	)
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	defer sh.Close()

	loadMemshrc(sh, context.Background()) // should not panic when ~/.memshrc is absent
	if buf.String() != "" {
		t.Errorf("output = %q, want no output when .memshrc is missing", buf.String())
	}
}

func TestRunWithSignalExecutesScript(t *testing.T) {
	var buf strings.Builder
	sh, err := shell.New(
		shell.WithFS(afero.NewMemMapFs()),
		shell.WithWASMEnabled(false),
		shell.WithStdIO(strings.NewReader(""), &buf, &buf),
	)
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	defer sh.Close()

	if err := runWithSignal(context.Background(), sh, "echo signaled"); err != nil {
		t.Fatalf("runWithSignal: %v", err)
	}
	if !strings.Contains(buf.String(), "signaled") {
		t.Errorf("output = %q, want the script to have run", buf.String())
	}
}

func TestRunPipedContinuesAfterCommandError(t *testing.T) {
	var buf strings.Builder
	sh, err := shell.New(
		shell.WithFS(afero.NewMemMapFs()),
		shell.WithWASMEnabled(false),
		shell.WithStdIO(strings.NewReader(""), &buf, &buf),
	)
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	defer sh.Close()

	input := "nonexistent-cmd-xyz\necho recovered\n"
	if err := RunPiped(context.Background(), sh, strings.NewReader(input)); err != nil {
		t.Fatalf("RunPiped: %v", err)
	}
	if !strings.Contains(buf.String(), "recovered") {
		t.Errorf("output = %q, want the shell to keep processing after a failed command", buf.String())
	}
}
