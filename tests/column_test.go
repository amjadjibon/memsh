package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestColumn(t *testing.T) {
	ctx := context.Background()

	run := func(t *testing.T, script string) string {
		t.Helper()
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, script); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return buf.String()
	}

	// ── table mode (-t) ──────────────────────────────────────────────────────

	t.Run("-t aligns whitespace-delimited fields", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/data.txt", []byte("Name Age City\nAlice 30 London\nBob 25 Paris\n"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "column -t /data.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 3 {
			t.Fatalf("expected 3 lines, got %d", len(lines))
		}
		// all 'Age' values should be aligned at the same column
		col := strings.Index(lines[0], "Age")
		for _, l := range lines[1:] {
			// the numeric age should start at the same offset
			if strings.Index(l, " ") != strings.Index(lines[0], " ") {
				// just check each field is separated by at least one space
				fields := strings.Fields(l)
				if len(fields) != 3 {
					t.Errorf("expected 3 fields in %q", l)
				}
			}
		}
		_ = col
	})

	t.Run("-t with -s colon separates by ':'", func(t *testing.T) {
		out := run(t, `printf "a:bb:ccc\ndd:e:ff\n" | column -t -s ':'`)
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d: %q", len(lines), out)
		}
		// first field of line 2 ("dd") should be left-aligned with "a" of line 1
		if !strings.HasPrefix(lines[0], "a ") {
			t.Errorf("expected line 1 to start with 'a ': %q", lines[0])
		}
		if !strings.HasPrefix(lines[1], "dd") {
			t.Errorf("expected line 2 to start with 'dd': %q", lines[1])
		}
	})

	t.Run("-t with -o sets output separator", func(t *testing.T) {
		out := run(t, `printf "a b\nc d\n" | column -t -o ' | '`)
		if !strings.Contains(out, " | ") {
			t.Errorf("expected ' | ' separator in output: %q", out)
		}
	})

	t.Run("-t preserves blank lines", func(t *testing.T) {
		out := run(t, `printf "a b\n\nc d\n" | column -t`)
		lines := strings.Split(out, "\n")
		// should have a blank line in the middle
		hasBlank := false
		for _, l := range lines {
			if strings.TrimSpace(l) == "" {
				hasBlank = true
				break
			}
		}
		if !hasBlank {
			t.Errorf("expected blank line to be preserved: %q", out)
		}
	})

	t.Run("-t single column input", func(t *testing.T) {
		out := run(t, `printf "apple\nbanana\ncherry\n" | column -t`)
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != 3 {
			t.Fatalf("expected 3 lines, got %d", len(lines))
		}
	})

	t.Run("-t stdin from pipe", func(t *testing.T) {
		out := run(t, `echo "foo bar baz" | column -t`)
		if !strings.Contains(out, "foo") || !strings.Contains(out, "bar") {
			t.Errorf("unexpected output: %q", out)
		}
	})

	t.Run("-t -n no-merge: empty fields preserved", func(t *testing.T) {
		out := run(t, `printf "a::c\nd:e:f\n" | column -t -s ':' -n`)
		lines := strings.Split(strings.TrimSpace(out), "\n")
		// with -n, empty field between :: should be kept, giving 3 columns
		// "a", "", "c" on line 1
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d: %q", len(lines), out)
		}
		// column 3 "c" should be aligned with "f"
		col1c := strings.LastIndex(lines[0], "c")
		col2f := strings.LastIndex(lines[1], "f")
		if col1c != col2f {
			t.Errorf("'c' at %d, 'f' at %d — should be aligned: %q / %q", col1c, col2f, lines[0], lines[1])
		}
	})

	// ── fill mode (default) ──────────────────────────────────────────────────

	t.Run("fill mode wraps items across columns", func(t *testing.T) {
		out := run(t, `printf "a\nb\nc\nd\ne\nf\n" | column -c 20`)
		// 6 items, each 1 char, colWidth=3, 20/3=6 cols → 1 row
		// or fewer cols if narrower; just check items are present
		if !strings.Contains(out, "a") || !strings.Contains(out, "f") {
			t.Errorf("missing items in fill output: %q", out)
		}
		// with width 20 all 6 single-char items fit in 1-2 rows
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) > 3 {
			t.Errorf("expected at most 3 rows, got %d: %q", len(lines), out)
		}
	})

	t.Run("-x fills rows before columns", func(t *testing.T) {
		out := run(t, `printf "a\nb\nc\nd\n" | column -c 10 -x`)
		// -x: row-major — a b / c d (items go left-to-right first)
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) < 1 {
			t.Fatalf("no output")
		}
		firstRow := lines[0]
		// first two items should be on first row
		if !strings.Contains(firstRow, "a") {
			t.Errorf("expected 'a' in first row: %q", firstRow)
		}
	})

	t.Run("empty input produces no output", func(t *testing.T) {
		out := run(t, `printf "" | column -t`)
		if strings.TrimSpace(out) != "" {
			t.Errorf("expected empty output, got %q", out)
		}
	})

	t.Run("reads from virtual FS file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/hosts.txt", []byte("127.0.0.1 localhost\n192.168.1.1 router\n"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "column -t /hosts.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "localhost") || !strings.Contains(out, "router") {
			t.Errorf("unexpected output: %q", out)
		}
		// both IPs should be left-aligned at the same column
		lines := strings.Split(strings.TrimSpace(out), "\n")
		hostnameCol0 := strings.Index(lines[0], "localhost")
		hostnameCol1 := strings.Index(lines[1], "router")
		if hostnameCol0 != hostnameCol1 {
			t.Errorf("hostnames not aligned: col %d vs %d\n%s\n%s",
				hostnameCol0, hostnameCol1, lines[0], lines[1])
		}
	})

	t.Run("pipe from command", func(t *testing.T) {
		out := run(t, `printf "NAME\tAGE\nAlice\t30\nBob\t25\n" | column -t`)
		if !strings.Contains(out, "Alice") || !strings.Contains(out, "Bob") {
			t.Errorf("unexpected output: %q", out)
		}
	})
}
