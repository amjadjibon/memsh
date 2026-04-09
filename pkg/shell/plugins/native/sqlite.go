// Package native contains the built-in native Go plugins shipped with memsh.
package native

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/afero"
	"golang.org/x/term"
	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"

	// register the modernc pure-Go SQLite driver
	_ "modernc.org/sqlite"
)

// SQLitePlugin implements an sqlite3-like CLI against the virtual filesystem.
//
//	sqlite3 [flags] [DBFILE] [SQL...]
//
// DBFILE defaults to ":memory:". When a path is given the database bytes are
// copied from afero to a temp OS file, operated on, then written back.
type SQLitePlugin struct{}

func (SQLitePlugin) Name() string        { return "sqlite3" }
func (SQLitePlugin) Description() string { return "SQLite database CLI" }
func (SQLitePlugin) Usage() string {
	return "sqlite3 [-csv|-json|-line|-column|-table|-list] [-header] [-echo] [-bail] [-separator S] [-nullvalue S] [-cmd SQL] [DBFILE] [SQL...]"
}

// compile-time check
var _ plugins.PluginInfo = SQLitePlugin{}

// outputMode describes how query results are rendered.
type outputMode int

const (
	modeList   outputMode = iota // default: pipe-delimited
	modeCSV                      // RFC 4180 CSV
	modeJSON                     // JSON array
	modeLine                     // col = val per column per row
	modeColumn                   // space-aligned columns
	modeTable                    // Unicode box table
)

// sqliteOpts holds the parsed flag state for one invocation.
type sqliteOpts struct {
	mode      outputMode
	header    bool
	echo      bool
	bail      bool
	sep       string
	nullValue string
	preCmds   []string // -cmd SQL
}

func (SQLitePlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	opts := sqliteOpts{
		mode: modeList,
		sep:  "|",
	}

	var positional []string
	endFlags := false

	for i := 1; i < len(args); i++ {
		a := args[i]
		if endFlags || a == "" || (len(a) > 0 && a[0] != '-') {
			positional = append(positional, a)
			continue
		}
		if a == "--" {
			endFlags = true
			continue
		}
		switch a {
		case "-csv":
			opts.mode = modeCSV
		case "-json":
			opts.mode = modeJSON
		case "-line":
			opts.mode = modeLine
		case "-column":
			opts.mode = modeColumn
		case "-table":
			opts.mode = modeTable
		case "-list":
			opts.mode = modeList
		case "-header":
			opts.header = true
		case "-echo":
			opts.echo = true
		case "-bail":
			opts.bail = true
		case "-separator":
			i++
			if i >= len(args) {
				return fmt.Errorf("sqlite3: -separator requires an argument")
			}
			opts.sep = args[i]
		case "-nullvalue":
			i++
			if i >= len(args) {
				return fmt.Errorf("sqlite3: -nullvalue requires an argument")
			}
			opts.nullValue = args[i]
		case "-cmd":
			i++
			if i >= len(args) {
				return fmt.Errorf("sqlite3: -cmd requires an argument")
			}
			opts.preCmds = append(opts.preCmds, args[i])
		default:
			// unknown flag — treat as positional (mirrors real sqlite3 behaviour)
			positional = append(positional, a)
		}
	}

	// First positional arg is the DB file; remainder are SQL statements.
	dbPath := ":memory:"
	var sqlArgs []string
	if len(positional) > 0 {
		dbPath = positional[0]
		sqlArgs = positional[1:]
	}

	// --- open the database ------------------------------------------------
	db, cleanup, err := openDB(sc, dbPath)
	if err != nil {
		return fmt.Errorf("sqlite3: %w", err)
	}
	defer cleanup()

	// --- build the statement source ---------------------------------------
	// 1. -cmd statements (always executed first)
	// 2. SQL args (joined), or stdin if none
	var preCmds []string
	for _, pre := range opts.preCmds {
		preCmds = append(preCmds, splitStatements(pre)...)
	}

	// Execute -cmd statements
	for _, stmt := range preCmds {
		if err := execStatement(ctx, db, stmt, &opts, hc.Stdout, hc.Stderr, dbPath); err != nil {
			if opts.bail {
				return err
			}
		}
	}

	if len(sqlArgs) > 0 {
		// Batch mode: SQL passed as command-line argument
		joined := strings.Join(sqlArgs, " ")
		for _, stmt := range splitStatements(joined) {
			if err := execStatement(ctx, db, stmt, &opts, hc.Stdout, hc.Stderr, dbPath); err != nil {
				if opts.bail {
					return err
				}
			}
		}
		return nil
	}

	// Check if stdin is an interactive terminal
	if f, ok := hc.Stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		return sqliteREPL(ctx, db, &opts, hc.Stdin, hc.Stdout, dbPath)
	}

	// Non-interactive: read all stdin and execute
	raw, err := io.ReadAll(hc.Stdin)
	if err != nil {
		return err
	}
	for _, stmt := range splitStatements(string(raw)) {
		if err := execStatement(ctx, db, stmt, &opts, hc.Stdout, hc.Stderr, dbPath); err != nil {
			if opts.bail {
				return err
			}
		}
	}
	return nil
}

// execStatement executes one SQL statement or dot command, printing errors to stderr.
// Returns the error so callers can decide whether to bail.
func execStatement(ctx context.Context, db *sql.DB, stmt string, opts *sqliteOpts, out, errOut io.Writer, dbPath string) error {
	trimmed := strings.TrimSpace(stmt)
	if trimmed == "" {
		return nil
	}
	if opts.echo {
		fmt.Fprintf(out, "%s\n", trimmed)
	}
	var execErr error
	if strings.HasPrefix(trimmed, ".") {
		execErr = handleDotCmd(ctx, db, trimmed, opts, out, dbPath)
		if _, isQuit := execErr.(dotQuit); isQuit {
			return execErr
		}
	} else {
		upper := strings.ToUpper(trimmed)
		if isQueryStmt(upper) {
			execErr = queryAndFormat(ctx, db, trimmed, opts, out)
		} else {
			_, execErr = db.ExecContext(ctx, trimmed)
		}
	}
	if execErr != nil {
		fmt.Fprintf(errOut, "Error: %s\n", execErr.Error())
	}
	return execErr
}

// sqliteREPL runs an interactive sqlite> prompt loop.
func sqliteREPL(ctx context.Context, db *sql.DB, opts *sqliteOpts, in io.Reader, out io.Writer, dbPath string) error {
	// Print version banner
	var version string
	if row := db.QueryRowContext(ctx, "SELECT sqlite_version()"); row != nil {
		_ = row.Scan(&version)
	}
	if version == "" {
		version = "3.x"
	}
	fmt.Fprintf(out, "SQLite version %s\nEnter \".help\" for usage hints.\n", version)

	scanner := bufio.NewScanner(in)
	var buf strings.Builder

	for {
		// Print prompt
		if buf.Len() == 0 {
			fmt.Fprint(out, "sqlite> ")
		} else {
			fmt.Fprint(out, "   ...> ")
		}

		if !scanner.Scan() {
			break
		}
		line := scanner.Text()

		// Dot command: only at statement boundary
		if buf.Len() == 0 {
			trimLine := strings.TrimSpace(line)
			if strings.HasPrefix(trimLine, ".") {
				err := handleDotCmd(ctx, db, trimLine, opts, out, dbPath)
				if _, isQuit := err.(dotQuit); isQuit {
					return nil
				}
				if err != nil {
					fmt.Fprintf(out, "Error: %s\n", err)
				}
				continue
			}
			// blank line at boundary — skip
			if trimLine == "" {
				continue
			}
		}

		buf.WriteString(line)
		buf.WriteString("\n")

		// Execute when we have a complete statement ending with ';'
		trimmed := strings.TrimSpace(buf.String())
		if strings.HasSuffix(trimmed, ";") {
			for _, stmt := range splitStatements(trimmed) {
				err := execStatement(ctx, db, stmt, opts, out, out, dbPath)
				if _, isQuit := err.(dotQuit); isQuit {
					return nil
				}
			}
			buf.Reset()
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

// isQueryStmt returns true for statements that produce result rows.
func isQueryStmt(upper string) bool {
	for _, kw := range []string{"SELECT", "WITH", "PRAGMA", "EXPLAIN", "VALUES"} {
		if strings.HasPrefix(upper, kw) {
			return true
		}
	}
	return false
}

// openDB opens a SQLite database. For ":memory:" it opens directly; for any
// other path it copies afero bytes to a temp file, opens that, and returns a
// cleanup function that writes the bytes back and removes the temp file.
func openDB(sc plugins.ShellContext, dbPath string) (*sql.DB, func(), error) {
	if dbPath == ":memory:" {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			return nil, nil, err
		}
		return db, func() { db.Close() }, nil
	}

	absPath := sc.ResolvePath(dbPath)

	// copy virtual file → temp OS file
	tmp, err := os.CreateTemp("", "memsh-sqlite-*.db")
	if err != nil {
		return nil, nil, err
	}
	tmpName := tmp.Name()

	// try to read existing bytes from afero; ignore not-found
	if data, readErr := afero.ReadFile(sc.FS, absPath); readErr == nil {
		if _, writeErr := tmp.Write(data); writeErr != nil {
			tmp.Close()
			os.Remove(tmpName)
			return nil, nil, writeErr
		}
	}
	tmp.Close()

	db, err := sql.Open("sqlite", tmpName)
	if err != nil {
		os.Remove(tmpName)
		return nil, nil, err
	}

	cleanup := func() {
		db.Close()
		// write temp file bytes back into afero
		if written, readErr := os.ReadFile(tmpName); readErr == nil {
			_ = afero.WriteFile(sc.FS, absPath, written, 0o644)
		}
		os.Remove(tmpName)
	}

	return db, cleanup, nil
}

// splitStatements splits a SQL string into individual statements. It honours:
//   - single-quoted string literals  ('it”s fine')
//   - double-quoted identifiers      ("col name")
//   - back-tick identifiers          (`col name`)
//   - block comments                 /* ... */
//   - line comments                  -- ...
//   - ';' as statement terminator
//   - dot commands (lines starting with '.') as self-contained statements
func splitStatements(input string) []string {
	var stmts []string
	var cur strings.Builder

	flush := func() {
		s := strings.TrimSpace(cur.String())
		if s != "" {
			stmts = append(stmts, s)
		}
		cur.Reset()
	}

	runes := []rune(input)
	n := len(runes)

	for i := 0; i < n; {
		r := runes[i]

		// Dot command: only recognised at the start of a (logical) line when
		// the current buffer is empty.
		if r == '.' && strings.TrimSpace(cur.String()) == "" {
			// read to end of line
			flush()
			j := i
			for j < n && runes[j] != '\n' {
				j++
			}
			stmts = append(stmts, string(runes[i:j]))
			i = j
			continue
		}

		// Line comment  -- ...
		if r == '-' && i+1 < n && runes[i+1] == '-' {
			cur.WriteRune(r)
			cur.WriteRune(runes[i+1])
			i += 2
			for i < n && runes[i] != '\n' {
				cur.WriteRune(runes[i])
				i++
			}
			continue
		}

		// Block comment  /* ... */
		if r == '/' && i+1 < n && runes[i+1] == '*' {
			cur.WriteRune(r)
			cur.WriteRune(runes[i+1])
			i += 2
			for i < n {
				if runes[i] == '*' && i+1 < n && runes[i+1] == '/' {
					cur.WriteRune(runes[i])
					cur.WriteRune(runes[i+1])
					i += 2
					break
				}
				cur.WriteRune(runes[i])
				i++
			}
			continue
		}

		// Quoted strings / identifiers
		if r == '\'' || r == '"' || r == '`' {
			quote := r
			cur.WriteRune(r)
			i++
			for i < n {
				c := runes[i]
				cur.WriteRune(c)
				i++
				if c == quote {
					// doubled quote = escape
					if i < n && runes[i] == quote {
						cur.WriteRune(runes[i])
						i++
					} else {
						break
					}
				}
			}
			continue
		}

		// Statement terminator
		if r == ';' {
			cur.WriteRune(r)
			flush()
			i++
			continue
		}

		cur.WriteRune(r)
		i++
	}

	flush()
	return stmts
}

// handleDotCmd dispatches dot commands.
func handleDotCmd(ctx context.Context, db *sql.DB, stmt string, opts *sqliteOpts, out io.Writer, dbPath string) error {
	parts := tokeniseDotCmd(stmt)
	if len(parts) == 0 {
		return nil
	}
	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case ".quit", ".exit":
		return dotQuit{}

	case ".tables":
		return dotTables(ctx, db, args, out)

	case ".schema":
		return dotSchema(ctx, db, args, out)

	case ".databases":
		fmt.Fprintf(out, "main: %s\n", dbPath)

	case ".mode":
		if len(args) < 1 {
			return fmt.Errorf(".mode requires an argument")
		}
		switch args[0] {
		case "csv":
			opts.mode = modeCSV
		case "json":
			opts.mode = modeJSON
		case "line":
			opts.mode = modeLine
		case "column":
			opts.mode = modeColumn
		case "table":
			opts.mode = modeTable
		case "list":
			opts.mode = modeList
		default:
			return fmt.Errorf("unknown mode: %s", args[0])
		}

	case ".headers":
		if len(args) < 1 {
			return fmt.Errorf(".headers requires on|off")
		}
		switch strings.ToLower(args[0]) {
		case "on":
			opts.header = true
		case "off":
			opts.header = false
		default:
			return fmt.Errorf(".headers: expected on or off, got %s", args[0])
		}

	case ".separator":
		if len(args) < 1 {
			return fmt.Errorf(".separator requires an argument")
		}
		opts.sep = args[0]

	case ".nullvalue":
		if len(args) < 1 {
			return fmt.Errorf(".nullvalue requires an argument")
		}
		opts.nullValue = args[0]

	case ".dump":
		table := ""
		if len(args) > 0 {
			table = args[0]
		}
		return dotDump(ctx, db, table, out)

	default:
		return fmt.Errorf("unknown dot command: %s", cmd)
	}

	return nil
}

// dotQuit is a sentinel error used to stop processing on .quit/.exit.
type dotQuit struct{}

func (dotQuit) Error() string { return ".quit" }

// tokeniseDotCmd splits a dot command line into tokens, respecting quoted strings.
func tokeniseDotCmd(line string) []string {
	var tokens []string
	var cur strings.Builder
	inQ := false
	qChar := rune(0)

	for _, r := range line {
		if inQ {
			if r == qChar {
				inQ = false
			} else {
				cur.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			inQ = true
			qChar = r
			continue
		}
		if unicode.IsSpace(r) {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// dotTables prints table names, optionally filtered by a LIKE pattern.
func dotTables(ctx context.Context, db *sql.DB, args []string, out io.Writer) error {
	q := "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name"
	var queryArgs []any
	if len(args) > 0 {
		q = "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE ? ORDER BY name"
		queryArgs = []any{args[0]}
	}
	rows, err := db.QueryContext(ctx, q, queryArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		fmt.Fprintln(out, name)
	}
	return rows.Err()
}

// dotSchema prints the CREATE statements for tables/views.
func dotSchema(ctx context.Context, db *sql.DB, args []string, out io.Writer) error {
	q := "SELECT sql FROM sqlite_master WHERE sql IS NOT NULL ORDER BY type, name"
	var queryArgs []any
	if len(args) > 0 {
		q = "SELECT sql FROM sqlite_master WHERE sql IS NOT NULL AND name = ? ORDER BY type, name"
		queryArgs = []any{args[0]}
	}
	rows, err := db.QueryContext(ctx, q, queryArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return err
		}
		fmt.Fprintln(out, s+";")
	}
	return rows.Err()
}

// dotDump emits CREATE TABLE + INSERT INTO statements for all (or one) table.
func dotDump(ctx context.Context, db *sql.DB, table string, out io.Writer) error {
	fmt.Fprintln(out, "BEGIN TRANSACTION;")

	tableQ := "SELECT name, sql FROM sqlite_master WHERE type='table' ORDER BY name"
	var tableArgs []any
	if table != "" {
		tableQ = "SELECT name, sql FROM sqlite_master WHERE type='table' AND name=? ORDER BY name"
		tableArgs = []any{table}
	}

	rows, err := db.QueryContext(ctx, tableQ, tableArgs...)
	if err != nil {
		return err
	}
	var tables []struct{ name, createSQL string }
	for rows.Next() {
		var t struct{ name, createSQL string }
		if err := rows.Scan(&t.name, &t.createSQL); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, t := range tables {
		fmt.Fprintf(out, "%s;\n", t.createSQL)

		dataRows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %q", t.name)) //nolint:gosec
		if err != nil {
			return err
		}
		cols, _ := dataRows.Columns()
		for dataRows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := dataRows.Scan(ptrs...); err != nil {
				dataRows.Close()
				return err
			}
			colNames := make([]string, len(cols))
			for i, c := range cols {
				colNames[i] = fmt.Sprintf("%q", c)
			}
			valStrs := make([]string, len(vals))
			for i, v := range vals {
				valStrs[i] = sqliteValueToSQL(v)
			}
			fmt.Fprintf(out, "INSERT INTO %q (%s) VALUES (%s);\n",
				t.name,
				strings.Join(colNames, ", "),
				strings.Join(valStrs, ", "),
			)
		}
		dataRows.Close()
		if err := dataRows.Err(); err != nil {
			return err
		}
	}

	fmt.Fprintln(out, "COMMIT;")
	return nil
}

// sqliteValueToSQL formats a scanned value as a SQL literal.
func sqliteValueToSQL(v any) string {
	if v == nil {
		return "NULL"
	}
	switch t := v.(type) {
	case int64:
		return fmt.Sprintf("%d", t)
	case float64:
		return fmt.Sprintf("%g", t)
	case bool:
		if t {
			return "1"
		}
		return "0"
	case []byte:
		return sqliteQuoteString(string(t))
	default:
		return sqliteQuoteString(fmt.Sprintf("%v", t))
	}
}

// sqliteQuoteString escapes a string as a SQL single-quoted literal.
func sqliteQuoteString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// queryAndFormat runs a SELECT-like statement and formats the results.
func queryAndFormat(ctx context.Context, db *sql.DB, stmt string, opts *sqliteOpts, out io.Writer) error {
	rows, err := db.QueryContext(ctx, stmt)
	if err != nil {
		return err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return err
	}

	switch opts.mode {
	case modeCSV:
		return formatCSV(rows, cols, opts, out)
	case modeJSON:
		return formatJSON(rows, cols, opts, out)
	case modeLine:
		return formatLine(rows, cols, opts, out)
	case modeColumn:
		return formatColumn(rows, cols, opts, out)
	case modeTable:
		return formatTable(rows, cols, opts, out)
	default:
		return formatList(rows, cols, opts, out)
	}
}

// ---- format helpers -------------------------------------------------------

// scanRow scans one row into a slice of string values.
func scanRow(rows *sql.Rows, cols []string, nullVal string) ([]string, error) {
	raw := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range raw {
		ptrs[i] = &raw[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	out := make([]string, len(cols))
	for i, v := range raw {
		out[i] = anyToString(v, nullVal)
	}
	return out, nil
}

// anyToString converts a scanned database value to its display string.
func anyToString(v any, nullVal string) string {
	if v == nil {
		return nullVal
	}
	switch t := v.(type) {
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}

func formatList(rows *sql.Rows, cols []string, opts *sqliteOpts, out io.Writer) error {
	if opts.header {
		fmt.Fprintln(out, strings.Join(cols, opts.sep))
	}
	for rows.Next() {
		vals, err := scanRow(rows, cols, opts.nullValue)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, strings.Join(vals, opts.sep))
	}
	return rows.Err()
}

func formatCSV(rows *sql.Rows, cols []string, opts *sqliteOpts, out io.Writer) error {
	w := csv.NewWriter(out)
	if opts.header {
		if err := w.Write(cols); err != nil {
			return err
		}
	}
	for rows.Next() {
		vals, err := scanRow(rows, cols, opts.nullValue)
		if err != nil {
			return err
		}
		if err := w.Write(vals); err != nil {
			return err
		}
	}
	w.Flush()
	return rows.Err()
}

func formatJSON(rows *sql.Rows, cols []string, _ *sqliteOpts, out io.Writer) error {
	var result []map[string]any
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			v := raw[i]
			switch t := v.(type) {
			case nil:
				row[col] = nil
			case int64:
				row[col] = t
			case float64:
				row[col] = t
			case bool:
				row[col] = t
			case []byte:
				row[col] = string(t)
			default:
				row[col] = fmt.Sprint(t)
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if result == nil {
		result = []map[string]any{}
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(b))
	return nil
}

func formatLine(rows *sql.Rows, cols []string, opts *sqliteOpts, out io.Writer) error {
	first := true
	for rows.Next() {
		vals, err := scanRow(rows, cols, opts.nullValue)
		if err != nil {
			return err
		}
		if !first {
			fmt.Fprintln(out)
		}
		first = false
		for i, col := range cols {
			fmt.Fprintf(out, "%s = %s\n", col, vals[i])
		}
	}
	return rows.Err()
}

func formatColumn(rows *sql.Rows, cols []string, opts *sqliteOpts, out io.Writer) error {
	// collect all rows first to compute widths
	var allRows [][]string
	for rows.Next() {
		vals, err := scanRow(rows, cols, opts.nullValue)
		if err != nil {
			return err
		}
		allRows = append(allRows, vals)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = utf8.RuneCountInString(c)
	}
	for _, row := range allRows {
		for i, v := range row {
			if l := utf8.RuneCountInString(v); l > widths[i] {
				widths[i] = l
			}
		}
	}

	printRow := func(vals []string) {
		for i, v := range vals {
			if i > 0 {
				fmt.Fprint(out, "  ")
			}
			fmt.Fprintf(out, "%-*s", widths[i], v)
		}
		fmt.Fprintln(out)
	}

	if opts.header {
		printRow(cols)
		underline := make([]string, len(cols))
		for i, w := range widths {
			underline[i] = strings.Repeat("-", w)
		}
		printRow(underline)
	}
	for _, row := range allRows {
		printRow(row)
	}
	return nil
}

func formatTable(rows *sql.Rows, cols []string, opts *sqliteOpts, out io.Writer) error {
	var allRows [][]string
	for rows.Next() {
		vals, err := scanRow(rows, cols, opts.nullValue)
		if err != nil {
			return err
		}
		allRows = append(allRows, vals)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = utf8.RuneCountInString(c)
	}
	for _, row := range allRows {
		for i, v := range row {
			if l := utf8.RuneCountInString(v); l > widths[i] {
				widths[i] = l
			}
		}
	}

	// box-drawing helpers
	top := func() string {
		var b strings.Builder
		b.WriteString("┌")
		for i, w := range widths {
			b.WriteString(strings.Repeat("─", w+2))
			if i < len(widths)-1 {
				b.WriteString("┬")
			}
		}
		b.WriteString("┐")
		return b.String()
	}
	mid := func() string {
		var b strings.Builder
		b.WriteString("├")
		for i, w := range widths {
			b.WriteString(strings.Repeat("─", w+2))
			if i < len(widths)-1 {
				b.WriteString("┼")
			}
		}
		b.WriteString("┤")
		return b.String()
	}
	bot := func() string {
		var b strings.Builder
		b.WriteString("└")
		for i, w := range widths {
			b.WriteString(strings.Repeat("─", w+2))
			if i < len(widths)-1 {
				b.WriteString("┴")
			}
		}
		b.WriteString("┘")
		return b.String()
	}
	row := func(vals []string) string {
		var b strings.Builder
		b.WriteString("│")
		for i, v := range vals {
			fmt.Fprintf(&b, " %-*s │", widths[i], v)
		}
		return b.String()
	}

	fmt.Fprintln(out, top())
	fmt.Fprintln(out, row(cols))
	if len(allRows) > 0 {
		fmt.Fprintln(out, mid())
		for _, r := range allRows {
			fmt.Fprintln(out, row(r))
		}
	}
	fmt.Fprintln(out, bot())
	return nil
}
