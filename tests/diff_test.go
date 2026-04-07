package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/shell"
)

func TestDiff(t *testing.T) {
	ctx := context.Background()

	setup := func(t *testing.T) (*shell.Shell, *strings.Builder, afero.Fs) {
		t.Helper()
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		return s, &buf, fs
	}

	writeFile := func(fs afero.Fs, path, content string) {
		t.Helper()
		if err := afero.WriteFile(fs, path, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
	}

	t.Run("identical files exit 0 no output", func(t *testing.T) {
		s, buf, fs := setup(t)
		writeFile(fs, "/a.txt", "hello\nworld\n")
		writeFile(fs, "/b.txt", "hello\nworld\n")
		err := s.Run(ctx, "diff /a.txt /b.txt")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if buf.String() != "" {
			t.Errorf("expected empty output, got %q", buf.String())
		}
	})

	t.Run("normal diff shows added lines", func(t *testing.T) {
		s, buf, fs := setup(t)
		writeFile(fs, "/a.txt", "line1\nline2\n")
		writeFile(fs, "/b.txt", "line1\nline2\nline3\n")
		err := s.Run(ctx, "diff /a.txt /b.txt")
		if exitCode(err) != 1 {
			t.Fatalf("expected exit 1, got %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "> line3") {
			t.Errorf("expected '> line3' in output, got:\n%s", out)
		}
	})

	t.Run("normal diff shows deleted lines", func(t *testing.T) {
		s, buf, fs := setup(t)
		writeFile(fs, "/a.txt", "line1\nline2\nline3\n")
		writeFile(fs, "/b.txt", "line1\nline3\n")
		err := s.Run(ctx, "diff /a.txt /b.txt")
		if exitCode(err) != 1 {
			t.Fatalf("expected exit 1, got %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "< line2") {
			t.Errorf("expected '< line2' in output, got:\n%s", out)
		}
	})

	t.Run("unified diff has hunk header", func(t *testing.T) {
		s, buf, fs := setup(t)
		writeFile(fs, "/a.txt", "aaa\nbbb\nccc\n")
		writeFile(fs, "/b.txt", "aaa\nBBB\nccc\n")
		err := s.Run(ctx, "diff -u /a.txt /b.txt")
		if exitCode(err) != 1 {
			t.Fatalf("expected exit 1, got %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "@@ ") {
			t.Errorf("expected @@ hunk header, got:\n%s", out)
		}
		if !strings.Contains(out, "--- /a.txt") {
			t.Errorf("expected --- header, got:\n%s", out)
		}
		if !strings.Contains(out, "+++ /b.txt") {
			t.Errorf("expected +++ header, got:\n%s", out)
		}
		if !strings.Contains(out, "-bbb") {
			t.Errorf("expected -bbb line, got:\n%s", out)
		}
		if !strings.Contains(out, "+BBB") {
			t.Errorf("expected +BBB line, got:\n%s", out)
		}
	})

	t.Run("unified diff context lines", func(t *testing.T) {
		s, buf, fs := setup(t)
		// 10 lines, change line 5
		writeFile(fs, "/a.txt", "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n")
		writeFile(fs, "/b.txt", "1\n2\n3\n4\nX\n6\n7\n8\n9\n10\n")
		err := s.Run(ctx, "diff -u /a.txt /b.txt")
		if exitCode(err) != 1 {
			t.Fatalf("expected exit 1, got %v", err)
		}
		out := buf.String()
		// with 3 context lines, should see lines 2,3,4 before and 6,7,8 after
		if !strings.Contains(out, " 2") || !strings.Contains(out, " 8") {
			t.Errorf("expected context lines, got:\n%s", out)
		}
	})

	t.Run("-U 0 no context lines", func(t *testing.T) {
		s, buf, fs := setup(t)
		writeFile(fs, "/a.txt", "aaa\nbbb\nccc\n")
		writeFile(fs, "/b.txt", "aaa\nBBB\nccc\n")
		err := s.Run(ctx, "diff -U 0 /a.txt /b.txt")
		if exitCode(err) != 1 {
			t.Fatalf("expected exit 1, got %v", err)
		}
		out := buf.String()
		// context lines should not appear
		if strings.Contains(out, " aaa") || strings.Contains(out, " ccc") {
			t.Errorf("expected no context lines with -U 0, got:\n%s", out)
		}
	})

	t.Run("-q quiet mode", func(t *testing.T) {
		s, buf, fs := setup(t)
		writeFile(fs, "/a.txt", "hello\n")
		writeFile(fs, "/b.txt", "world\n")
		err := s.Run(ctx, "diff -q /a.txt /b.txt")
		if exitCode(err) != 1 {
			t.Fatalf("expected exit 1, got %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if !strings.Contains(out, "differ") {
			t.Errorf("expected 'differ' message, got %q", out)
		}
		if strings.Contains(out, "< ") || strings.Contains(out, "> ") {
			t.Errorf("quiet mode should not show diff lines, got %q", out)
		}
	})

	t.Run("-i ignore case", func(t *testing.T) {
		s, _, fs := setup(t)
		writeFile(fs, "/a.txt", "Hello\n")
		writeFile(fs, "/b.txt", "hello\n")
		err := s.Run(ctx, "diff -i /a.txt /b.txt")
		if err != nil {
			t.Fatalf("case-insensitive diff should find no diff, got %v", err)
		}
	})

	t.Run("-b ignore space change", func(t *testing.T) {
		s, _, fs := setup(t)
		writeFile(fs, "/a.txt", "hello  world\n")
		writeFile(fs, "/b.txt", "hello world\n")
		err := s.Run(ctx, "diff -b /a.txt /b.txt")
		if err != nil {
			t.Fatalf("ignore-space diff should find no diff, got %v", err)
		}
	})

	t.Run("-w ignore all whitespace", func(t *testing.T) {
		s, _, fs := setup(t)
		writeFile(fs, "/a.txt", "hello world\n")
		writeFile(fs, "/b.txt", "helloworld\n")
		err := s.Run(ctx, "diff -w /a.txt /b.txt")
		if err != nil {
			t.Fatalf("ignore-all-space diff should find no diff, got %v", err)
		}
	})

	t.Run("color output has ANSI codes", func(t *testing.T) {
		s, buf, fs := setup(t)
		writeFile(fs, "/a.txt", "aaa\n")
		writeFile(fs, "/b.txt", "bbb\n")
		err := s.Run(ctx, "diff --color /a.txt /b.txt")
		if exitCode(err) != 1 {
			t.Fatalf("expected exit 1, got %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "\033[") {
			t.Errorf("expected ANSI codes in color output, got %q", out)
		}
	})

	t.Run("missing operand exits 2", func(t *testing.T) {
		s, _, fs := setup(t)
		writeFile(fs, "/a.txt", "x\n")
		err := s.Run(ctx, "diff /a.txt")
		if exitCode(err) != 2 {
			t.Errorf("expected exit 2, got %v", err)
		}
	})
}
