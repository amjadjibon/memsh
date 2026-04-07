package tests

import (
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/shell"
)

func makeCompletionFS(t *testing.T) afero.Fs {
	t.Helper()
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/etc", 0755)
	_ = fs.MkdirAll("/home/user", 0755)
	_ = fs.MkdirAll("/usr/bin", 0755)
	_ = afero.WriteFile(fs, "/etc/hosts", []byte("127.0.0.1 localhost\n"), 0644)
	_ = afero.WriteFile(fs, "/etc/passwd", []byte("root:x:0:0\n"), 0644)
	_ = afero.WriteFile(fs, "/home/user/notes.txt", []byte("hello\n"), 0644)
	_ = afero.WriteFile(fs, "/hello.sh", []byte("echo hi\n"), 0755)
	return fs
}

func TestCompleteCommand(t *testing.T) {
	fs := makeCompletionFS(t)
	commands := []string{"cat", "cd", "echo", "env", "grep", "ls", "mkdir", "rm"}

	result := shell.Complete("ec", 2, fs, "/", commands)
	if len(result.Completions) != 1 || result.Completions[0] != "echo" {
		t.Errorf("completions = %v, want [echo]", result.Completions)
	}
	if result.Prefix != "" {
		t.Errorf("prefix = %q, want empty", result.Prefix)
	}
	if result.Token != "ec" {
		t.Errorf("token = %q, want ec", result.Token)
	}
}

func TestCompleteCommandMultiple(t *testing.T) {
	fs := makeCompletionFS(t)
	commands := []string{"cat", "cd", "chmod", "chown"}

	result := shell.Complete("ch", 2, fs, "/", commands)
	if len(result.Completions) != 2 {
		t.Errorf("completions = %v, want [chmod chown]", result.Completions)
	}
}

func TestCompleteCommandEmpty(t *testing.T) {
	fs := makeCompletionFS(t)
	commands := []string{"cat", "ls", "echo"}

	result := shell.Complete("", 0, fs, "/", commands)
	if len(result.Completions) != 3 {
		t.Errorf("expected all 3 commands for empty input, got %v", result.Completions)
	}
}

func TestCompleteAbsolutePath(t *testing.T) {
	fs := makeCompletionFS(t)
	commands := []string{"ls"}

	// ls /et → should complete to /etc/
	result := shell.Complete("ls /et", 6, fs, "/", commands)
	if len(result.Completions) != 1 || result.Completions[0] != "/etc/" {
		t.Errorf("completions = %v, want [/etc/]", result.Completions)
	}
	if result.Prefix != "ls " {
		t.Errorf("prefix = %q, want 'ls '", result.Prefix)
	}
}

func TestCompleteInsideDirectory(t *testing.T) {
	fs := makeCompletionFS(t)
	commands := []string{"cat"}

	// cat /etc/ → list /etc contents
	result := shell.Complete("cat /etc/", 9, fs, "/", commands)
	names := result.Completions
	if len(names) != 2 {
		t.Errorf("completions = %v, want [/etc/hosts /etc/passwd]", names)
	}
	if names[0] != "/etc/hosts" || names[1] != "/etc/passwd" {
		t.Errorf("completions = %v", names)
	}
}

func TestCompleteRelativePath(t *testing.T) {
	fs := makeCompletionFS(t)
	commands := []string{"cat"}

	// cwd = /etc, completing "pass"
	result := shell.Complete("cat pass", 8, fs, "/etc", commands)
	if len(result.Completions) != 1 || result.Completions[0] != "passwd" {
		t.Errorf("completions = %v, want [passwd]", result.Completions)
	}
}

func TestCompleteNoMatch(t *testing.T) {
	fs := makeCompletionFS(t)
	commands := []string{"cat", "ls"}

	result := shell.Complete("xyz", 3, fs, "/", commands)
	if len(result.Completions) != 0 {
		t.Errorf("expected no completions, got %v", result.Completions)
	}
}

func TestCompleteAfterPipe(t *testing.T) {
	fs := makeCompletionFS(t)
	commands := []string{"cat", "grep", "sort"}

	// "ls / | gr" → completing "gr" which is a command after the pipe
	result := shell.Complete("ls / | gr", 9, fs, "/", commands)
	if len(result.Completions) != 1 || result.Completions[0] != "grep" {
		t.Errorf("completions = %v, want [grep]", result.Completions)
	}
}

func TestCompleteAfterSemicolon(t *testing.T) {
	fs := makeCompletionFS(t)
	commands := []string{"cat", "ls", "echo"}

	result := shell.Complete("ls /; ec", 8, fs, "/", commands)
	if len(result.Completions) != 1 || result.Completions[0] != "echo" {
		t.Errorf("completions = %v, want [echo]", result.Completions)
	}
}

func TestDefaultCommands(t *testing.T) {
	cmds := shell.DefaultCommands()
	if len(cmds) == 0 {
		t.Fatal("DefaultCommands returned empty list")
	}
	// Spot-check known builtins
	cmdSet := make(map[string]bool, len(cmds))
	for _, c := range cmds {
		cmdSet[c] = true
	}
	for _, expected := range []string{"cat", "ls", "echo", "grep", "awk", "jq"} {
		if !cmdSet[expected] {
			t.Errorf("DefaultCommands missing %q", expected)
		}
	}
	// Verify sorted
	for i := 1; i < len(cmds); i++ {
		if cmds[i] < cmds[i-1] {
			t.Errorf("DefaultCommands not sorted: %q before %q", cmds[i-1], cmds[i])
		}
	}
}

func TestCompleteRootSlash(t *testing.T) {
	fs := makeCompletionFS(t)
	commands := []string{"ls"}

	// ls / → list root
	result := shell.Complete("ls /", 4, fs, "/", commands)
	names := result.Completions
	if len(names) == 0 {
		t.Error("expected completions for '/'")
	}
	for _, n := range names {
		if n[0] != '/' {
			t.Errorf("completion %q should start with /", n)
		}
	}
}
