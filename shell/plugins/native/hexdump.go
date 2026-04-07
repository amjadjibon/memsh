package native

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"unicode"

	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/shell/plugins"
)

// ── xxd ─────────────────────────────────────────────────────────────────────

// XxdPlugin produces or reverses a hex dump in xxd style.
//
//	xxd [file]            hex dump with offsets + ASCII sidebar
//	xxd -p [file]         plain continuous hex
//	xxd -r [file]         reverse: hex text → binary
//	xxd -b [file]         binary (bit) dump
//	xxd -u [file]         uppercase hex
//	xxd -c N [file]       N bytes per row (default 16)
//	xxd -l N [file]       stop after N bytes
//	xxd -s N [file]       skip N bytes
//	echo "data" | xxd
type XxdPlugin struct{}

func (XxdPlugin) Name() string        { return "xxd" }
func (XxdPlugin) Description() string { return "hex dump or reverse hex dump" }
func (XxdPlugin) Usage() string       { return "xxd [-p] [-r] [-b] [-u] [-c cols] [-l len] [-s seek] [file]" }

func (XxdPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	plain := false
	reverse := false
	binary := false
	upper := false
	cols := 16
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
		case "-p", "--plain":
			plain = true
		case "-r", "--reverse":
			reverse = true
		case "-b", "--bits":
			binary = true
		case "-u", "--uppercase":
			upper = true
		case "-c", "--cols":
			if i+1 >= len(args) {
				return fmt.Errorf("xxd: option requires an argument -- 'c'")
			}
			i++
			fmt.Sscanf(args[i], "%d", &cols)
		case "-l", "--len":
			if i+1 >= len(args) {
				return fmt.Errorf("xxd: option requires an argument -- 'l'")
			}
			i++
			fmt.Sscanf(args[i], "%d", &limit)
		case "-s", "--seek":
			if i+1 >= len(args) {
				return fmt.Errorf("xxd: option requires an argument -- 's'")
			}
			i++
			fmt.Sscanf(args[i], "%d", &skip)
		default:
			// combined short flags: -pu, -ru, etc.
			unknown := ""
			for _, c := range a[1:] {
				switch c {
				case 'p':
					plain = true
				case 'r':
					reverse = true
				case 'b':
					binary = true
				case 'u':
					upper = true
				default:
					unknown += string(c)
				}
			}
			if unknown != "" {
				return fmt.Errorf("xxd: invalid option -- '%s'", unknown)
			}
		}
	}

	if cols <= 0 {
		cols = 16
	}

	src, err := openInput(hc, sc, file)
	if err != nil {
		return fmt.Errorf("xxd: %w", err)
	}
	defer src.Close()

	data, err := readLimited(src, skip, limit)
	if err != nil {
		return fmt.Errorf("xxd: %w", err)
	}

	switch {
	case reverse:
		return xxdReverse(hc.Stdout, data)
	case plain:
		return xxdPlain(hc.Stdout, data, upper)
	case binary:
		return xxdBinary(hc.Stdout, data, cols)
	default:
		return xxdDump(hc.Stdout, data, cols, upper)
	}
}

// xxdDump writes the standard xxd hex+ASCII layout.
func xxdDump(w io.Writer, data []byte, cols int, upper bool) error {
	fmtByte := "%02x"
	if upper {
		fmtByte = "%02X"
	}
	for offset := 0; offset < len(data); offset += cols {
		end := offset + cols
		if end > len(data) {
			end = len(data)
		}
		row := data[offset:end]

		// address
		fmt.Fprintf(w, "%08x: ", offset)

		// hex pairs, space-separated, split at cols/2
		half := cols / 2
		for i, b := range row {
			fmt.Fprintf(w, fmtByte, b)
			if i == half-1 {
				fmt.Fprint(w, " ")
			} else if i < len(row)-1 {
				fmt.Fprint(w, " ")
			}
		}
		// pad short rows
		for i := len(row); i < cols; i++ {
			fmt.Fprint(w, "   ")
			if i == half-1 {
				fmt.Fprint(w, " ")
			}
		}

		// ASCII sidebar
		fmt.Fprint(w, "  ")
		for _, b := range row {
			if b >= 32 && b < 127 {
				fmt.Fprintf(w, "%c", b)
			} else {
				fmt.Fprint(w, ".")
			}
		}
		fmt.Fprintln(w)
	}
	return nil
}

// xxdPlain writes hex bytes as a continuous lowercase/uppercase stream, 60 chars per line.
func xxdPlain(w io.Writer, data []byte, upper bool) error {
	h := hex.EncodeToString(data)
	if upper {
		h = strings.ToUpper(h)
	}
	const lineLen = 60
	for i := 0; i < len(h); i += lineLen {
		end := i + lineLen
		if end > len(h) {
			end = len(h)
		}
		fmt.Fprintln(w, h[i:end])
	}
	return nil
}

// xxdBinary writes a bit dump (8 bits per byte, space-separated).
func xxdBinary(w io.Writer, data []byte, cols int) error {
	for offset := 0; offset < len(data); offset += cols {
		end := offset + cols
		if end > len(data) {
			end = len(data)
		}
		row := data[offset:end]

		fmt.Fprintf(w, "%08x: ", offset)
		for i, b := range row {
			fmt.Fprintf(w, "%08b", b)
			if i < len(row)-1 {
				fmt.Fprint(w, " ")
			}
		}
		fmt.Fprint(w, "  ")
		for _, b := range row {
			if b >= 32 && b < 127 {
				fmt.Fprintf(w, "%c", b)
			} else {
				fmt.Fprint(w, ".")
			}
		}
		fmt.Fprintln(w)
	}
	return nil
}

// xxdReverse decodes hex text back to binary.
func xxdReverse(w io.Writer, data []byte) error {
	// Strip xxd annotations: keep only hex chars.
	// Supports both plain hex and annotated "offset: hexbytes  ascii" format.
	var hexChars strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// If the line looks like annotated xxd output, strip the offset and ASCII.
		if idx := strings.Index(line, ": "); idx >= 0 {
			line = line[idx+2:]
			// trim ASCII sidebar after double-space
			if sp := strings.Index(line, "  "); sp >= 0 {
				line = line[:sp]
			}
		}
		for _, c := range line {
			if unicode.Is(unicode.ASCII_Hex_Digit, c) {
				hexChars.WriteRune(c)
			}
		}
	}
	decoded, err := hex.DecodeString(hexChars.String())
	if err != nil {
		return fmt.Errorf("xxd: reverse: %w", err)
	}
	_, err = w.Write(decoded)
	return err
}

// ensure XxdPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = XxdPlugin{}

// ── hexdump ──────────────────────────────────────────────────────────────────

// HexdumpPlugin produces BSD-style hex dumps.
//
//	hexdump [file]        two-byte hex words
//	hexdump -C [file]     canonical: offsets + hex bytes + ASCII (like xxd)
//	hexdump -n N [file]   limit to N bytes
//	hexdump -s N [file]   skip N bytes
//	hexdump -v [file]     don't collapse duplicate rows with *
type HexdumpPlugin struct{}

func (HexdumpPlugin) Name() string        { return "hexdump" }
func (HexdumpPlugin) Description() string { return "display file contents in hexadecimal" }
func (HexdumpPlugin) Usage() string       { return "hexdump [-C] [-n len] [-s skip] [-v] [file]" }

func (HexdumpPlugin) Run(ctx context.Context, args []string) error {
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

// hexdumpCanonical writes canonical (-C) output: offset | hex bytes | ASCII.
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

// hexdumpTwoWord writes the default hexdump layout: two-byte little-endian words.
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

// ensure HexdumpPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = HexdumpPlugin{}

// ── shared helpers ───────────────────────────────────────────────────────────

type readCloser struct {
	io.Reader
	close func()
}

func (r readCloser) Close() error { r.close(); return nil }

// openInput returns the named virtual-FS file or stdin as a ReadCloser.
func openInput(hc interp.HandlerContext, sc plugins.ShellContext, file string) (io.ReadCloser, error) {
	if file == "" || file == "-" {
		return readCloser{hc.Stdin, func() {}}, nil
	}
	f, err := sc.FS.Open(sc.ResolvePath(file))
	if err != nil {
		return nil, err
	}
	return f, nil
}

// readLimited reads from r, skipping skip bytes and returning at most limit
// bytes (-1 = unlimited).
func readLimited(r io.Reader, skip, limit int) ([]byte, error) {
	if skip > 0 {
		if _, err := io.CopyN(io.Discard, r, int64(skip)); err != nil && err != io.EOF {
			return nil, err
		}
	}
	if limit >= 0 {
		return io.ReadAll(io.LimitReader(r, int64(limit)))
	}
	return io.ReadAll(r)
}
