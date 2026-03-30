package shell

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/amjadjibon/memsh/shell/plugins"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// ErrExit is returned by the shell when an exit or quit command is executed.
var ErrExit = errors.New("exit")

// execHandler returns a middleware-style exec handler that intercepts known
// built-in commands and delegates everything else to next.
func (s *Shell) execHandler(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		if len(args) == 0 {
			return nil
		}

		// Inject shell-level state so Plugin.Run implementations can access
		// the virtual FS, cwd, and env without needing a reference to Shell.
		ctx = plugins.WithShellContext(ctx, plugins.ShellContext{
			FS:          s.fs,
			Cwd:         s.cwd,
			Env:         func(key string) string { return s.env[key] },
			ResolvePath: s.resolvePath,
		})

		switch args[0] {
		case "exit", "quit":
			return ErrExit
		case "cd":
			return s.builtinCd(ctx, args)
		case "pwd":
			return s.builtinPwd(ctx, args)
		case "mkdir":
			return s.builtinMkdir(ctx, args)
		case "rm":
			return s.builtinRm(ctx, args)
		case "touch":
			return s.builtinTouch(ctx, args)
		case "ls":
			return s.builtinLs(ctx, args)
		case "cat":
			return s.builtinCat(ctx, args)
		case "echo":
			return s.builtinEcho(ctx, args)
		case "tee":
			return s.builtinTee(ctx, args)
		case "cp":
			return s.builtinCp(ctx, args)
		case "mv":
			return s.builtinMv(ctx, args)
		case "head":
			return s.builtinHead(ctx, args)
		case "tail":
			return s.builtinTail(ctx, args)
		case "sort":
			return s.builtinSort(ctx, args)
		case "uniq":
			return s.builtinUniq(ctx, args)
		case "cut":
			return s.builtinCut(ctx, args)
		case "tr":
			return s.builtinTr(ctx, args)
		case "chmod":
			return s.builtinChmod(ctx, args)
		case "diff":
			return s.builtinDiff(ctx, args)
		case "stat":
			return s.builtinStat(ctx, args)
		case "man":
			return s.builtinHelp(ctx, args)
		default:
			if fn, ok := s.builtins[args[0]]; ok {
				return fn(ctx, args)
			}
			if _, ok := s.plugins[args[0]]; ok {
				return s.runPlugin(ctx, args[0], args)
			}
			return next(ctx, args)
		}
	}
}

func (s *Shell) builtinCd(_ context.Context, args []string) error {
	if len(args) > 2 {
		return fmt.Errorf("cd: too many arguments")
	}

	dir := "/"
	if len(args) == 2 {
		dir = args[1]
	}

	target := s.resolvePath(dir)

	info, err := s.fs.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cd: %s: No such file or directory", dir)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("cd: %s: Not a directory", dir)
	}

	s.cwd = target
	return nil
}

func (s *Shell) builtinPwd(ctx context.Context, _ []string) error {
	hc := interp.HandlerCtx(ctx)
	_, err := fmt.Fprintln(hc.Stdout, s.cwd)
	return err
}

func (s *Shell) builtinMkdir(_ context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("mkdir: missing operand")
	}
	for _, dir := range args[1:] {
		if err := s.fs.MkdirAll(s.resolvePath(dir), 0755); err != nil {
			return fmt.Errorf("mkdir: cannot create directory '%s': %w", dir, err)
		}
	}
	return nil
}

func (s *Shell) builtinRm(_ context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("rm: missing operand")
	}
	for _, target := range args[1:] {
		if err := s.fs.RemoveAll(s.resolvePath(target)); err != nil {
			return fmt.Errorf("rm: cannot remove '%s': %w", target, err)
		}
	}
	return nil
}

func (s *Shell) builtinTouch(_ context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("touch: missing file operand")
	}
	for _, target := range args[1:] {
		absPath := s.resolvePath(target)
		now := time.Now()
		if err := s.fs.Chtimes(absPath, now, now); err != nil {
			if os.IsNotExist(err) {
				f, err := s.fs.Create(absPath)
				if err != nil {
					return fmt.Errorf("touch: cannot touch '%s': %w", target, err)
				}
				f.Close()
			} else {
				return fmt.Errorf("touch: cannot touch '%s': %w", target, err)
			}
		}
	}
	return nil
}

func (s *Shell) builtinLs(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	target := s.cwd
	if len(args) > 1 {
		target = s.resolvePath(args[1])
	}

	f, err := s.fs.Open(target)
	if err != nil {
		return fmt.Errorf("ls: cannot access '%s': %w", target, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	if !info.IsDir() {
		fmt.Fprintln(hc.Stdout, filepath.Base(target))
		return nil
	}

	names, err := f.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		fmt.Fprintln(hc.Stdout, name)
	}
	return nil
}

func (s *Shell) builtinCat(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	if len(args) < 2 {
		return fmt.Errorf("cat: missing operand")
	}
	for _, target := range args[1:] {
		absPath := s.resolvePath(target)
		f, err := s.fs.Open(absPath)
		if err != nil {
			return fmt.Errorf("cat: %s: No such file or directory", target)
		}
		io.Copy(hc.Stdout, f)
		f.Close()
	}
	return nil
}

func (s *Shell) builtinEcho(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	for i, arg := range args[1:] {
		if i > 0 {
			fmt.Fprint(hc.Stdout, " ")
		}
		fmt.Fprint(hc.Stdout, arg)
	}
	fmt.Fprintln(hc.Stdout)
	return nil
}

func (s *Shell) builtinCp(_ context.Context, args []string) error {
	recursive := false
	var positional []string
	for _, a := range args[1:] {
		switch a {
		case "-r", "-R", "--recursive":
			recursive = true
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) < 2 {
		return fmt.Errorf("cp: missing destination file operand")
	}

	src := s.resolvePath(positional[0])
	dst := s.resolvePath(positional[1])

	srcInfo, err := s.fs.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cp: cannot stat '%s': No such file or directory", positional[0])
		}
		return fmt.Errorf("cp: %w", err)
	}

	if srcInfo.IsDir() {
		if !recursive {
			return fmt.Errorf("cp: -r not specified; omitting directory '%s'", positional[0])
		}
		dstInfo, err := s.fs.Stat(dst)
		if err == nil && dstInfo.IsDir() {
			dst = filepath.Join(dst, filepath.Base(src))
		}
		return s.cpDir(src, dst)
	}

	dstInfo, err := s.fs.Stat(dst)
	if err == nil && dstInfo.IsDir() {
		dst = filepath.Join(dst, filepath.Base(src))
	}
	return s.cpFile(src, dst)
}

func (s *Shell) cpFile(src, dst string) error {
	in, err := s.fs.Open(src)
	if err != nil {
		return fmt.Errorf("cp: %w", err)
	}
	defer in.Close()

	out, err := s.fs.Create(dst)
	if err != nil {
		return fmt.Errorf("cp: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func (s *Shell) cpDir(src, dst string) error {
	return afero.Walk(s.fs, src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return s.fs.MkdirAll(target, 0755)
		}
		return s.cpFile(path, target)
	})
}

func (s *Shell) builtinMv(_ context.Context, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("mv: missing destination file operand")
	}

	src := s.resolvePath(args[1])
	dst := s.resolvePath(args[2])

	dstInfo, err := s.fs.Stat(dst)
	if err == nil && dstInfo.IsDir() {
		dst = filepath.Join(dst, filepath.Base(src))
	}

	if err := s.fs.Rename(src, dst); err != nil {
		return fmt.Errorf("mv: cannot move '%s' to '%s': %w", args[1], args[2], err)
	}
	return nil
}

func (s *Shell) builtinHead(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	n := 10
	var files []string

	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "-n" {
			if i+1 >= len(args) {
				return fmt.Errorf("head: option requires an argument -- 'n'")
			}
			i++
			v, err := strconv.Atoi(args[i])
			if err != nil || v < 0 {
				return fmt.Errorf("head: invalid number of lines: '%s'", args[i])
			}
			n = v
		} else if strings.HasPrefix(a, "-n") {
			v, err := strconv.Atoi(a[2:])
			if err != nil || v < 0 {
				return fmt.Errorf("head: invalid number of lines: '%s'", a[2:])
			}
			n = v
		} else {
			files = append(files, a)
		}
	}

	if len(files) == 0 {
		return headLines(hc.Stdin, hc.Stdout, n)
	}
	for _, f := range files {
		r, err := s.fs.Open(s.resolvePath(f))
		if err != nil {
			return fmt.Errorf("head: %s: %w", f, err)
		}
		err = headLines(r, hc.Stdout, n)
		r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func headLines(r io.Reader, w io.Writer, n int) error {
	sc := bufio.NewScanner(r)
	for i := 0; i < n && sc.Scan(); i++ {
		fmt.Fprintln(w, sc.Text())
	}
	return sc.Err()
}

func (s *Shell) builtinTail(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	n := 10
	var files []string

	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "-n" {
			if i+1 >= len(args) {
				return fmt.Errorf("tail: option requires an argument -- 'n'")
			}
			i++
			v, err := strconv.Atoi(args[i])
			if err != nil || v < 0 {
				return fmt.Errorf("tail: invalid number of lines: '%s'", args[i])
			}
			n = v
		} else if strings.HasPrefix(a, "-n") {
			v, err := strconv.Atoi(a[2:])
			if err != nil || v < 0 {
				return fmt.Errorf("tail: invalid number of lines: '%s'", a[2:])
			}
			n = v
		} else {
			files = append(files, a)
		}
	}

	if len(files) == 0 {
		return tailLines(hc.Stdin, hc.Stdout, n)
	}
	for _, f := range files {
		r, err := s.fs.Open(s.resolvePath(f))
		if err != nil {
			return fmt.Errorf("tail: %s: %w", f, err)
		}
		err = tailLines(r, hc.Stdout, n)
		r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func tailLines(r io.Reader, w io.Writer, n int) error {
	sc := bufio.NewScanner(r)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
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

// builtinTee reads stdin and writes to stdout AND each named virtual FS file.
// -a appends instead of overwriting.
func (s *Shell) builtinTee(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	appendMode := false
	var targets []string

	for _, a := range args[1:] {
		if a == "-a" {
			appendMode = true
		} else {
			targets = append(targets, a)
		}
	}

	writers := []io.Writer{hc.Stdout}
	var toClose []io.Closer

	for _, t := range targets {
		absPath := s.resolvePath(t)
		var f afero.File
		var err error
		if appendMode {
			f, err = s.fs.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		} else {
			f, err = s.fs.Create(absPath)
		}
		if err != nil {
			return fmt.Errorf("tee: %s: %w", t, err)
		}
		writers = append(writers, f)
		toClose = append(toClose, f)
	}
	defer func() {
		for _, c := range toClose {
			c.Close()
		}
	}()

	_, err := io.Copy(io.MultiWriter(writers...), hc.Stdin)
	return err
}

// ── sort ─────────────────────────────────────────────────────────────────────

func (s *Shell) builtinSort(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	reverse, unique, numeric := false, false, false
	var files []string

	for _, a := range args[1:] {
		switch a {
		case "-r":
			reverse = true
		case "-u":
			unique = true
		case "-n":
			numeric = true
		default:
			files = append(files, a)
		}
	}

	lines, err := readLines(s, hc, files)
	if err != nil {
		return err
	}

	sort.SliceStable(lines, func(i, j int) bool {
		if numeric {
			ni, _ := strconv.Atoi(strings.TrimSpace(lines[i]))
			nj, _ := strconv.Atoi(strings.TrimSpace(lines[j]))
			if reverse {
				return ni > nj
			}
			return ni < nj
		}
		if reverse {
			return lines[i] > lines[j]
		}
		return lines[i] < lines[j]
	})

	if unique {
		deduped := lines[:0]
		for i, l := range lines {
			if i == 0 || l != lines[i-1] {
				deduped = append(deduped, l)
			}
		}
		lines = deduped
	}

	for _, l := range lines {
		fmt.Fprintln(hc.Stdout, l)
	}
	return nil
}

// ── uniq ─────────────────────────────────────────────────────────────────────

func (s *Shell) builtinUniq(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	count, onlyDups, onlyUniq := false, false, false
	var files []string

	for _, a := range args[1:] {
		switch a {
		case "-c":
			count = true
		case "-d":
			onlyDups = true
		case "-u":
			onlyUniq = true
		default:
			files = append(files, a)
		}
	}

	lines, err := readLines(s, hc, files)
	if err != nil {
		return err
	}

	type run struct {
		line string
		n    int
	}
	var runs []run
	for _, l := range lines {
		if len(runs) > 0 && runs[len(runs)-1].line == l {
			runs[len(runs)-1].n++
		} else {
			runs = append(runs, run{l, 1})
		}
	}

	for _, r := range runs {
		isDup := r.n > 1
		if onlyDups && !isDup {
			continue
		}
		if onlyUniq && isDup {
			continue
		}
		if count {
			fmt.Fprintf(hc.Stdout, "%7d %s\n", r.n, r.line)
		} else {
			fmt.Fprintln(hc.Stdout, r.line)
		}
	}
	return nil
}

// ── cut ──────────────────────────────────────────────────────────────────────

func (s *Shell) builtinCut(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	delim := "\t"
	var fieldList, charList string
	var files []string

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-d":
			if i+1 < len(args) {
				i++
				delim = args[i]
			}
		case "-f":
			if i+1 < len(args) {
				i++
				fieldList = args[i]
			}
		case "-c":
			if i+1 < len(args) {
				i++
				charList = args[i]
			}
		default:
			files = append(files, args[i])
		}
	}

	if fieldList == "" && charList == "" {
		return fmt.Errorf("cut: must specify -f or -c")
	}

	lines, err := readLines(s, hc, files)
	if err != nil {
		return err
	}

	for _, line := range lines {
		if charList != "" {
			runes := []rune(line)
			indices, parseErr := parseRangeList(charList, len(runes))
			if parseErr != nil {
				return fmt.Errorf("cut: invalid character range: %v", parseErr)
			}
			var out []rune
			for _, idx := range indices {
				if idx < len(runes) {
					out = append(out, runes[idx])
				}
			}
			fmt.Fprintln(hc.Stdout, string(out))
		} else {
			parts := strings.Split(line, delim)
			indices, parseErr := parseRangeList(fieldList, len(parts))
			if parseErr != nil {
				return fmt.Errorf("cut: invalid field range: %v", parseErr)
			}
			var selected []string
			for _, idx := range indices {
				if idx < len(parts) {
					selected = append(selected, parts[idx])
				}
			}
			fmt.Fprintln(hc.Stdout, strings.Join(selected, delim))
		}
	}
	return nil
}

// parseRangeList parses a cut-style range list like "1,3,5-7,9-" (1-based)
// and returns a sorted de-duplicated slice of 0-based indices up to max.
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
		} else {
			v, err := strconv.Atoi(part)
			if err != nil || v < 1 {
				return nil, fmt.Errorf("invalid field %q", part)
			}
			seen[v-1] = true
		}
	}
	result := make([]int, 0, len(seen))
	for i := range seen {
		result = append(result, i)
	}
	sort.Ints(result)
	return result, nil
}

// ── tr ───────────────────────────────────────────────────────────────────────

func (s *Shell) builtinTr(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	delete, squeeze := false, false
	var positional []string

	for _, a := range args[1:] {
		switch a {
		case "-d":
			delete = true
		case "-s":
			squeeze = true
		default:
			positional = append(positional, a)
		}
	}

	if len(positional) == 0 {
		return fmt.Errorf("tr: missing operand")
	}

	set1 := expandTrSet(positional[0])
	set1Map := make(map[rune]bool, len(set1))
	for _, r := range set1 {
		set1Map[r] = true
	}

	var set2 []rune
	if !delete && len(positional) >= 2 {
		set2 = expandTrSet(positional[1])
	}

	input, err := io.ReadAll(hc.Stdin)
	if err != nil {
		return err
	}

	var sb strings.Builder
	prev := rune(-1)
	for _, r := range string(input) {
		if delete {
			if !set1Map[r] {
				sb.WriteRune(r)
			}
			continue
		}
		if idx := runeIndex(set1, r); idx >= 0 && len(set2) > 0 {
			mapped := set2[min(idx, len(set2)-1)]
			if squeeze && mapped == prev {
				continue
			}
			sb.WriteRune(mapped)
			prev = mapped
		} else if squeeze && set1Map[r] && r == prev {
			// squeeze repeated chars in set1 (no set2 case)
			continue
		} else {
			sb.WriteRune(r)
			prev = r
		}
	}
	_, err = fmt.Fprint(hc.Stdout, sb.String())
	return err
}

func expandTrSet(s string) []rune {
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

// ── chmod ─────────────────────────────────────────────────────────────────────

func (s *Shell) builtinChmod(_ context.Context, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("chmod: missing operand")
	}
	modeStr := args[1]
	v, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		return fmt.Errorf("chmod: invalid mode '%s'", modeStr)
	}
	mode := os.FileMode(v)
	for _, target := range args[2:] {
		if err := s.fs.Chmod(s.resolvePath(target), mode); err != nil {
			return fmt.Errorf("chmod: cannot chmod '%s': %w", target, err)
		}
	}
	return nil
}

// ── diff ──────────────────────────────────────────────────────────────────────

func (s *Shell) builtinDiff(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	if len(args) < 3 {
		return fmt.Errorf("diff: missing operand")
	}

	readFileLines := func(path string) ([]string, error) {
		f, err := s.fs.Open(s.resolvePath(path))
		if err != nil {
			return nil, fmt.Errorf("diff: %s: %w", path, err)
		}
		defer f.Close()
		var lines []string
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		return lines, sc.Err()
	}

	a, err := readFileLines(args[1])
	if err != nil {
		return err
	}
	b, err := readFileLines(args[2])
	if err != nil {
		return err
	}

	// LCS-based diff
	edits := lcsEdits(a, b)
	changed := false
	for _, e := range edits {
		changed = true
		fmt.Fprintln(hc.Stdout, e)
	}
	if changed {
		return interp.ExitStatus(1)
	}
	return nil
}

// lcsEdits returns a slice of "< line" / "> line" diff lines using LCS.
func lcsEdits(a, b []string) []string {
	m, n := len(a), len(b)
	// dp[i][j] = LCS length of a[:i] and b[:j]
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	// Backtrack
	var edits []string
	i, j := m, n
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			i--
			j--
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			edits = append(edits, "> "+b[j-1])
			j--
		default:
			edits = append(edits, "< "+a[i-1])
			i--
		}
	}
	// Reverse
	for l, r := 0, len(edits)-1; l < r; l, r = l+1, r-1 {
		edits[l], edits[r] = edits[r], edits[l]
	}
	return edits
}

// ── stat ──────────────────────────────────────────────────────────────────────

func (s *Shell) builtinStat(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	if len(args) < 2 {
		return fmt.Errorf("stat: missing operand")
	}
	for _, target := range args[1:] {
		info, err := s.fs.Stat(s.resolvePath(target))
		if err != nil {
			return fmt.Errorf("stat: cannot stat '%s': %w", target, err)
		}
		fmt.Fprintf(hc.Stdout, "  File: %s\n", target)
		fmt.Fprintf(hc.Stdout, "  Size: %d\n", info.Size())
		fmt.Fprintf(hc.Stdout, "  Mode: %s\n", info.Mode())
		fmt.Fprintf(hc.Stdout, " IsDir: %v\n", info.IsDir())
		fmt.Fprintf(hc.Stdout, "ModTime: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))
	}
	return nil
}

// ── help / man ────────────────────────────────────────────────────────────────

var builtinHelp = map[string][2]string{
	"cat":   {"concatenate and print files", "cat <file>..."},
	"cd":    {"change working directory", "cd [dir]"},
	"chmod": {"change file permissions", "chmod <octal-mode> <file>..."},
	"cp":    {"copy files or directories", "cp [-r] <src> <dst>"},
	"cut":   {"extract fields or characters", "cut -d <delim> -f <fields> [file]\n       cut -c <chars> [file]"},
	"diff":  {"compare two files line by line", "diff <file1> <file2>"},
	"echo":  {"print arguments", "echo [arg]..."},
	"find":  {"search virtual filesystem", "find [path] [-name <glob>] [-type f|d]"},
	"grep":  {"search file contents for patterns", "grep [-i] [-n] [-v] [-r] <pattern> [file...]"},
	"head":  {"print first lines of a file", "head [-n N] [file]"},
	"ls":    {"list directory contents", "ls [path]"},
	"mkdir": {"create directories", "mkdir <dir>..."},
	"mv":    {"move or rename files", "mv <src> <dst>"},
	"pwd":   {"print working directory", "pwd"},
	"rm":    {"remove files or directories", "rm <path>..."},
	"sort":  {"sort lines of text", "sort [-r] [-u] [-n] [file]"},
	"stat":  {"show file status", "stat <file>..."},
	"tail":  {"print last lines of a file", "tail [-n N] [file]"},
	"tee":   {"read stdin; write to stdout and files", "tee [-a] [file]..."},
	"touch": {"create or update file timestamps", "touch <file>..."},
	"tr":    {"translate or delete characters", "tr [-d] [-s] <set1> [set2]"},
	"uniq":  {"filter adjacent duplicate lines", "uniq [-c] [-d] [-u] [file]"},
	"awk":   {"pattern scanning and processing", "awk '<prog>' [file...]\n      awk -f <progfile> [file...]"},
	"wc":    {"count lines, words, and bytes", "wc [-l] [-w] [-c] [file]"},
	"base64": {"encode or decode base64 data", "base64 [-d] [data...]"},
}

func (s *Shell) builtinHelp(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	if len(args) >= 2 {
		cmd := args[1]
		if info, ok := builtinHelp[cmd]; ok {
			fmt.Fprintf(hc.Stdout, "%s - %s\nUsage: %s\n", cmd, info[0], info[1])
		} else {
			fmt.Fprintf(hc.Stdout, "help: no help entry for '%s'\n", cmd)
		}
		return nil
	}

	// List all commands
	names := make([]string, 0, len(builtinHelp))
	for k := range builtinHelp {
		names = append(names, k)
	}
	sort.Strings(names)
	fmt.Fprintln(hc.Stdout, "Available commands:")
	for _, name := range names {
		fmt.Fprintf(hc.Stdout, "  %-10s  %s\n", name, builtinHelp[name][0])
	}
	return nil
}

// ── shared helpers ────────────────────────────────────────────────────────────

// readLines reads lines from the provided files (via virtual FS) or from
// hc.Stdin if files is empty.
func readLines(s *Shell, hc interp.HandlerContext, files []string) ([]string, error) {
	var lines []string
	if len(files) == 0 {
		sc := bufio.NewScanner(hc.Stdin)
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		return lines, sc.Err()
	}
	for _, f := range files {
		r, err := s.fs.Open(s.resolvePath(f))
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		r.Close()
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	return lines, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
