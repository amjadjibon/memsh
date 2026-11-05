package session

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func newQuota(t *testing.T, l Limits, seed map[string]string) *QuotaFs {
	t.Helper()
	base := afero.NewMemMapFs()
	for p, c := range seed {
		if err := afero.WriteFile(base, p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fs, err := l.WrapFS(base)
	if err != nil {
		t.Fatal(err)
	}
	q, ok := fs.(*QuotaFs)
	if !ok {
		t.Fatalf("WrapFS returned %T, want *QuotaFs", fs)
	}
	return q
}

func TestWrapFSWithoutLimitsIsPassthrough(t *testing.T) {
	base := afero.NewMemMapFs()
	fs, err := Limits{}.WrapFS(base)
	if err != nil || fs != base {
		t.Fatalf("WrapFS() = %v, %v; want base fs unchanged", fs, err)
	}
}

func TestQuotaFsRejectsWritePastByteLimit(t *testing.T) {
	q := newQuota(t, Limits{MaxBytes: 10}, map[string]string{"/a": "1234"})

	f, err := q.Create("/b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("123456")); err != nil {
		t.Fatalf("write within limit: %v", err)
	}
	if _, err := f.Write([]byte("x")); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("write past limit err = %v, want ErrQuotaExceeded", err)
	}
	_ = f.Close()
	if _, b := q.Usage(); b != 10 {
		t.Fatalf("usage = %d bytes, want 10", b)
	}
	// Overwriting existing bytes does not grow the file.
	if err := afero.WriteFile(q, "/a", []byte("abcd"), 0o644); err != nil {
		t.Fatalf("same-size overwrite: %v", err)
	}
}

func TestQuotaFsAppendAndTruncate(t *testing.T) {
	q := newQuota(t, Limits{MaxBytes: 8}, map[string]string{"/log": "1234"})

	f, err := q.OpenFile("/log", os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("5678"); err != nil {
		t.Fatalf("append to limit: %v", err)
	}
	if _, err := f.WriteString("9"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("append past limit err = %v", err)
	}
	if err := f.Truncate(2); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, b := q.Usage(); b != 2 {
		t.Fatalf("usage after truncate = %d, want 2", b)
	}
	// O_TRUNC on open refunds the old size.
	if err := afero.WriteFile(q, "/log", []byte("abcdefgh"), 0o644); err != nil {
		t.Fatalf("rewrite to limit: %v", err)
	}
}

func TestQuotaFsFileLimitAndRefunds(t *testing.T) {
	q := newQuota(t, Limits{MaxFiles: 2}, map[string]string{"/a": "x"})

	if err := afero.WriteFile(q, "/b", []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(q, "/c", nil, 0o644); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("third file err = %v, want ErrQuotaExceeded", err)
	}
	if ok, _ := afero.Exists(q, "/c"); ok {
		t.Fatal("rejected file was created")
	}
	if err := q.Remove("/b"); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(q, "/c", nil, 0o644); err != nil {
		t.Fatalf("create after remove: %v", err)
	}
	// Renaming over an existing file frees the destination.
	if err := q.Rename("/c", "/a"); err != nil {
		t.Fatal(err)
	}
	if files, _ := q.Usage(); files != 1 {
		t.Fatalf("files after rename-over = %d, want 1", files)
	}
	if err := q.MkdirAll("/d/e", 0o755); err != nil {
		t.Fatal(err)
	}
	_ = afero.WriteFile(q, "/d/e/f", []byte("z"), 0o644)
	if err := q.RemoveAll("/d"); err != nil {
		t.Fatal(err)
	}
	if files, _ := q.Usage(); files != 1 {
		t.Fatalf("files after RemoveAll = %d, want 1", files)
	}
}

func TestLimitedBuffer(t *testing.T) {
	b := NewLimitedBuffer(5)
	if n, err := b.Write([]byte("abc")); n != 3 || err != nil {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if n, err := b.Write([]byte("defg")); n != 2 || err == nil {
		t.Fatalf("Write past cap = %d, %v; want 2, error", n, err)
	}
	if got := b.String(); !strings.HasPrefix(got, "abcde") || !strings.Contains(got, "truncated") {
		t.Fatalf("String() = %q", got)
	}
	b.Reset()
	if b.String() != "" {
		t.Fatal("Reset did not clear")
	}
}
