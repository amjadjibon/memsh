package native

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/amjadjibon/memsh/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type XxdPlugin struct{}

func (XxdPlugin) Name() string                                 { return "xxd" }
func (XxdPlugin) Description() string                          { return "hex dump or reverse hex dump" }
func (XxdPlugin) Usage() string                                { return "xxd [-p] [-r] [-b] [-u] [-c cols] [-l len] [-s seek] [file]" }
func (XxdPlugin) Run(ctx context.Context, args []string) error { return runXxd(ctx, args) }

func runXxd(ctx context.Context, args []string) error {
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

type readCloser struct {
	io.Reader
	close func()
}

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

func xxdPlain(w io.Writer, data []byte, upper bool) error {
	h := hex.EncodeToString(data)
	if upper {
		h = strings.ToUpper(h)
	}
	const lineLen = 60
	for i := 0; i < len(h); i += lineLen {
		end := min(i+lineLen, len(h))
		fmt.Fprintln(w, h[i:end])
	}
	return nil
}

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

func (r readCloser) Close() error { r.close(); return nil }

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
