package shell

import (
	"testing"

	"github.com/spf13/afero"
)

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
		want  string
	}{
		{"/", true, "/"},
		{"/foo/bar", true, "/foo/bar"},
		{"/foo/../bar", true, "/bar"},
		{"/foo/./bar", true, "/foo/bar"},
		{"relative", false, ""},
		{"../etc/passwd", false, ""},
		{"..", false, ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, ok := sanitizePath(tc.input)
			if ok != tc.ok {
				t.Errorf("sanitizePath(%q) ok = %v, want %v", tc.input, ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("sanitizePath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCompleteCommandPosition(t *testing.T) {
	commands := []string{"echo", "ls", "cat", "grep", "mkdir"}
	fs := afero.NewMemMapFs()

	result := Complete("ec", 2, fs, "/", commands)
	if result.Token != "ec" {
		t.Errorf("Token = %q, want %q", result.Token, "ec")
	}
	if len(result.Completions) != 1 || result.Completions[0] != "echo" {
		t.Errorf("Completions = %v, want [echo]", result.Completions)
	}
}

func TestCompleteCommandAfterPipe(t *testing.T) {
	commands := []string{"echo", "ls", "grep"}
	fs := afero.NewMemMapFs()

	result := Complete("echo hello | gr", 15, fs, "/", commands)
	if result.Token != "gr" {
		t.Errorf("Token = %q, want %q", result.Token, "gr")
	}
	if len(result.Completions) != 1 || result.Completions[0] != "grep" {
		t.Errorf("Completions = %v, want [grep]", result.Completions)
	}
}

func TestCompletePathAbsolute(t *testing.T) {
	fs := afero.NewMemMapFs()
	afero.WriteFile(fs, "/etc/config", []byte("x"), 0644)
	fs.Mkdir("/etc/init.d", 0755)

	result := Complete("cat /etc/", 10, fs, "/", nil)
	found := make(map[string]bool)
	for _, c := range result.Completions {
		found[c] = true
	}
	if !found["/etc/config"] {
		t.Error("expected '/etc/config' in completions")
	}
	if !found["/etc/init.d/"] {
		t.Error("expected '/etc/init.d/' in completions (directory with trailing slash)")
	}
}

func TestCompletePathRelative(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Mkdir("/home/user/src", 0755)
	afero.WriteFile(fs, "/home/user/src/main.go", []byte("x"), 0644)
	afero.WriteFile(fs, "/home/user/src/util.go", []byte("x"), 0644)

	result := Complete("cat m", 5, fs, "/home/user/src", nil)
	if result.Token != "m" {
		t.Errorf("Token = %q, want %q", result.Token, "m")
	}
	if len(result.Completions) != 1 || result.Completions[0] != "main.go" {
		t.Errorf("Completions = %v, want [main.go]", result.Completions)
	}
}

func TestCompletePathPartial(t *testing.T) {
	fs := afero.NewMemMapFs()
	afero.WriteFile(fs, "/foo1", []byte("x"), 0644)
	afero.WriteFile(fs, "/foo2", []byte("x"), 0644)
	afero.WriteFile(fs, "/bar", []byte("x"), 0644)

	result := Complete("cat /foo", 8, fs, "/", nil)
	if len(result.Completions) != 2 {
		t.Errorf("Completions = %v, want 2 results", result.Completions)
	}
}

func TestCompleteEmpty(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Mkdir("/dir1", 0755)
	fs.Mkdir("/dir2", 0755)

	result := Complete("ls ", 3, fs, "/", nil)
	if result.Token != "" {
		t.Errorf("Token = %q, want empty", result.Token)
	}
	if len(result.Completions) != 2 {
		t.Errorf("Completions = %v, want 2 results", result.Completions)
	}
}

func TestCompleteCursorOutOfRange(t *testing.T) {
	result := Complete("echo", -1, nil, "/", []string{"echo"})
	if result.Token != "echo" {
		t.Errorf("Token = %q, want %q", result.Token, "echo")
	}

	result = Complete("echo", 100, nil, "/", []string{"echo"})
	if result.Token != "echo" {
		t.Errorf("Token = %q, want %q", result.Token, "echo")
	}
}

func TestAbsJoin(t *testing.T) {
	tests := []struct {
		path, cwd, want string
	}{
		{"", "/home", "/home"},
		{".", "/home", "/home"},
		{"./", "/home", "/home"},
		{"/abs", "/home", "/abs"},
		{"rel", "/home", "/home/rel"},
		{"sub/dir", "/home", "/home/sub/dir"},
	}
	for _, tc := range tests {
		got := absJoin(tc.path, tc.cwd)
		if got != tc.want {
			t.Errorf("absJoin(%q, %q) = %q, want %q", tc.path, tc.cwd, got, tc.want)
		}
	}
}
