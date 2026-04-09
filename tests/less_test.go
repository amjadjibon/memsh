package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/amjadjibon/memsh/pkg/shell/plugins/native"
)

// parsePagerOutput splits the pager sentinel output into metadata+content.
func parsePagerOutput(raw string) (startLine int, showNumbers bool, content string, ok bool) {
	if !strings.HasPrefix(raw, native.PagerSentinel) {
		return 0, false, "", false
	}
	body := raw[len(native.PagerSentinel):]
	nl := strings.Index(body, "\n")
	if nl < 0 {
		return 0, false, body, true
	}
	meta := struct {
		Start   int  `json:"start"`
		Numbers bool `json:"numbers"`
	}{Numbers: true}
	_ = json.Unmarshal([]byte(body[:nl]), &meta)
	return meta.Start, meta.Numbers, body[nl+1:], true
}

func TestLess(t *testing.T) {
	// pagerCtx simulates the HTTP server context (pager mode enabled).
	pagerCtx := native.WithPagerMode(context.Background())
	// replCtx simulates the REPL (no pager mode).
	replCtx := context.Background()

	newShell := func(t *testing.T, buf *strings.Builder, fs afero.Fs) *shell.Shell {
		t.Helper()
		return NewTestShell(t, buf, shell.WithFS(fs))
	}

	t.Run("pager mode emits sentinel", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/data.txt", []byte("line1\nline2\nline3\n"), 0o644)
		var buf strings.Builder
		s := newShell(t, &buf, fs)
		if err := s.Run(pagerCtx, "less /data.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.HasPrefix(out, native.PagerSentinel) {
			t.Fatalf("expected pager sentinel prefix, got: %q", out[:min(len(out), 40)])
		}
		_, _, content, ok := parsePagerOutput(out)
		if !ok {
			t.Fatal("failed to parse pager output")
		}
		if !strings.Contains(content, "line1") || !strings.Contains(content, "line3") {
			t.Errorf("expected file content, got: %q", content)
		}
	})

	t.Run("repl mode outputs raw content no sentinel", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/data.txt", []byte("hello\nworld\n"), 0o644)
		var buf strings.Builder
		s := newShell(t, &buf, fs)
		if err := s.Run(replCtx, "less /data.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if strings.HasPrefix(out, native.PagerSentinel) {
			t.Error("REPL mode must not emit sentinel")
		}
		if !strings.Contains(out, "hello") || !strings.Contains(out, "world") {
			t.Errorf("expected raw content in REPL mode, got: %q", out)
		}
	})

	t.Run("more alias pager mode", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("hello\n"), 0o644)
		var buf strings.Builder
		s := newShell(t, &buf, fs)
		if err := s.Run(pagerCtx, "more /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(buf.String(), native.PagerSentinel) {
			t.Error("expected pager sentinel from 'more'")
		}
	})

	t.Run("more alias repl mode no sentinel", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("hi\n"), 0o644)
		var buf strings.Builder
		s := newShell(t, &buf, fs)
		if err := s.Run(replCtx, "more /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.HasPrefix(buf.String(), native.PagerSentinel) {
			t.Error("REPL mode must not emit sentinel")
		}
	})

	t.Run("reads from stdin when no file (pager mode)", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(pagerCtx, "echo piped | less"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.HasPrefix(out, native.PagerSentinel) {
			t.Errorf("expected sentinel, got: %q", out[:min(len(out), 40)])
		}
	})

	t.Run("+N sets start line in metadata", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("a\nb\nc\nd\ne\n"), 0o644)
		var buf strings.Builder
		s := newShell(t, &buf, fs)
		if err := s.Run(pagerCtx, "less +3 /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		startLine, _, _, ok := parsePagerOutput(buf.String())
		if !ok {
			t.Fatal("failed to parse pager output")
		}
		if startLine != 2 { // +3 → 0-based index 2
			t.Errorf("expected startLine=2, got %d", startLine)
		}
	})

	t.Run("-n disables line numbers", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("x\n"), 0o644)
		var buf strings.Builder
		s := newShell(t, &buf, fs)
		if err := s.Run(pagerCtx, "less -n /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, showNums, _, ok := parsePagerOutput(buf.String())
		if !ok {
			t.Fatal("failed to parse pager output")
		}
		if showNums {
			t.Error("expected showNumbers=false with -n flag")
		}
	})

	t.Run("-N enables line numbers", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("x\n"), 0o644)
		var buf strings.Builder
		s := newShell(t, &buf, fs)
		if err := s.Run(pagerCtx, "less -N /f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, showNums, _, ok := parsePagerOutput(buf.String())
		if !ok {
			t.Fatal("failed to parse pager output")
		}
		if !showNums {
			t.Error("expected showNumbers=true with -N flag")
		}
	})

	t.Run("unknown flags silently ignored", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("ok\n"), 0o644)
		var buf strings.Builder
		s := newShell(t, &buf, fs)
		if err := s.Run(pagerCtx, "less -FXr /f.txt"); err != nil {
			t.Fatalf("unexpected error with unknown flags: %v", err)
		}
		if !strings.HasPrefix(buf.String(), native.PagerSentinel) {
			t.Error("expected pager sentinel")
		}
	})

	t.Run("missing file reports error to stderr, sentinel on stdout", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var stdout, stderr strings.Builder
		s, err := shell.New(
			shell.WithFS(fs),
			shell.WithStdIO(strings.NewReader(""), &stdout, &stderr),
			shell.WithWASMEnabled(false),
		)
		if err != nil {
			t.Fatalf("shell.New: %v", err)
		}
		if runErr := s.Run(pagerCtx, "less /nonexistent.txt"); runErr != nil {
			t.Fatalf("unexpected error: %v", runErr)
		}
		if !strings.HasPrefix(stdout.String(), native.PagerSentinel) {
			t.Errorf("expected sentinel on stdout, got: %q", stdout.String()[:min(len(stdout.String()), 40)])
		}
		if !strings.Contains(stderr.String(), "nonexistent") {
			t.Errorf("expected error message on stderr, got: %q", stderr.String())
		}
	})

	t.Run("pipeline output goes to pager (pager mode)", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := newShell(t, &buf, fs)
		if err := s.Run(pagerCtx, "seq 5 | less"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, _, content, ok := parsePagerOutput(buf.String())
		if !ok {
			t.Fatal("failed to parse pager output")
		}
		for _, want := range []string{"1", "2", "3", "4", "5"} {
			if !strings.Contains(content, want) {
				t.Errorf("expected %q in content, got: %q", want, content[:min(len(content), 60)])
			}
		}
	})

	t.Run("pipeline output goes to stdout in repl mode", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := newShell(t, &buf, fs)
		if err := s.Run(replCtx, "seq 3 | less"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if strings.HasPrefix(out, native.PagerSentinel) {
			t.Error("REPL mode must not emit sentinel")
		}
		for _, want := range []string{"1", "2", "3"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected %q in output, got: %q", want, out)
			}
		}
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
