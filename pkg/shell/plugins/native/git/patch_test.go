package git

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// TestApplyPatchLongLine checks that a line longer than bufio's default
// 64 KiB token limit is applied rather than silently ending the parse.
func TestApplyPatchLongLine(t *testing.T) {
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/r/a.txt", []byte("old\n"), 0o644)
	_ = afero.WriteFile(fs, "/r/b.txt", []byte("one\n"), 0o644)

	long := strings.Repeat("x", 100_000)
	patch := "--- a/a.txt\n+++ b/a.txt\n@@ -1,1 +1,1 @@\n-old\n+" + long + "\n" +
		"--- a/b.txt\n+++ b/b.txt\n@@ -1,1 +1,1 @@\n-one\n+two\n"

	modified, err := applyPatch(fs, "/r", []byte(patch))
	if err != nil {
		t.Fatalf("applyPatch: %v", err)
	}
	if len(modified) != 2 {
		t.Fatalf("modified %v, want both files", modified)
	}
	if got, _ := afero.ReadFile(fs, "/r/a.txt"); strings.TrimSpace(string(got)) != long {
		t.Errorf("a.txt not updated (len %d)", len(got))
	}
	if got, _ := afero.ReadFile(fs, "/r/b.txt"); strings.TrimSpace(string(got)) != "two" {
		t.Errorf("b.txt = %q, want two", got)
	}
}
