package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestSnapshotRoundTrip(t *testing.T) {
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/hello.txt", []byte("hello world\n"), 0644)
	_ = fs.MkdirAll("/subdir", 0755)
	_ = afero.WriteFile(fs, "/subdir/data.json", []byte(`{"key":"value"}`), 0600)

	snap, err := shell.TakeSnapshot(fs, "/subdir")
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}

	if snap.Version != 1 {
		t.Errorf("version = %d, want 1", snap.Version)
	}
	if snap.Cwd != "/subdir" {
		t.Errorf("cwd = %q, want /subdir", snap.Cwd)
	}
	if len(snap.Files) < 3 {
		t.Errorf("files count = %d, want >= 3", len(snap.Files))
	}

	// Marshal + Unmarshal round-trip
	data, err := shell.MarshalSnapshot(snap)
	if err != nil {
		t.Fatalf("MarshalSnapshot: %v", err)
	}
	snap2, err := shell.UnmarshalSnapshot(data)
	if err != nil {
		t.Fatalf("UnmarshalSnapshot: %v", err)
	}

	if snap2.Cwd != snap.Cwd {
		t.Errorf("round-trip cwd = %q, want %q", snap2.Cwd, snap.Cwd)
	}
	if len(snap2.Files) != len(snap.Files) {
		t.Errorf("round-trip files = %d, want %d", len(snap2.Files), len(snap.Files))
	}
}

func TestSnapshotRestoreFS(t *testing.T) {
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/greeting.txt", []byte("hi there\n"), 0644)
	_ = fs.MkdirAll("/configs", 0755)
	_ = afero.WriteFile(fs, "/configs/app.cfg", []byte("port=8080\n"), 0600)

	snap, err := shell.TakeSnapshot(fs, "/")
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}

	fs2, cwd, err := shell.RestoreSnapshot(snap)
	if err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}
	if cwd != "/" {
		t.Errorf("restored cwd = %q, want /", cwd)
	}

	data, err := afero.ReadFile(fs2, "/greeting.txt")
	if err != nil {
		t.Fatalf("read greeting.txt: %v", err)
	}
	if string(data) != "hi there\n" {
		t.Errorf("greeting.txt = %q, want %q", string(data), "hi there\n")
	}

	data2, err := afero.ReadFile(fs2, "/configs/app.cfg")
	if err != nil {
		t.Fatalf("read configs/app.cfg: %v", err)
	}
	if string(data2) != "port=8080\n" {
		t.Errorf("app.cfg = %q, want %q", string(data2), "port=8080\n")
	}

	// Verify mode
	info, err := fs2.Stat("/configs/app.cfg")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode() != 0600 {
		t.Errorf("mode = %o, want 0600", info.Mode())
	}
}

func TestSnapshotEmptyFS(t *testing.T) {
	fs := afero.NewMemMapFs()
	snap, err := shell.TakeSnapshot(fs, "/")
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}
	if len(snap.Files) != 0 {
		t.Errorf("expected no files, got %d", len(snap.Files))
	}

	data, err := shell.MarshalSnapshot(snap)
	if err != nil {
		t.Fatalf("MarshalSnapshot: %v", err)
	}
	snap2, err := shell.UnmarshalSnapshot(data)
	if err != nil {
		t.Fatalf("UnmarshalSnapshot: %v", err)
	}
	if len(snap2.Files) != 0 {
		t.Errorf("expected no files after restore, got %d", len(snap2.Files))
	}
}

func TestSnapshotFromScript(t *testing.T) {
	var buf strings.Builder
	s := NewTestShell(t, &buf)

	ctx := context.Background()
	err := s.Run(ctx, `mkdir -p /data && echo "snapshot test" > /data/result.txt`)
	if err != nil {
		t.Fatalf("run script: %v", err)
	}

	snap, err := shell.TakeSnapshot(s.FS(), s.Cwd())
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}

	found := false
	for _, f := range snap.Files {
		if f.Path == "/data/result.txt" {
			found = true
			if string(f.Content) != "snapshot test\n" {
				t.Errorf("content = %q, want %q", string(f.Content), "snapshot test\n")
			}
		}
	}
	if !found {
		t.Error("/data/result.txt not found in snapshot")
	}

	// Restore and verify
	fs2, _, err := shell.RestoreSnapshot(snap)
	if err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}

	data, err := afero.ReadFile(fs2, "/data/result.txt")
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(data) != "snapshot test\n" {
		t.Errorf("restored content = %q", string(data))
	}
}

func TestSnapshotUnsupportedVersion(t *testing.T) {
	data := []byte(`{"version":99,"cwd":"/","files":[]}`)
	snap, err := shell.UnmarshalSnapshot(data)
	if err != nil {
		t.Fatalf("UnmarshalSnapshot: %v", err)
	}
	_, _, err = shell.RestoreSnapshot(snap)
	if err == nil {
		t.Error("expected error for unsupported version, got nil")
	}
}

func TestSnapshotDirectoryStructure(t *testing.T) {
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/a/b/c", 0755)
	_ = afero.WriteFile(fs, "/a/b/c/deep.txt", []byte("deep"), 0644)

	snap, err := shell.TakeSnapshot(fs, "/")
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}

	fs2, _, err := shell.RestoreSnapshot(snap)
	if err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}

	info, err := fs2.Stat("/a/b/c")
	if err != nil {
		t.Fatalf("stat /a/b/c: %v", err)
	}
	if !info.IsDir() {
		t.Error("/a/b/c should be a directory")
	}

	data, err := afero.ReadFile(fs2, "/a/b/c/deep.txt")
	if err != nil {
		t.Fatalf("read deep.txt: %v", err)
	}
	if string(data) != "deep" {
		t.Errorf("deep.txt = %q, want %q", string(data), "deep")
	}
}
