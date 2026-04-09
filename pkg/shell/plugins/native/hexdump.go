package native

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type HexdumpPlugin struct{}

func (HexdumpPlugin) Name() string                                 { return "hexdump" }
func (HexdumpPlugin) Description() string                          { return "display file contents in hexadecimal" }
func (HexdumpPlugin) Usage() string                                { return "hexdump [-C] [-n len] [-s skip] [-v] [file]" }
func (HexdumpPlugin) Run(ctx context.Context, args []string) error { return runHexdump(ctx, args) }

func runHexdump(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	canonical := false
	noCollapse := false
	limit := -1
	skip := 0
	var file string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			file = a
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		switch a {
		case "--canonical":
			canonical = true
		case "-n":
			if i+1 >= len(args) {
				return fmt.Errorf("hexdump: option requires an argument -- 'n'")
			}
			i++
			fmt.Sscanf(args[i], "%d", &limit)
		case "-s":
			if i+1 >= len(args) {
				return fmt.Errorf("hexdump: option requires an argument -- 's'")
			}
			i++
			fmt.Sscanf(args[i], "%d", &skip)
		default:
			unknown := ""
			for _, c := range a[1:] {
				switch c {
				case 'C':
					canonical = true
				case 'v':
					noCollapse = true
				default:
					unknown += string(c)
				}
			}
			if unknown != "" {
				return fmt.Errorf("hexdump: invalid option -- '%s'", unknown)
			}
		}
	}

	src, err := openInput(hc, sc, file)
	if err != nil {
		return fmt.Errorf("hexdump: %w", err)
	}
	defer src.Close()

	data, err := readLimited(src, skip, limit)
	if err != nil {
		return fmt.Errorf("hexdump: %w", err)
	}

	if canonical {
		return hexdumpCanonical(hc.Stdout, data, noCollapse)
	}
	return hexdumpTwoWord(hc.Stdout, data, noCollapse)
}

func hexdumpCanonical(w io.Writer, data []byte, noCollapse bool) error {
	const cols = 16
	var lastRow []byte
	collapsed := false

	for offset := 0; offset < len(data); offset += cols {
		end := offset + cols
		if end > len(data) {
			end = len(data)
		}
		row := data[offset:end]

		if !noCollapse && offset > 0 && len(row) == cols && bytes.Equal(row, lastRow) {
			if !collapsed {
				fmt.Fprintln(w, "*")
				collapsed = true
			}
			continue
		}
		collapsed = false
		lastRow = make([]byte, len(row))
		copy(lastRow, row)

		fmt.Fprintf(w, "%08x  ", offset)

		for i, b := range row {
			fmt.Fprintf(w, "%02x ", b)
			if i == 7 {
				fmt.Fprint(w, " ")
			}
		}
		// pad
		for i := len(row); i < cols; i++ {
			fmt.Fprint(w, "   ")
			if i == 7 {
				fmt.Fprint(w, " ")
			}
		}

		fmt.Fprint(w, " |")
		for _, b := range row {
			if b >= 32 && b < 127 {
				fmt.Fprintf(w, "%c", b)
			} else {
				fmt.Fprint(w, ".")
			}
		}
		fmt.Fprintln(w, "|")
	}
	fmt.Fprintf(w, "%08x\n", len(data))
	return nil
}

func hexdumpTwoWord(w io.Writer, data []byte, noCollapse bool) error {
	const cols = 16
	var lastRow []byte
	collapsed := false

	for offset := 0; offset < len(data); offset += cols {
		end := offset + cols
		if end > len(data) {
			end = len(data)
		}
		row := data[offset:end]

		if !noCollapse && offset > 0 && len(row) == cols && bytes.Equal(row, lastRow) {
			if !collapsed {
				fmt.Fprintln(w, "*")
				collapsed = true
			}
			continue
		}
		collapsed = false
		lastRow = make([]byte, len(row))
		copy(lastRow, row)

		fmt.Fprintf(w, "%07x ", offset)
		for i := 0; i < cols; i += 2 {
			if i+1 < len(row) {
				// little-endian two-byte word
				fmt.Fprintf(w, "%02x%02x ", row[i+1], row[i])
			} else if i < len(row) {
				fmt.Fprintf(w, "  %02x ", row[i])
			} else {
				fmt.Fprint(w, "     ")
			}
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "%07x\n", len(data))
	return nil
}
