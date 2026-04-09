package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestSQLiteBasic(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

	script := `sqlite3 :memory: "CREATE TABLE users (id INTEGER, name TEXT); INSERT INTO users VALUES (1,'Alice'); SELECT id, name FROM users;"`
	if err := s.Run(ctx, script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if !strings.Contains(out, "1") || !strings.Contains(out, "Alice") {
		t.Errorf("expected row output, got %q", out)
	}
}

func TestSQLiteCSVMode(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

	script := `sqlite3 -csv -header :memory: "CREATE TABLE t (a TEXT, b INTEGER); INSERT INTO t VALUES ('hello', 42); SELECT a, b FROM t;"`
	if err := s.Run(ctx, script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %q", out)
	}
	if lines[0] != "a,b" {
		t.Errorf("expected CSV header 'a,b', got %q", lines[0])
	}
	if !strings.Contains(lines[1], "hello") || !strings.Contains(lines[1], "42") {
		t.Errorf("expected data row with hello and 42, got %q", lines[1])
	}
}

func TestSQLiteJSONMode(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

	script := `sqlite3 -json :memory: "CREATE TABLE t (name TEXT, val INTEGER); INSERT INTO t VALUES ('foo', 10); SELECT name, val FROM t;"`
	if err := s.Run(ctx, script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(out, "[") || !strings.HasSuffix(out, "]") {
		t.Errorf("expected JSON array, got %q", out)
	}
	if !strings.Contains(out, `"foo"`) || !strings.Contains(out, `10`) {
		t.Errorf("expected foo and 10 in JSON output, got %q", out)
	}
}

func TestSQLiteLineMode(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

	script := `sqlite3 -line :memory: "CREATE TABLE t (x TEXT); INSERT INTO t VALUES ('bar'); SELECT x FROM t;"`
	if err := s.Run(ctx, script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if !strings.Contains(out, "x = bar") {
		t.Errorf("expected 'x = bar' in line mode output, got %q", out)
	}
}

func TestSQLiteTableMode(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

	script := `sqlite3 -table :memory: "CREATE TABLE t (col1 TEXT, col2 INTEGER); INSERT INTO t VALUES ('data', 99); SELECT col1, col2 FROM t;"`
	if err := s.Run(ctx, script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	// table mode always shows headers and box borders
	if !strings.Contains(out, "col1") {
		t.Errorf("expected column name 'col1' in table output, got %q", out)
	}
	if !strings.Contains(out, "col2") {
		t.Errorf("expected column name 'col2' in table output, got %q", out)
	}
	if !strings.Contains(out, "data") {
		t.Errorf("expected 'data' in table output, got %q", out)
	}
	// box border characters
	if !strings.Contains(out, "┌") || !strings.Contains(out, "└") {
		t.Errorf("expected box borders in table output, got %q", out)
	}
}

func TestSQLiteFromStdin(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder

	sql := "CREATE TABLE t (n INTEGER); INSERT INTO t VALUES (7); SELECT n FROM t;"
	s, err := shell.New(
		shell.WithFS(afero.NewMemMapFs()),
		shell.WithStdIO(strings.NewReader(sql), &buf, &buf),
		shell.WithWASMEnabled(false),
	)
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}

	if err := s.Run(ctx, "sqlite3 :memory:"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out != "7" {
		t.Errorf("expected '7', got %q", out)
	}
}

func TestSQLiteDotTables(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

	script := `sqlite3 :memory: "CREATE TABLE alpha (id INTEGER); CREATE TABLE beta (id INTEGER); .tables"`
	if err := s.Run(ctx, script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if !strings.Contains(out, "alpha") {
		t.Errorf("expected 'alpha' in .tables output, got %q", out)
	}
	if !strings.Contains(out, "beta") {
		t.Errorf("expected 'beta' in .tables output, got %q", out)
	}
}

func TestSQLiteDotSchema(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

	script := `sqlite3 :memory: "CREATE TABLE items (id INTEGER PRIMARY KEY, label TEXT); .schema"`
	if err := s.Run(ctx, script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if !strings.Contains(out, "CREATE TABLE") {
		t.Errorf("expected CREATE TABLE in .schema output, got %q", out)
	}
	if !strings.Contains(out, "items") {
		t.Errorf("expected 'items' in .schema output, got %q", out)
	}
}

func TestSQLiteMultiStatement(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

	script := `sqlite3 :memory: "CREATE TABLE t (v INTEGER); INSERT INTO t VALUES (1); INSERT INTO t VALUES (2); SELECT v FROM t ORDER BY v;"`
	if err := s.Run(ctx, script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	lines := strings.Split(out, "\n")
	if len(lines) != 2 || lines[0] != "1" || lines[1] != "2" {
		t.Errorf("expected '1\n2', got %q", out)
	}
}

func TestSQLiteNullValue(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

	script := `sqlite3 -nullvalue "N/A" :memory: "CREATE TABLE t (v TEXT); INSERT INTO t VALUES (NULL); SELECT v FROM t;"`
	if err := s.Run(ctx, script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out != "N/A" {
		t.Errorf("expected 'N/A', got %q", out)
	}
}

func TestSQLiteVirtualFS(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()

	// Shell 1: create a table and insert a row into /test.db
	var buf1 strings.Builder
	s1 := NewTestShell(t, &buf1, shell.WithFS(fs))
	script1 := `sqlite3 /test.db "CREATE TABLE kv (key TEXT, val TEXT); INSERT INTO kv VALUES ('greeting','hello');"`
	if err := s1.Run(ctx, script1); err != nil {
		t.Fatalf("shell1 error: %v", err)
	}

	// Shell 2: open the same /test.db from the shared FS and query it
	var buf2 strings.Builder
	s2 := NewTestShell(t, &buf2, shell.WithFS(fs))
	script2 := `sqlite3 /test.db "SELECT val FROM kv WHERE key='greeting';"`
	if err := s2.Run(ctx, script2); err != nil {
		t.Fatalf("shell2 error: %v", err)
	}
	out := strings.TrimSpace(buf2.String())
	if out != "hello" {
		t.Errorf("expected 'hello' from persisted db, got %q", out)
	}
}

func TestSQLiteDotDump(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

	script := `sqlite3 :memory: "CREATE TABLE dumptbl (id INTEGER, name TEXT); INSERT INTO dumptbl VALUES (1,'Row1'); .dump"`
	if err := s.Run(ctx, script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "CREATE TABLE") {
		t.Errorf("expected CREATE TABLE in dump output, got %q", out)
	}
	if !strings.Contains(out, "INSERT INTO") {
		t.Errorf("expected INSERT INTO in dump output, got %q", out)
	}
	if !strings.Contains(out, "Row1") {
		t.Errorf("expected 'Row1' in dump output, got %q", out)
	}
	if !strings.Contains(out, "BEGIN TRANSACTION") {
		t.Errorf("expected BEGIN TRANSACTION in dump output, got %q", out)
	}
	if !strings.Contains(out, "COMMIT") {
		t.Errorf("expected COMMIT in dump output, got %q", out)
	}
}

// TestSQLiteREPLViaPipe verifies that dot commands and multi-statement SQL work
// when piped via stdin (the non-terminal code path that mirrors REPL input).
func TestSQLiteREPLViaPipe(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder

	input := strings.NewReader(`CREATE TABLE items (id INTEGER, label TEXT);
INSERT INTO items VALUES (1, 'alpha');
INSERT INTO items VALUES (2, 'beta');
SELECT id, label FROM items;
.tables
`)
	s, err := shell.New(
		shell.WithStdIO(input, &buf, &buf),
		shell.WithWASMEnabled(false),
		shell.WithFS(afero.NewMemMapFs()),
	)
	if err != nil {
		t.Fatalf("shell.New: %v", err)
	}
	defer s.Close()

	if err := s.Run(ctx, "sqlite3 :memory:"); err != nil {
		t.Fatalf("sqlite3 repl-via-pipe: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "1|alpha") {
		t.Errorf("expected row '1|alpha', got %q", out)
	}
	if !strings.Contains(out, "2|beta") {
		t.Errorf("expected row '2|beta', got %q", out)
	}
	if !strings.Contains(out, "items") {
		t.Errorf("expected '.tables' to list 'items', got %q", out)
	}
}

func TestSQLiteSeparator(t *testing.T) {
	ctx := context.Background()
	var buf strings.Builder
	s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

	script := `sqlite3 -separator , :memory: "CREATE TABLE t (a TEXT, b TEXT); INSERT INTO t VALUES ('x','y'); SELECT a, b FROM t;"`
	if err := s.Run(ctx, script); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out != "x,y" {
		t.Errorf("expected 'x,y' with comma separator, got %q", out)
	}
}
