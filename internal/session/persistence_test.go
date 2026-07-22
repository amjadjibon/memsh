package session

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/amjadjibon/memsh/pkg/network"
	"github.com/spf13/afero"
)

func TestSessionFilePath(t *testing.T) {
	p1 := sessionFilePath("/tmp/sessions", "abc123")
	p2 := sessionFilePath("/tmp/sessions", "abc123")
	if p1 != p2 {
		t.Errorf("sessionFilePath not deterministic: %q != %q", p1, p2)
	}
	p3 := sessionFilePath("/tmp/sessions", "different")
	if p1 == p3 {
		t.Errorf("sessionFilePath collided for different ids: %q", p1)
	}
	if filepath.Dir(p1) != "/tmp/sessions" {
		t.Errorf("sessionFilePath dir = %q, want /tmp/sessions", filepath.Dir(p1))
	}
	if filepath.Ext(p1) != ".json" {
		t.Errorf("sessionFilePath ext = %q, want .json", filepath.Ext(p1))
	}
}

func TestWriteReadPersistedSession(t *testing.T) {
	dir := t.TempDir()
	rec := persistedSession{
		ID:        "sess1",
		Cwd:       "/home",
		CreatedAt: time.Now().Truncate(time.Second),
		LastUse:   time.Now().Truncate(time.Second),
		RcLoaded:  true,
		RuntimeNS: int64(5 * time.Second),
		Network:   network.Usage{Requests: 3, BytesSent: 100},
	}
	if err := writePersistedSession(dir, rec); err != nil {
		t.Fatalf("writePersistedSession: %v", err)
	}

	got, err := readPersistedSession(dir, "sess1")
	if err != nil {
		t.Fatalf("readPersistedSession: %v", err)
	}
	if got.ID != rec.ID || got.Cwd != rec.Cwd || !got.RcLoaded {
		t.Errorf("round trip mismatch: got %+v, want %+v", got, rec)
	}
	if got.Network.Requests != 3 || got.Network.BytesSent != 100 {
		t.Errorf("network usage mismatch: %+v", got.Network)
	}
}

func TestReadPersistedSessionMissing(t *testing.T) {
	dir := t.TempDir()
	if _, err := readPersistedSession(dir, "nonexistent"); err == nil {
		t.Error("expected error reading nonexistent session")
	}
}

func TestReadPersistedSessionEmptyDir(t *testing.T) {
	if _, err := readPersistedSession("", "id"); err == nil {
		t.Error("expected error for empty dir")
	}
}

func TestWritePersistedSessionEmptyDir(t *testing.T) {
	if err := writePersistedSession("", persistedSession{ID: "x"}); err != nil {
		t.Errorf("writePersistedSession with empty dir should no-op, got: %v", err)
	}
}

func TestRemovePersistedSession(t *testing.T) {
	dir := t.TempDir()
	if err := writePersistedSession(dir, persistedSession{ID: "sess1", Cwd: "/"}); err != nil {
		t.Fatalf("writePersistedSession: %v", err)
	}
	if err := removePersistedSession(dir, "sess1"); err != nil {
		t.Fatalf("removePersistedSession: %v", err)
	}
	if _, err := readPersistedSession(dir, "sess1"); err == nil {
		t.Error("expected error reading removed session")
	}
}

func TestRemovePersistedSessionMissingIsNoop(t *testing.T) {
	dir := t.TempDir()
	if err := removePersistedSession(dir, "nonexistent"); err != nil {
		t.Errorf("removing a nonexistent session should not error, got: %v", err)
	}
}

func TestSaveLoadShellSessionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, "/greeting.txt", []byte("hello persistence"), 0o644); err != nil {
		t.Fatal(err)
	}

	netUsage := network.Usage{Requests: 2, BytesSent: 50, BytesReceived: 200}
	if err := SaveShellSession(dir, "sess1", fs, "/work", 7*time.Second, true, netUsage); err != nil {
		t.Fatalf("SaveShellSession: %v", err)
	}

	restoredFs, cwd, runtime, rcLoaded, gotNet, err := LoadShellSession(dir, "sess1")
	if err != nil {
		t.Fatalf("LoadShellSession: %v", err)
	}
	if cwd != "/work" {
		t.Errorf("cwd = %q, want /work", cwd)
	}
	if runtime != 7*time.Second {
		t.Errorf("runtime = %v, want 7s", runtime)
	}
	if !rcLoaded {
		t.Error("rcLoaded should be true")
	}
	if gotNet != netUsage {
		t.Errorf("network = %+v, want %+v", gotNet, netUsage)
	}
	data, err := afero.ReadFile(restoredFs, "/greeting.txt")
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(data) != "hello persistence" {
		t.Errorf("restored file content = %q", data)
	}
}

func TestSaveShellSessionEmptyDirNoop(t *testing.T) {
	fs := afero.NewMemMapFs()
	if err := SaveShellSession("", "sess1", fs, "/", 0, false, network.Usage{}); err != nil {
		t.Errorf("SaveShellSession with empty dir should no-op, got: %v", err)
	}
}

func TestLoadShellSessionMissing(t *testing.T) {
	dir := t.TempDir()
	if _, _, _, _, _, err := LoadShellSession(dir, "nonexistent"); err == nil {
		t.Error("expected error loading nonexistent session")
	}
}
