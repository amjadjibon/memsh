package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHistoryDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := historyDir()
	if err != nil {
		t.Fatalf("historyDir: %v", err)
	}
	want := filepath.Join(home, ".memsh", "history")
	if dir != want {
		t.Errorf("historyDir = %q, want %q", dir, want)
	}
}

func TestCountLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session1")
	content := "echo one\n\n  \nls\npwd\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := countLines(path)
	if err != nil {
		t.Fatalf("countLines: %v", err)
	}
	if n != 3 {
		t.Errorf("countLines = %d, want 3 (blank lines excluded)", n)
	}
}

func TestCountLinesMissingFile(t *testing.T) {
	if _, err := countLines(filepath.Join(t.TempDir(), "nonexistent")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCountLinesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := countLines(path)
	if err != nil {
		t.Fatalf("countLines: %v", err)
	}
	if n != 0 {
		t.Errorf("countLines = %d, want 0", n)
	}
}

func TestFindByPrefixUniqueMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "abc123"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "def456"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	match, err := findByPrefix(dir, "abc")
	if err != nil {
		t.Fatalf("findByPrefix: %v", err)
	}
	if match != filepath.Join(dir, "abc123") {
		t.Errorf("findByPrefix = %q, want abc123", match)
	}
}

func TestFindByPrefixAmbiguous(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "abc111"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "abc222"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := findByPrefix(dir, "abc"); err == nil {
		t.Error("expected error for ambiguous prefix")
	}
}

func TestFindByPrefixNoMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "abc123"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := findByPrefix(dir, "zzz"); err == nil {
		t.Error("expected error for no match")
	}
}

func TestFindByPrefixMissingDir(t *testing.T) {
	if _, err := findByPrefix(filepath.Join(t.TempDir(), "nonexistent"), "abc"); err == nil {
		t.Error("expected error for missing directory")
	}
}
