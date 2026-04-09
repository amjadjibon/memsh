package native

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/spf13/afero"
	"golang.org/x/term"
	"mvdan.cc/sh/v3/interp"
)

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (cr *contextReader) Read(p []byte) (int, error) {
	select {
	case <-cr.ctx.Done():
		return 0, cr.ctx.Err()
	default:
		return cr.r.Read(p)
	}
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	isTerminal := false
	if f, ok := src.(*os.File); ok {
		isTerminal = term.IsTerminal(int(f.Fd()))
	}

	if !isTerminal {
		buf := make([]byte, 32*1024)
		var written int64
		for {
			select {
			case <-ctx.Done():
				return written, ctx.Err()
			default:
			}

			nr, err := src.Read(buf)
			if nr > 0 {
				nw, errw := dst.Write(buf[:nr])
				if nw > 0 {
					written += int64(nw)
				}
				if errw != nil {
					return written, errw
				}
				if nr != nw {
					return written, io.ErrShortWrite
				}
			}
			if err != nil {
				if err == io.EOF {
					return written, nil
				}
				return written, err
			}
		}
	}

	type result struct {
		n   int64
		err error
	}

	resCh := make(chan result, 1)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)

	go func() {
		n, err := io.Copy(dst, src)
		resCh <- result{n: n, err: err}
	}()

	select {
	case res := <-resCh:
		signal.Stop(sigCh)
		return res.n, res.err
	case <-sigCh:
		signal.Stop(sigCh)
		return 0, nil
	case <-ctx.Done():
		signal.Stop(sigCh)
		return 0, ctx.Err()
	}
}

func scanWithContext(ctx context.Context, r io.Reader) *bufio.Scanner {
	return bufio.NewScanner(&contextReader{ctx: ctx, r: r})
}

func readLines(ctx context.Context, hc interp.HandlerContext, sc plugins.ShellContext, files []string) ([]string, error) {
	var lines []string
	if len(files) == 0 {
		scanner := scanWithContext(ctx, hc.Stdin)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		return lines, scanner.Err()
	}

	for _, f := range files {
		r, err := sc.FS.Open(sc.ResolvePath(f))
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		r.Close()
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	return lines, nil
}

func headLines(r io.Reader, w io.Writer, n int) error {
	scanner := bufio.NewScanner(r)
	for i := 0; i < n && scanner.Scan(); i++ {
		fmt.Fprintln(w, scanner.Text())
	}
	return scanner.Err()
}

func tailLines(r io.Reader, w io.Writer, n int) error {
	scanner := bufio.NewScanner(r)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	start := len(lines) - n
	if start < 0 {
		start = 0
	}
	for _, line := range lines[start:] {
		fmt.Fprintln(w, line)
	}
	return nil
}

func parseRangeList(spec string, max int) ([]int, error) {
	seen := map[int]bool{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if idx := strings.Index(part, "-"); idx >= 0 {
			startStr := part[:idx]
			endStr := part[idx+1:]
			start := 1
			end := max
			if startStr != "" {
				v, err := strconv.Atoi(startStr)
				if err != nil || v < 1 {
					return nil, fmt.Errorf("invalid range %q", part)
				}
				start = v
			}
			if endStr != "" {
				v, err := strconv.Atoi(endStr)
				if err != nil || v < 1 {
					return nil, fmt.Errorf("invalid range %q", part)
				}
				end = v
			}
			for i := start; i <= end; i++ {
				seen[i-1] = true
			}
			continue
		}

		v, err := strconv.Atoi(part)
		if err != nil || v < 1 {
			return nil, fmt.Errorf("invalid field %q", part)
		}
		seen[v-1] = true
	}

	result := make([]int, 0, len(seen))
	for i := range seen {
		result = append(result, i)
	}
	slices.Sort(result)
	return result, nil
}

func expandTrSet(s string) []rune {
	s = strings.ReplaceAll(s, "[:alpha:]", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	s = strings.ReplaceAll(s, "[:digit:]", "0123456789")
	s = strings.ReplaceAll(s, "[:lower:]", "abcdefghijklmnopqrstuvwxyz")
	s = strings.ReplaceAll(s, "[:upper:]", "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	s = strings.ReplaceAll(s, "[:space:]", " \t\n\r\f\v")
	s = strings.ReplaceAll(s, "[:alnum:]", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
	s = strings.ReplaceAll(s, "[:punct:]", "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~")

	runes := []rune(s)
	var out []rune
	for i := 0; i < len(runes); i++ {
		if i+2 < len(runes) && runes[i+1] == '-' {
			for c := runes[i]; c <= runes[i+2]; c++ {
				out = append(out, c)
			}
			i += 2
		} else {
			out = append(out, runes[i])
		}
	}
	return out
}

func runeIndex(set []rune, r rune) int {
	for i, c := range set {
		if c == r {
			return i
		}
	}
	return -1
}

func sortInts(values []int) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func readAllVirtual(sc plugins.ShellContext, path string) ([]byte, error) {
	return afero.ReadFile(sc.FS, sc.ResolvePath(path))
}

func parseDuration(s string) (time.Duration, error) {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(f * float64(time.Second)), nil
	}
	return time.ParseDuration(s)
}
