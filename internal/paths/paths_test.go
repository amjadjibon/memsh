package paths_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amjadjibon/memsh/internal/paths"
)

func TestPathHelpersUseHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name string
		fn   func() (string, error)
		want string
	}{
		{
			name: "config file",
			fn:   paths.ConfigFile,
			want: filepath.Join(home, ".memsh", "config.toml"),
		},
		{
			name: "memshrc file",
			fn:   paths.MemshrcFile,
			want: filepath.Join(home, ".memshrc"),
		},
		{
			name: "ssh host key file",
			fn:   paths.SSHHostKeyFile,
			want: filepath.Join(home, ".memsh", "ssh_host_key"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn()
			if err != nil {
				t.Fatalf("path helper returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDirectoryHelpersCreateDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name string
		fn   func() (string, error)
		want string
	}{
		{
			name: "memsh dir",
			fn:   paths.MemshDir,
			want: filepath.Join(home, ".memsh"),
		},
		{
			name: "history dir",
			fn:   paths.HistoryDir,
			want: filepath.Join(home, ".memsh", "history"),
		},
		{
			name: "plugin dir",
			fn:   paths.PluginDir,
			want: filepath.Join(home, ".memsh", "plugins"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn()
			if err != nil {
				t.Fatalf("directory helper returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("dir = %q, want %q", got, tt.want)
			}
			info, err := os.Stat(got)
			if err != nil {
				t.Fatalf("created directory stat failed: %v", err)
			}
			if !info.IsDir() {
				t.Fatalf("%q is not a directory", got)
			}
		})
	}
}

func TestFileHelpersDoNotCreateMemshDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, fn := range []func() (string, error){paths.ConfigFile, paths.SSHHostKeyFile} {
		if _, err := fn(); err != nil {
			t.Fatalf("file helper returned error: %v", err)
		}
		if _, err := os.Stat(filepath.Join(home, ".memsh")); !os.IsNotExist(err) {
			t.Fatalf("file helper created .memsh directory, stat err = %v", err)
		}
	}
}
