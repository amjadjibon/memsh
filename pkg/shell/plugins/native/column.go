package native

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

// ColumnPlugin formats input into aligned columns.
//
//	column -t              table mode: align whitespace-delimited fields
//	column -t -s ':'       use ':' as input separator
//	column -t -o ' | '     use ' | ' as output separator
//	column -t -n           don't merge adjacent delimiters (treat empty fields as real)
//	column -c 80           fill-column mode: wrap items into N-wide display
//	column -x              fill rows before columns (row-major order)
//	cat /etc/passwd | column -t -s ':'
type ColumnPlugin struct{}

func (ColumnPlugin) Name() string        { return "column" }
func (ColumnPlugin) Description() string { return "format input into aligned columns" }
func (ColumnPlugin) Usage() string {
	return "column [-t] [-s sep] [-o sep] [-n] [-x] [-c width] [file...]"
}

func (ColumnPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	tableMode := false
	inputSep := ""    // default: any whitespace
	outputSep := "  " // default output column separator
	noMerge := false  // -n: don't merge adjacent delimiters
	rowFirst := false // -x: fill rows before columns
	displayWidth := 80
	var files []string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			files = append(files, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		switch a {
		case "--table":
			tableMode = true
			continue
		case "--separator":
			if i+1 >= len(args) {
				return fmt.Errorf("column: option requires an argument -- 's'")
			}
			i++
			inputSep = args[i]
			continue
		case "--output-separator":
			if i+1 >= len(args) {
				return fmt.Errorf("column: option requires an argument -- 'o'")
			}
			i++
			outputSep = args[i]
			continue
		case "--columns":
			if i+1 >= len(args) {
				return fmt.Errorf("column: option requires an argument -- 'c'")
			}
			i++
			fmt.Sscanf(args[i], "%d", &displayWidth)
			continue
		case "--no-mergedelimiters":
			noMerge = true
			continue
		case "--fillrows":
			rowFirst = true
			continue
		}

		// combined short flags
		unknown := ""
		for j := 1; j < len(a); j++ {
			c := a[j]
			switch c {
			case 't':
				tableMode = true
			case 'n':
				noMerge = true
			case 'x':
				rowFirst = true
			case 's':
				if j+1 < len(a) {
					inputSep = a[j+1:]
					j = len(a) // consume rest
				} else if i+1 < len(args) {
					i++
					inputSep = args[i]
				} else {
					return fmt.Errorf("column: option requires an argument -- 's'")
				}
			case 'o':
				if j+1 < len(a) {
					outputSep = a[j+1:]
					j = len(a)
				} else if i+1 < len(args) {
					i++
					outputSep = args[i]
				} else {
					return fmt.Errorf("column: option requires an argument -- 'o'")
				}
			case 'c':
				if j+1 < len(a) {
					fmt.Sscanf(a[j+1:], "%d", &displayWidth)
					j = len(a)
				} else if i+1 < len(args) {
					i++
					fmt.Sscanf(args[i], "%d", &displayWidth)
				} else {
					return fmt.Errorf("column: option requires an argument -- 'c'")
				}
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("column: invalid option -- '%s'", unknown)
		}
	}

	// read all input
	lines, err := columnReadLines(hc, sc, files)
	if err != nil {
		return err
	}

	if tableMode {
		return columnTable(hc.Stdout, lines, inputSep, outputSep, noMerge)
	}
	return columnFill(hc.Stdout, lines, displayWidth, rowFirst)
}

// columnReadLines collects non-empty lines from files or stdin.
func columnReadLines(hc interp.HandlerContext, sc plugins.ShellContext, files []string) ([]string, error) {
	var lines []string
	scan := func(r io.Reader) error {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		return scanner.Err()
	}

	if len(files) == 0 {
		return lines, scan(hc.Stdin)
	}
	for _, f := range files {
		fh, err := sc.FS.Open(sc.ResolvePath(f))
		if err != nil {
			return nil, fmt.Errorf("column: %s: %w", f, err)
		}
		err = scan(fh)
		fh.Close()
		if err != nil {
			return nil, err
		}
	}
	return lines, nil
}

// columnTable aligns fields into a table with padded columns.
func columnTable(w io.Writer, lines []string, inputSep, outputSep string, noMerge bool) error {
	if len(lines) == 0 {
		return nil
	}

	split := func(line string) []string {
		if inputSep == "" {
			if noMerge {
				return strings.Split(line, "\t")
			}
			return strings.Fields(line)
		}
		if noMerge {
			return strings.Split(line, inputSep)
		}
		// merge consecutive separators (default)
		fields := strings.Split(line, inputSep)
		var out []string
		for _, f := range fields {
			if f != "" {
				out = append(out, f)
			}
		}
		return out
	}

	// parse all rows
	rows := make([][]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			rows = append(rows, nil) // preserve blank lines
			continue
		}
		rows = append(rows, split(l))
	}

	// compute max width per column
	numCols := 0
	for _, r := range rows {
		if len(r) > numCols {
			numCols = len(r)
		}
	}
	widths := make([]int, numCols)
	for _, r := range rows {
		for i, f := range r {
			if len(f) > widths[i] {
				widths[i] = len(f)
			}
		}
	}

	// print
	for _, r := range rows {
		if r == nil {
			fmt.Fprintln(w)
			continue
		}
		var sb strings.Builder
		for i, f := range r {
			if i > 0 {
				sb.WriteString(outputSep)
			}
			// last column: no padding
			if i == len(r)-1 {
				sb.WriteString(f)
			} else {
				sb.WriteString(f)
				sb.WriteString(strings.Repeat(" ", widths[i]-len(f)))
			}
		}
		fmt.Fprintln(w, sb.String())
	}
	return nil
}

// columnFill wraps items across columns to fit within displayWidth.
// rowFirst=false (default): fill down columns then across.
// rowFirst=true  (-x):      fill across rows then down.
func columnFill(w io.Writer, lines []string, displayWidth int, rowFirst bool) error {
	// collect non-empty items
	var items []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			items = append(items, strings.TrimSpace(l))
		}
	}
	if len(items) == 0 {
		return nil
	}

	// find the widest item
	maxW := 0
	for _, item := range items {
		if len(item) > maxW {
			maxW = len(item)
		}
	}

	colWidth := maxW + 2 // pad with 2 spaces between columns
	numCols := displayWidth / colWidth
	if numCols < 1 {
		numCols = 1
	}
	numRows := (len(items) + numCols - 1) / numCols

	getItem := func(row, col int) string {
		var idx int
		if rowFirst {
			idx = row*numCols + col
		} else {
			idx = col*numRows + row
		}
		if idx >= len(items) {
			return ""
		}
		return items[idx]
	}

	for row := 0; row < numRows; row++ {
		var sb strings.Builder
		for col := 0; col < numCols; col++ {
			item := getItem(row, col)
			if item == "" {
				continue
			}
			if col == numCols-1 || getItem(row, col+1) == "" {
				sb.WriteString(item)
			} else {
				sb.WriteString(item)
				sb.WriteString(strings.Repeat(" ", colWidth-len(item)))
			}
		}
		fmt.Fprintln(w, sb.String())
	}
	return nil
}

// ensure ColumnPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = ColumnPlugin{}
