package shell

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/amjadjibon/memsh/shell/plugins"
	"github.com/spf13/afero"
	"golang.org/x/term"
	"mvdan.cc/sh/v3/interp"
)

var ErrExit = errors.New("exit")

// contextReader wraps an io.Reader to respect context cancellation
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

// copyWithContext copies from src to dst while respecting context cancellation
// For terminals, it sets up a special signal handler to detect Ctrl+C quickly
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	// Check if src is a terminal
	isTerminal := false
	if f, ok := src.(*os.File); ok {
		isTerminal = term.IsTerminal(int(f.Fd()))
	}

	if !isTerminal {
		// For non-terminal input, use standard copy with chunking
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
				nw, errw := dst.Write(buf[0:nr])
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

	// For terminal input, use goroutine with signal handling for better Ctrl+C response
	type result struct {
		n   int64
		err error
	}

	resCh := make(chan result, 1)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)

	// Copy in a goroutine
	go func() {
		n, err := io.Copy(dst, src)
		resCh <- result{n, err}
	}()

	// Wait for copy to complete or context/signal to cancel
	select {
	case res := <-resCh:
		signal.Stop(sigCh)
		return res.n, res.err
	case <-sigCh:
		// SIGINT received - cancel and return
		signal.Stop(sigCh)
		return 0, nil
	case <-ctx.Done():
		signal.Stop(sigCh)
		return 0, ctx.Err()
	}
}

// scanWithContext creates a context-aware buffered scanner
func scanWithContext(ctx context.Context, r io.Reader) *bufio.Scanner {
	cr := &contextReader{ctx: ctx, r: r}
	return bufio.NewScanner(cr)
}

func (s *Shell) execHandler(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		if len(args) == 0 {
			return nil
		}

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
		case "printf":
			return s.builtinPrintf(ctx, args)
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
		case "wc":
			return s.builtinWc(ctx, args)
		case "man", "help":
			return s.builtinHelp(ctx, args)
		case "read":
			return s.builtinRead(ctx, args)
		case "seq":
			return s.builtinSeq(ctx, args)
		case "date":
			return s.builtinDate(ctx, args)
		case "sleep":
			return s.builtinSleep(ctx, args)
		case "rmdir":
			return s.builtinRmdir(ctx, args)
		case "yes":
			return s.builtinYes(ctx, args)
		case "clear", "reset":
			return s.builtinClear(ctx, args)
		case "env", "printenv":
			return s.builtinEnv(ctx, args)
		case "which":
			return s.builtinWhich(ctx, args)
		case "ln":
			return s.builtinLn(ctx, args)
		case "xargs":
			return s.builtinXargs(ctx, args)
		case "timeout":
			return s.builtinTimeout(ctx, next, args)
		case "source", ".":
			return s.builtinSource(ctx, args)
		case "du":
			return s.builtinDu(ctx, args)
		case "df":
			return s.builtinDf(ctx, args)
		case "grep":
			return s.builtinGrep(ctx, args)
		case "find":
			return s.builtinFind(ctx, args)
		case "sed":
			return s.builtinSed(ctx, args)
		default:
			if fn, ok := s.builtins[args[0]]; ok {
				return fn(ctx, args)
			}
			if _, ok := s.plugins[args[0]]; ok {
				return s.runPlugin(ctx, args[0], args)
			}
			if s.allowExternalCmds {
				return next(ctx, args)
			}
			return fmt.Errorf("%s: command not found", args[0])
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
	s.runner.Dir = target
	return nil
}

func (s *Shell) builtinPwd(ctx context.Context, _ []string) error {
	hc := interp.HandlerCtx(ctx)
	_, err := fmt.Fprintln(hc.Stdout, s.cwd)
	return err
}

func (s *Shell) builtinMkdir(ctx context.Context, args []string) error {
	verbose := false
	var perm os.FileMode = 0755
	var dirs []string

	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			dirs = append(dirs, args[i+1:]...)
			break
		}
		if a == "" || a[0] != '-' {
			dirs = append(dirs, a)
			continue
		}
		if a == "-m" || a == "--mode" {
			if i+1 >= len(args) {
				return fmt.Errorf("mkdir: missing operand for -m")
			}
			i++
			v, err := strconv.ParseUint(args[i], 8, 32)
			if err != nil {
				return fmt.Errorf("mkdir: invalid mode '%s'", args[i])
			}
			perm = os.FileMode(v)
			continue
		}
		if a == "--parents" {
			continue
		}
		if a == "--verbose" {
			verbose = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'p':
			case 'v':
				verbose = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("mkdir: invalid option -- '%s'", unknown)
		}
	}

	if len(dirs) == 0 {
		return fmt.Errorf("mkdir: missing operand")
	}

	hc := interp.HandlerCtx(ctx)
	for _, dir := range dirs {
		if err := s.fs.MkdirAll(s.resolvePath(dir), perm); err != nil {
			return fmt.Errorf("mkdir: cannot create directory '%s': %w", dir, err)
		}
		if verbose {
			fmt.Fprintf(hc.Stdout, "mkdir: created directory '%s'\n", dir)
		}
	}
	return nil
}

func (s *Shell) builtinRm(ctx context.Context, args []string) error {
	force := false
	recursive := false
	verbose := false
	dirOnly := false
	interactive := false
	endOfFlags := false
	var targets []string

	for _, a := range args[1:] {
		if endOfFlags || a == "" || a[0] != '-' {
			targets = append(targets, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		if a == "--force" {
			force = true
			continue
		}
		if a == "--recursive" {
			recursive = true
			continue
		}
		if a == "--verbose" {
			verbose = true
			continue
		}
		if a == "--dir" {
			dirOnly = true
			continue
		}
		if a == "--interactive" {
			interactive = true
			continue
		}
		// combined short flags: -rf, -rfv, etc.
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'f':
				force = true
			case 'r', 'R':
				recursive = true
			case 'v':
				verbose = true
			case 'd':
				dirOnly = true
			case 'i':
				interactive = true
			case 'I':
				// prompt once before removing more than 3 files — treat as interactive
				interactive = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("rm: invalid option -- '%s'", unknown)
		}
	}

	if len(targets) == 0 {
		if force {
			return nil
		}
		return fmt.Errorf("rm: missing operand")
	}

	hc := interp.HandlerCtx(ctx)
	for _, target := range targets {
		absPath := s.resolvePath(target)
		info, err := s.fs.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) && force {
				continue
			}
			return fmt.Errorf("rm: cannot remove '%s': %w", target, err)
		}
		if info.IsDir() {
			if !recursive && !dirOnly {
				return fmt.Errorf("rm: cannot remove '%s': Is a directory", target)
			}
			if interactive && recursive {
				fmt.Fprintf(hc.Stdout, "rm: descend into directory '%s'? ", target)
				var resp string
				fmt.Fscan(hc.Stdin, &resp)
				resp = strings.ToLower(strings.TrimSpace(resp))
				if resp != "y" && resp != "yes" {
					continue
				}
			}
		} else if interactive {
			fmt.Fprintf(hc.Stdout, "rm: remove regular file '%s'? ", target)
			var resp string
			fmt.Fscan(hc.Stdin, &resp)
			resp = strings.ToLower(strings.TrimSpace(resp))
			if resp != "y" && resp != "yes" {
				continue
			}
		}
		if err := s.fs.RemoveAll(absPath); err != nil {
			return fmt.Errorf("rm: cannot remove '%s': %w", target, err)
		}
		if verbose {
			if info.IsDir() {
				fmt.Fprintf(hc.Stdout, "removed directory '%s'\n", target)
			} else {
				fmt.Fprintf(hc.Stdout, "removed '%s'\n", target)
			}
		}
	}
	return nil
}

func (s *Shell) builtinRmdir(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("rmdir: missing operand")
	}
	hc := interp.HandlerCtx(ctx)
	for _, target := range args[1:] {
		absPath := s.resolvePath(target)
		info, err := s.fs.Stat(absPath)
		if err != nil {
			return fmt.Errorf("rmdir: cannot remove '%s': %w", target, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("rmdir: cannot remove '%s': Not a directory", target)
		}
		f, err := s.fs.Open(absPath)
		if err != nil {
			return err
		}
		names, _ := f.Readdirnames(-1)
		f.Close()
		if len(names) > 0 {
			return fmt.Errorf("rmdir: cannot remove '%s': Directory not empty", target)
		}
		if err := s.fs.Remove(absPath); err != nil {
			return fmt.Errorf("rmdir: cannot remove '%s': %w", target, err)
		}
		if len(args) > 2 {
			fmt.Fprintf(hc.Stdout, "rmdir: removing directory, '%s'\n", target)
		}
	}
	return nil
}

func (s *Shell) builtinTouch(_ context.Context, args []string) error {
	noCreate := false
	reference := ""
	var targets []string

	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			targets = append(targets, args[i+1:]...)
			break
		}
		if a == "" || a[0] != '-' || a == "-" {
			targets = append(targets, a)
			continue
		}
		if a == "-r" {
			if i+1 >= len(args) {
				return fmt.Errorf("touch: missing operand for -r")
			}
			i++
			reference = args[i]
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'c':
				noCreate = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("touch: invalid option -- '%s'", unknown)
		}
	}

	if len(targets) == 0 {
		return fmt.Errorf("touch: missing file operand")
	}

	var refTime time.Time
	if reference != "" {
		refInfo, err := s.fs.Stat(s.resolvePath(reference))
		if err != nil {
			return fmt.Errorf("touch: cannot stat '%s': %w", reference, err)
		}
		refTime = refInfo.ModTime()
	}

	for _, target := range targets {
		absPath := s.resolvePath(target)
		now := time.Now()
		t := now
		if refTime.IsZero() == false {
			t = refTime
		}
		if err := s.fs.Chtimes(absPath, t, t); err != nil {
			if os.IsNotExist(err) {
				if noCreate {
					continue
				}
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
	longFormat := false
	showAll := false
	recursive := false
	var targets []string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			targets = append(targets, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		if a == "--format=long" {
			longFormat = true
			continue
		}
		if a == "--all" {
			showAll = true
			continue
		}
		if a == "--recursive" {
			recursive = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'l':
				longFormat = true
			case 'a':
				showAll = true
			case 'R':
				recursive = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("ls: invalid option -- '%s'", unknown)
		}
	}

	if len(targets) == 0 {
		targets = []string{s.cwd}
	}

	var listOne func(target string) error
	listOne = func(target string) error {
		absPath := s.resolvePath(target)
		f, err := s.fs.Open(absPath)
		if err != nil {
			return fmt.Errorf("ls: cannot access '%s': %w", target, err)
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil {
			return err
		}

		if !info.IsDir() {
			if longFormat {
				fmt.Fprintf(hc.Stdout, "%s  %8d  %s  %s\n", info.Mode(), info.Size(), info.ModTime().Format("Jan 02 15:04"), filepath.Base(target))
			} else {
				fmt.Fprintln(hc.Stdout, filepath.Base(target))
			}
			return nil
		}

		if recursive {
			fmt.Fprintf(hc.Stdout, "%s:\n", target)
		}

		names, err := f.Readdirnames(-1)
		if err != nil {
			return err
		}
		sort.Strings(names)
		for _, name := range names {
			if !showAll && strings.HasPrefix(name, ".") {
				continue
			}
			if longFormat {
				childPath := filepath.Join(absPath, name)
				ci, err := s.fs.Stat(childPath)
				if err != nil {
					ci = nil
				}
				if ci != nil {
					prefix := "-"
					if ci.IsDir() {
						prefix = "d"
					}
					fmt.Fprintf(hc.Stdout, "%s%s  %8d  %s  %s\n", prefix, ci.Mode().Perm(), ci.Size(), ci.ModTime().Format("Jan 02 15:04"), name)
				} else {
					fmt.Fprintln(hc.Stdout, name)
				}
			} else {
				fmt.Fprintln(hc.Stdout, name)
			}
		}

		if recursive {
			for _, name := range names {
				if !showAll && strings.HasPrefix(name, ".") {
					continue
				}
				childPath := filepath.Join(absPath, name)
				ci, err := s.fs.Stat(childPath)
				if err == nil && ci.IsDir() {
					fmt.Fprintln(hc.Stdout)
					listOne(filepath.Join(target, name))
				}
			}
		}
		return nil
	}

	for _, target := range targets {
		if err := listOne(target); err != nil {
			return err
		}
	}
	return nil
}

func (s *Shell) builtinCat(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	if len(args) < 2 {
		// Read from stdin with context awareness for Ctrl+C support
		_, err := copyWithContext(ctx, hc.Stdout, hc.Stdin)
		if ctx.Err() != nil {
			// Context was cancelled (Ctrl+C), don't return an error
			return nil
		}
		return err
	}
	for _, target := range args[1:] {
		if target == "-" {
			_, err := copyWithContext(ctx, hc.Stdout, hc.Stdin)
			if err != nil && ctx.Err() == nil {
				// Only return error if not a context cancellation
				return err
			}
			continue
		}
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
	noNewline := false
	interpretEsc := false
	var parts []string
	skipFlags := true

	for _, arg := range args[1:] {
		if skipFlags {
			if arg == "-n" {
				noNewline = true
				continue
			}
			if arg == "-e" {
				interpretEsc = true
				continue
			}
			if arg == "-ne" || arg == "-en" {
				noNewline = true
				interpretEsc = true
				continue
			}
			if arg == "-E" {
				continue
			}
			if strings.HasPrefix(arg, "-") && len(arg) > 1 {
				hasFlags := true
				for _, c := range arg[1:] {
					if c != 'n' && c != 'e' && c != 'E' {
						hasFlags = false
						break
					}
				}
				if hasFlags {
					for _, c := range arg[1:] {
						switch c {
						case 'n':
							noNewline = true
						case 'e':
							interpretEsc = true
						}
					}
					continue
				}
			}
		}
		skipFlags = false
		parts = append(parts, arg)
	}

	text := strings.Join(parts, " ")
	if interpretEsc {
		text = expandEscapeSequences(text)
	}

	if noNewline {
		fmt.Fprint(hc.Stdout, text)
	} else {
		fmt.Fprintln(hc.Stdout, text)
	}
	return nil
}

func expandEscapeSequences(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				sb.WriteByte('\n')
				i++
			case 't':
				sb.WriteByte('\t')
				i++
			case 'r':
				sb.WriteByte('\r')
				i++
			case '\\':
				sb.WriteByte('\\')
				i++
			case 'a':
				sb.WriteByte('\a')
				i++
			case 'b':
				sb.WriteByte('\b')
				i++
			case 'f':
				sb.WriteByte('\f')
				i++
			case 'v':
				sb.WriteByte('\v')
				i++
			case 'x':
				if i+3 < len(s) {
					hex := s[i+2 : i+4]
					if v, err := strconv.ParseUint(hex, 16, 8); err == nil {
						sb.WriteByte(byte(v))
						i += 3
						continue
					}
				}
				sb.WriteByte(s[i])
			default:
				if s[i+1] >= '0' && s[i+1] <= '7' {
					end := i + 2
					for end < len(s) && end < i+4 && s[end] >= '0' && s[end] <= '7' {
						end++
					}
					if v, err := strconv.ParseUint(s[i+1:end], 8, 8); err == nil {
						sb.WriteByte(byte(v))
						i = end - 1
						continue
					}
				}
				sb.WriteByte(s[i])
			}
		} else {
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}

func (s *Shell) builtinPrintf(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	if len(args) < 2 {
		return nil
	}
	format := args[1]
	formatArgs := args[2:]

	result := expandPrintfFormat(format, formatArgs)
	fmt.Fprint(hc.Stdout, result)
	return nil
}

func expandPrintfFormat(format string, args []string) string {
	var sb strings.Builder
	argIdx := 0
	for i := 0; i < len(format); i++ {
		if format[i] == '\\' && i+1 < len(format) {
			switch format[i+1] {
			case 'n':
				sb.WriteByte('\n')
				i++
			case 't':
				sb.WriteByte('\t')
				i++
			case 'r':
				sb.WriteByte('\r')
				i++
			case '\\':
				sb.WriteByte('\\')
				i++
			case 'a':
				sb.WriteByte('\a')
				i++
			case 'b':
				sb.WriteByte('\b')
				i++
			case 'f':
				sb.WriteByte('\f')
				i++
			case 'v':
				sb.WriteByte('\v')
				i++
			default:
				sb.WriteByte(format[i])
			}
			continue
		}
		if format[i] == '%' && i+1 < len(format) {
			spec := format[i+1]
			arg := ""
			if argIdx < len(args) {
				arg = args[argIdx]
				argIdx++
			}
			switch spec {
			case 's':
				sb.WriteString(arg)
			case 'd':
				if v, err := strconv.Atoi(arg); err == nil {
					sb.WriteString(strconv.Itoa(v))
				} else {
					sb.WriteByte('0')
				}
			case 'f':
				if v, err := strconv.ParseFloat(arg, 64); err == nil {
					sb.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
				} else {
					sb.WriteString("0.000000")
				}
			case '%':
				sb.WriteByte('%')
				argIdx--
			case 'c':
				if len(arg) > 0 {
					sb.WriteByte(arg[0])
				}
			case 'x':
				if v, err := strconv.Atoi(arg); err == nil {
					sb.WriteString(strconv.FormatInt(int64(v), 16))
				}
			case 'o':
				if v, err := strconv.Atoi(arg); err == nil {
					sb.WriteString(strconv.FormatInt(int64(v), 8))
				}
			default:
				sb.WriteByte('%')
				sb.WriteByte(spec)
				argIdx--
			}
			i++
			continue
		}
		sb.WriteByte(format[i])
	}
	return sb.String()
}

func (s *Shell) builtinCp(_ context.Context, args []string) error {
	recursive := false
	var positional []string
	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			positional = append(positional, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		if a == "--recursive" {
			recursive = true
			continue
		}
		if a == "--verbose" || a == "--preserve" || a == "--interactive" {
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'r', 'R':
				recursive = true
			case 'v', 'p', 'i':
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("cp: invalid option -- '%s'", unknown)
		}
	}
	if len(positional) < 2 {
		return fmt.Errorf("cp: missing destination file operand")
	}

	dst := s.resolvePath(positional[len(positional)-1])
	sources := positional[:len(positional)-1]

	for _, src := range sources {
		absSrc := s.resolvePath(src)
		srcInfo, err := s.fs.Stat(absSrc)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("cp: cannot stat '%s': No such file or directory", src)
			}
			return fmt.Errorf("cp: %w", err)
		}

		target := dst
		dstInfo, _ := s.fs.Stat(dst)
		if dstInfo != nil && dstInfo.IsDir() {
			target = filepath.Join(dst, filepath.Base(absSrc))
		}

		if srcInfo.IsDir() {
			if !recursive {
				return fmt.Errorf("cp: -r not specified; omitting directory '%s'", src)
			}
			if err := s.cpDir(absSrc, target); err != nil {
				return err
			}
		} else {
			if err := s.cpFile(absSrc, target); err != nil {
				return err
			}
		}
	}
	return nil
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

	dst := s.resolvePath(args[len(args)-1])
	sources := args[1 : len(args)-1]

	for _, src := range sources {
		absSrc := s.resolvePath(src)
		target := dst
		dstInfo, _ := s.fs.Stat(dst)
		if dstInfo != nil && dstInfo.IsDir() {
			target = filepath.Join(dst, filepath.Base(absSrc))
		}

		if err := s.fs.Rename(absSrc, target); err != nil {
			return fmt.Errorf("mv: cannot move '%s' to '%s': %w", src, args[len(args)-1], err)
		}
	}
	return nil
}

func (s *Shell) builtinHead(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	n := 10
	byteMode := false
	byteCount := 0
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
		} else if a == "-c" {
			if i+1 >= len(args) {
				return fmt.Errorf("head: option requires an argument -- 'c'")
			}
			i++
			byteMode = true
			v, err := strconv.Atoi(args[i])
			if err != nil || v < 0 {
				return fmt.Errorf("head: invalid number of bytes: '%s'", args[i])
			}
			byteCount = v
		} else if strings.HasPrefix(a, "-c") {
			byteMode = true
			v, err := strconv.Atoi(a[2:])
			if err != nil || v < 0 {
				return fmt.Errorf("head: invalid number of bytes: '%s'", a[2:])
			}
			byteCount = v
		} else {
			files = append(files, a)
		}
	}

	readHead := func(r io.Reader) error {
		if byteMode {
			_, err := io.CopyN(hc.Stdout, r, int64(byteCount))
			if err != nil && err != io.EOF {
				return err
			}
			return nil
		}
		return headLines(r, hc.Stdout, n)
	}

	if len(files) == 0 {
		return readHead(hc.Stdin)
	}
	for _, f := range files {
		r, err := s.fs.Open(s.resolvePath(f))
		if err != nil {
			return fmt.Errorf("head: %s: %w", f, err)
		}
		err = readHead(r)
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
	byteMode := false
	byteCount := 0
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
		} else if a == "-c" {
			if i+1 >= len(args) {
				return fmt.Errorf("tail: option requires an argument -- 'c'")
			}
			i++
			byteMode = true
			v, err := strconv.Atoi(args[i])
			if err != nil || v < 0 {
				return fmt.Errorf("tail: invalid number of bytes: '%s'", args[i])
			}
			byteCount = v
		} else if strings.HasPrefix(a, "-c") {
			byteMode = true
			v, err := strconv.Atoi(a[2:])
			if err != nil || v < 0 {
				return fmt.Errorf("tail: invalid number of bytes: '%s'", a[2:])
			}
			byteCount = v
		} else {
			files = append(files, a)
		}
	}

	readTail := func(r io.Reader) error {
		if byteMode {
			data, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			start := len(data) - byteCount
			if start < 0 {
				start = 0
			}
			_, err = hc.Stdout.Write(data[start:])
			return err
		}
		return tailLines(r, hc.Stdout, n)
	}

	if len(files) == 0 {
		return readTail(hc.Stdin)
	}
	for _, f := range files {
		r, err := s.fs.Open(s.resolvePath(f))
		if err != nil {
			return fmt.Errorf("tail: %s: %w", f, err)
		}
		err = readTail(r)
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

func (s *Shell) builtinTee(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	appendMode := false
	var targets []string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			targets = append(targets, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		if a == "--append" {
			appendMode = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'a':
				appendMode = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("tee: invalid option -- '%s'", unknown)
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

	_, err := copyWithContext(ctx, io.MultiWriter(writers...), hc.Stdin)
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func (s *Shell) builtinSort(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	reverse, unique, numeric := false, false, false
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
		if a == "--reverse" {
			reverse = true
			continue
		}
		if a == "--unique" {
			unique = true
			continue
		}
		if a == "--numeric-sort" {
			numeric = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'r':
				reverse = true
			case 'u':
				unique = true
			case 'n':
				numeric = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("sort: invalid option -- '%s'", unknown)
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

func (s *Shell) builtinUniq(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	count, onlyDups, onlyUniq := false, false, false
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
		if a == "--count" {
			count = true
			continue
		}
		if a == "--repeated" {
			onlyDups = true
			continue
		}
		if a == "--unique" {
			onlyUniq = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'c':
				count = true
			case 'd':
				onlyDups = true
			case 'u':
				onlyUniq = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("uniq: invalid option -- '%s'", unknown)
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

func (s *Shell) builtinCut(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	delim := "\t"
	var fieldList, charList string
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
		if a == "-d" || a == "--delimiter" {
			if i+1 >= len(args) {
				return fmt.Errorf("cut: missing operand for -d")
			}
			i++
			delim = args[i]
			continue
		}
		if a == "-f" || a == "--fields" {
			if i+1 >= len(args) {
				return fmt.Errorf("cut: missing operand for -f")
			}
			i++
			fieldList = args[i]
			continue
		}
		if a == "-c" || a == "--characters" {
			if i+1 >= len(args) {
				return fmt.Errorf("cut: missing operand for -c")
			}
			i++
			charList = args[i]
			continue
		}
		if a == "-s" || a == "--only-delimited" {
			continue
		}
		return fmt.Errorf("cut: invalid option -- '%s'", a)
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

func (s *Shell) builtinTr(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	delete, squeeze, complement := false, false, false
	var positional []string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			positional = append(positional, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		if a == "--delete" {
			delete = true
			continue
		}
		if a == "--squeeze-repeats" {
			squeeze = true
			continue
		}
		if a == "--complement" {
			complement = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'd':
				delete = true
			case 's':
				squeeze = true
			case 'c':
				complement = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("tr: invalid option -- '%s'", unknown)
		}
	}

	if len(positional) == 0 {
		return fmt.Errorf("tr: missing operand")
	}

	set1 := expandTrSet(positional[0])
	if complement {
		var complemented []rune
		for r := rune(0); r <= 127; r++ {
			found := false
			for _, sr := range set1 {
				if sr == r {
					found = true
					break
				}
			}
			if !found {
				complemented = append(complemented, r)
			}
		}
		set1 = complemented
	}

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

func (s *Shell) builtinChmod(_ context.Context, args []string) error {
	recursive := false
	endOfFlags := false
	var positional []string

	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			positional = append(positional, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		if a == "--recursive" {
			recursive = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'R':
				recursive = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("chmod: invalid option -- '%s'", unknown)
		}
	}

	if len(positional) < 2 {
		return fmt.Errorf("chmod: missing operand")
	}
	modeStr := positional[0]
	targets := positional[1:]

	var mode os.FileMode
	if strings.ContainsAny(modeStr, "ugoa+-=rwx") {
		m, err := parseSymbolicMode(modeStr)
		if err != nil {
			return fmt.Errorf("chmod: invalid mode '%s': %w", modeStr, err)
		}
		mode = m
	} else {
		v, err := strconv.ParseUint(modeStr, 8, 32)
		if err != nil {
			return fmt.Errorf("chmod: invalid mode '%s'", modeStr)
		}
		mode = os.FileMode(v)
	}

	for _, target := range targets {
		absPath := s.resolvePath(target)
		if recursive {
			if err := afero.Walk(s.fs, absPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				return s.fs.Chmod(path, mode)
			}); err != nil {
				return fmt.Errorf("chmod: cannot chmod '%s': %w", target, err)
			}
		} else {
			if err := s.fs.Chmod(absPath, mode); err != nil {
				return fmt.Errorf("chmod: cannot chmod '%s': %w", target, err)
			}
		}
	}
	return nil
}

func parseSymbolicMode(spec string) (os.FileMode, error) {
	mode := os.FileMode(0644)
	parts := strings.Split(spec, ",")
	for _, part := range parts {
		who := ""
		op := ""
		perm := ""
		i := 0
		for i < len(part) && (part[i] == 'u' || part[i] == 'g' || part[i] == 'o' || part[i] == 'a') {
			who += string(part[i])
			i++
		}
		if who == "" {
			who = "a"
		}
		if i >= len(part) {
			return 0, fmt.Errorf("invalid mode")
		}
		op = string(part[i])
		i++
		for i < len(part) && (part[i] == 'r' || part[i] == 'w' || part[i] == 'x') {
			perm += string(part[i])
			i++
		}

		for _, w := range who {
			for _, p := range perm {
				bit := os.FileMode(0)
				switch p {
				case 'r':
					switch w {
					case 'u':
						bit = 0400
					case 'g':
						bit = 0040
					case 'o':
						bit = 0004
					case 'a':
						bit = 0400 | 0040 | 0004
					}
				case 'w':
					switch w {
					case 'u':
						bit = 0200
					case 'g':
						bit = 0020
					case 'o':
						bit = 0002
					case 'a':
						bit = 0200 | 0020 | 0002
					}
				case 'x':
					switch w {
					case 'u':
						bit = 0100
					case 'g':
						bit = 0010
					case 'o':
						bit = 0001
					case 'a':
						bit = 0100 | 0010 | 0001
					}
				}
				switch op {
				case "+":
					mode |= bit
				case "-":
					mode &^= bit
				case "=":
					mask := os.FileMode(0)
					switch w {
					case 'u':
						mask = 0700
					case 'g':
						mask = 0070
					case 'o':
						mask = 0007
					case 'a':
						mask = 0777
					}
					mode = (mode &^ mask) | bit
				}
			}
		}
	}
	return mode, nil
}

// diffOpKind labels a single diff operation.
type diffOpKind int

const (
	diffKeep diffOpKind = iota
	diffDelete
	diffInsert
)

// diffOp is one operation from the LCS backtrack.
type diffOp struct {
	kind diffOpKind
	aIdx int // 0-based index in slice a (Keep/Delete)
	bIdx int // 0-based index in slice b (Keep/Insert)
	line string
}

// diffLCS builds the LCS DP table and backtracks to produce a diffOp slice.
func diffLCS(a, b []string) []diffOp {
	m, n := len(a), len(b)
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

	ops := make([]diffOp, 0, m+n)
	i, j := m, n
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			i--
			j--
			ops = append(ops, diffOp{diffKeep, i, j, a[i]})
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			j--
			ops = append(ops, diffOp{diffInsert, i, j, b[j]})
		default:
			i--
			ops = append(ops, diffOp{diffDelete, i, j, a[i]})
		}
	}
	// reverse
	for l, r := 0, len(ops)-1; l < r; l, r = l+1, r-1 {
		ops[l], ops[r] = ops[r], ops[l]
	}
	return ops
}

func (s *Shell) builtinDiff(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	// flags
	unified := false
	ctxLines := 3
	ignoreCase := false
	ignoreSpace := false
	ignoreAllSpace := false
	quiet := false
	color := false
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
		// long flags
		if a == "--unified" {
			unified = true
			continue
		}
		if a == "--ignore-case" {
			ignoreCase = true
			continue
		}
		if a == "--ignore-space-change" {
			ignoreSpace = true
			continue
		}
		if a == "--ignore-all-space" {
			ignoreAllSpace = true
			continue
		}
		if a == "--quiet" || a == "--brief" {
			quiet = true
			continue
		}
		if a == "--color" || a == "--color=always" || a == "--color=auto" {
			color = true
			continue
		}
		if a == "--color=never" {
			color = false
			continue
		}
		// -U N
		if a == "-U" && i+1 < len(args) {
			fmt.Sscanf(args[i+1], "%d", &ctxLines)
			i++
			unified = true
			continue
		}
		if len(a) > 2 && a[:2] == "-U" {
			fmt.Sscanf(a[2:], "%d", &ctxLines)
			unified = true
			continue
		}
		// short flags
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'u':
				unified = true
			case 'i':
				ignoreCase = true
			case 'b':
				ignoreSpace = true
			case 'w':
				ignoreAllSpace = true
			case 'q':
				quiet = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			fmt.Fprintf(hc.Stderr, "diff: invalid option -- '%s'\n", unknown)
			return interp.ExitStatus(2)
		}
	}

	if len(files) < 2 {
		fmt.Fprintln(hc.Stderr, "diff: missing operand")
		return interp.ExitStatus(2)
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

	normalise := func(line string) string {
		if ignoreAllSpace {
			// remove all whitespace
			var buf []byte
			for _, c := range line {
				if c != ' ' && c != '\t' {
					buf = append(buf, string(c)...)
				}
			}
			if ignoreCase {
				return strings.ToLower(string(buf))
			}
			return string(buf)
		}
		if ignoreSpace {
			// collapse runs of whitespace
			prev := ' '
			var buf []byte
			for _, c := range line {
				if c == ' ' || c == '\t' {
					if prev != ' ' {
						buf = append(buf, ' ')
					}
					prev = ' '
				} else {
					buf = append(buf, string(c)...)
					prev = c
				}
			}
			return strings.TrimRight(string(buf), " ")
		}
		if ignoreCase {
			return strings.ToLower(line)
		}
		return line
	}

	rawA, err := readFileLines(files[0])
	if err != nil {
		return err
	}
	rawB, err := readFileLines(files[1])
	if err != nil {
		return err
	}

	// build normalised copies for comparison
	normA := make([]string, len(rawA))
	normB := make([]string, len(rawB))
	for i, l := range rawA {
		normA[i] = normalise(l)
	}
	for i, l := range rawB {
		normB[i] = normalise(l)
	}

	ops := diffLCS(normA, normB)

	// check if any changes exist
	changed := false
	for _, op := range ops {
		if op.kind != diffKeep {
			changed = true
			break
		}
	}

	if !changed {
		return nil
	}

	if quiet {
		fmt.Fprintf(hc.Stdout, "Files %s and %s differ\n", files[0], files[1])
		return interp.ExitStatus(1)
	}

	// ANSI helpers
	ansiRed := "\033[31m"
	ansiGreen := "\033[32m"
	ansiCyan := "\033[36m"
	ansiBoldDiff := "\033[1m"
	ansiResetDiff := "\033[0m"
	col := func(code, text string) string {
		if !color {
			return text
		}
		return code + text + ansiResetDiff
	}

	if unified {
		// unified diff output
		fmt.Fprintln(hc.Stdout, col(ansiBoldDiff, "--- "+files[0]))
		fmt.Fprintln(hc.Stdout, col(ansiBoldDiff, "+++ "+files[1]))

		// build hunks by scanning ops
		opLen := len(ops)
		i := 0
		for i < opLen {
			// find next changed op
			if ops[i].kind == diffKeep {
				i++
				continue
			}
			// found a change; determine hunk bounds
			start := i - ctxLines
			if start < 0 {
				start = 0
			}
			// extend to include all changes within ctxLines of each other
			end := i
			for end < opLen {
				if ops[end].kind != diffKeep {
					// look ahead ctxLines for another change
					lookahead := end + 1
					for lookahead < opLen && lookahead <= end+ctxLines {
						if ops[lookahead].kind != diffKeep {
							end = lookahead
							break
						}
						lookahead++
					}
					if lookahead > end+ctxLines || lookahead >= opLen {
						end++
						break
					}
				}
				end++
			}
			// add trailing context
			hunkEnd := end + ctxLines - 1
			if hunkEnd >= opLen {
				hunkEnd = opLen - 1
			}

			hunkOps := ops[start : hunkEnd+1]

			// compute line ranges
			aStart, bStart := -1, -1
			aCount, bCount := 0, 0
			for _, op := range hunkOps {
				switch op.kind {
				case diffKeep:
					if aStart == -1 {
						aStart = op.aIdx + 1
						bStart = op.bIdx + 1
					}
					aCount++
					bCount++
				case diffDelete:
					if aStart == -1 {
						aStart = op.aIdx + 1
					}
					aCount++
				case diffInsert:
					if bStart == -1 {
						bStart = op.bIdx + 1
					}
					bCount++
				}
			}
			if aStart == -1 {
				aStart = 1
			}
			if bStart == -1 {
				bStart = 1
			}

			// format hunk header
			aRange := fmt.Sprintf("%d", aStart)
			if aCount != 1 {
				aRange = fmt.Sprintf("%d,%d", aStart, aCount)
			}
			bRange := fmt.Sprintf("%d", bStart)
			if bCount != 1 {
				bRange = fmt.Sprintf("%d,%d", bStart, bCount)
			}
			header := fmt.Sprintf("@@ -%s +%s @@", aRange, bRange)
			fmt.Fprintln(hc.Stdout, col(ansiCyan, header))

			for _, op := range hunkOps {
				switch op.kind {
				case diffKeep:
					fmt.Fprintln(hc.Stdout, " "+rawA[op.aIdx])
				case diffDelete:
					fmt.Fprintln(hc.Stdout, col(ansiRed, "-"+rawA[op.aIdx]))
				case diffInsert:
					fmt.Fprintln(hc.Stdout, col(ansiGreen, "+"+rawB[op.bIdx]))
				}
			}

			i = hunkEnd + 1
		}
	} else {
		// normal (ed-style) diff output
		// group consecutive changes into hunks
		i := 0
		for i < len(ops) {
			if ops[i].kind == diffKeep {
				i++
				continue
			}
			// collect a run of changes
			j := i
			for j < len(ops) && ops[j].kind != diffKeep {
				j++
			}
			chunk := ops[i:j]

			// determine ranges
			aStart, aEnd := -1, -1
			bStart, bEnd := -1, -1
			for _, op := range chunk {
				switch op.kind {
				case diffDelete:
					if aStart == -1 {
						aStart = op.aIdx + 1
					}
					aEnd = op.aIdx + 1
				case diffInsert:
					if bStart == -1 {
						bStart = op.bIdx + 1
					}
					bEnd = op.bIdx + 1
				}
			}

			hasDelete := aStart != -1
			hasInsert := bStart != -1

			var rangeA, rangeB string
			if hasDelete {
				if aStart == aEnd {
					rangeA = fmt.Sprintf("%d", aStart)
				} else {
					rangeA = fmt.Sprintf("%d,%d", aStart, aEnd)
				}
			}
			if hasInsert {
				if bStart == bEnd {
					rangeB = fmt.Sprintf("%d", bStart)
				} else {
					rangeB = fmt.Sprintf("%d,%d", bStart, bEnd)
				}
			}

			// determine command character
			var cmd string
			switch {
			case hasDelete && hasInsert:
				cmd = "c"
				if !hasDelete {
					rangeA = fmt.Sprintf("%d", aStart)
				}
			case hasDelete:
				cmd = "d"
			case hasInsert:
				cmd = "a"
				if aStart == -1 {
					// pure insert: find preceding keep
					for k := i - 1; k >= 0; k-- {
						if ops[k].kind == diffKeep {
							rangeA = fmt.Sprintf("%d", ops[k].aIdx+1)
							break
						}
					}
					if rangeA == "" {
						rangeA = "0"
					}
				}
			}

			// build header
			var header string
			switch cmd {
			case "c":
				header = rangeA + "c" + rangeB
			case "d":
				header = rangeA + "d" + fmt.Sprintf("%d", func() int {
					// line in b after which lines were deleted
					for k := i - 1; k >= 0; k-- {
						if ops[k].kind == diffKeep {
							return ops[k].bIdx + 1
						}
					}
					return 0
				}())
			case "a":
				header = rangeA + "a" + rangeB
			}

			fmt.Fprintln(hc.Stdout, col(ansiCyan, header))

			// print deleted lines
			if hasDelete {
				for _, op := range chunk {
					if op.kind == diffDelete {
						fmt.Fprintln(hc.Stdout, col(ansiRed, "< "+rawA[op.aIdx]))
					}
				}
			}
			// separator between delete and insert
			if hasDelete && hasInsert {
				fmt.Fprintln(hc.Stdout, "---")
			}
			// print inserted lines
			if hasInsert {
				for _, op := range chunk {
					if op.kind == diffInsert {
						fmt.Fprintln(hc.Stdout, col(ansiGreen, "> "+rawB[op.bIdx]))
					}
				}
			}

			i = j
		}
	}

	return interp.ExitStatus(1)
}

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

func (s *Shell) builtinWc(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	countLines, countWords, countBytes := false, false, false
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
		if a == "--lines" {
			countLines = true
			continue
		}
		if a == "--words" {
			countWords = true
			continue
		}
		if a == "--bytes" {
			countBytes = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'l':
				countLines = true
			case 'w':
				countWords = true
			case 'c':
				countBytes = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("wc: invalid option -- '%s'", unknown)
		}
	}

	if !countLines && !countWords && !countBytes {
		countLines, countWords, countBytes = true, true, true
	}

	totalL, totalW, totalB := 0, 0, 0

	wcOne := func(r io.Reader, name string) error {
		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		content := string(data)
		lc := strings.Count(content, "\n")
		if len(content) > 0 && !strings.HasSuffix(content, "\n") {
			lc++
		}
		wc := len(strings.Fields(content))
		bc := len(data)

		totalL += lc
		totalW += wc
		totalB += bc

		var parts []string
		if countLines {
			parts = append(parts, fmt.Sprintf("%d", lc))
		}
		if countWords {
			parts = append(parts, fmt.Sprintf("%d", wc))
		}
		if countBytes {
			parts = append(parts, fmt.Sprintf("%d", bc))
		}
		if name != "" {
			parts = append(parts, name)
		}
		fmt.Fprintln(hc.Stdout, strings.Join(parts, " "))
		return nil
	}

	if len(files) == 0 {
		return wcOne(hc.Stdin, "")
	}

	for _, f := range files {
		r, err := s.fs.Open(s.resolvePath(f))
		if err != nil {
			return fmt.Errorf("wc: %s: %w", f, err)
		}
		if err := wcOne(r, f); err != nil {
			r.Close()
			return err
		}
		r.Close()
	}

	if len(files) > 1 {
		var parts []string
		if countLines {
			parts = append(parts, fmt.Sprintf("%d", totalL))
		}
		if countWords {
			parts = append(parts, fmt.Sprintf("%d", totalW))
		}
		if countBytes {
			parts = append(parts, fmt.Sprintf("%d", totalB))
		}
		parts = append(parts, "total")
		fmt.Fprintln(hc.Stdout, strings.Join(parts, " "))
	}
	return nil
}

func (s *Shell) builtinGrep(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	caseInsensitive := false
	showLineNum := false
	invert := false
	recursive := false
	countMode := false
	listFiles := false
	wholeWord := false
	_ = false
	onlyMatch := false
	var pattern string
	var files []string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			if pattern == "" {
				pattern = a
			} else {
				files = append(files, a)
			}
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		// long flags
		switch a {
		case "--ignore-case":
			caseInsensitive = true
			continue
		case "--line-number":
			showLineNum = true
			continue
		case "--invert-match":
			invert = true
			continue
		case "--recursive":
			recursive = true
			continue
		case "--count":
			countMode = true
			continue
		case "--files-with-matches":
			listFiles = true
			continue
		case "--word-regexp":
			wholeWord = true
			continue
		case "--only-matching":
			onlyMatch = true
			continue
		case "--extended-regexp", "--quiet", "--silent":
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'i':
				caseInsensitive = true
			case 'n':
				showLineNum = true
			case 'v':
				invert = true
			case 'r', 'R':
				recursive = true
			case 'c':
				countMode = true
			case 'l':
				listFiles = true
			case 'w':
				wholeWord = true
			case 'E', 'q', 's':
			case 'o':
				onlyMatch = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("grep: invalid option -- '%s'", unknown)
		}
	}

	if pattern == "" {
		return fmt.Errorf("grep: missing pattern")
	}

	if caseInsensitive {
		pattern = "(?i)" + pattern
	}
	if wholeWord {
		pattern = `\b` + pattern + `\b`
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("grep: invalid pattern: %w", err)
	}

	grepFile := func(r io.Reader, name string) (matched bool, err error) {
		sc := bufio.NewScanner(r)
		lineNum := 0
		matchCount := 0
		for sc.Scan() {
			lineNum++
			line := sc.Text()
			match := re.MatchString(line)
			if invert {
				match = !match
			}
			if match {
				matched = true
				matchCount++
				if listFiles {
					return true, nil
				}
				if countMode {
					continue
				}
				if onlyMatch {
					locs := re.FindStringIndex(line)
					if len(locs) > 0 {
						for _, loc := range re.FindAllStringIndex(line, -1) {
							prefix := ""
							if len(files) > 1 || recursive {
								prefix = name + ":"
							}
							if showLineNum {
								fmt.Fprintf(hc.Stdout, "%s%d:%s\n", prefix, lineNum, line[loc[0]:loc[1]])
							} else {
								fmt.Fprintf(hc.Stdout, "%s%s\n", prefix, line[loc[0]:loc[1]])
							}
						}
					}
					continue
				}
				prefix := ""
				if len(files) > 1 || recursive {
					prefix = name + ":"
				}
				if showLineNum {
					fmt.Fprintf(hc.Stdout, "%s%d:%s\n", prefix, lineNum, line)
				} else {
					fmt.Fprintf(hc.Stdout, "%s%s\n", prefix, line)
				}
			}
		}
		if countMode && matched {
			prefix := ""
			if len(files) > 1 || recursive {
				prefix = name + ":"
			}
			fmt.Fprintf(hc.Stdout, "%s%d\n", prefix, matchCount)
		}
		return matched, sc.Err()
	}

	if len(files) == 0 {
		matched, err := grepFile(hc.Stdin, "")
		if err != nil {
			return err
		}
		if !matched && !invert {
			return interp.ExitStatus(1)
		}
		return nil
	}

	anyMatch := false
	for _, f := range files {
		absPath := s.resolvePath(f)
		info, err := s.fs.Stat(absPath)
		if err != nil {
			return fmt.Errorf("grep: %s: %w", f, err)
		}
		if info.IsDir() && recursive {
			err := afero.Walk(s.fs, absPath, func(path string, fi os.FileInfo, err error) error {
				if err != nil || fi.IsDir() {
					return err
				}
				r, err := s.fs.Open(path)
				if err != nil {
					return err
				}
				defer r.Close()
				matched, _ := grepFile(r, path)
				if matched {
					anyMatch = true
				}
				return nil
			})
			if err != nil {
				return err
			}
		} else {
			r, err := s.fs.Open(absPath)
			if err != nil {
				return fmt.Errorf("grep: %s: %w", f, err)
			}
			matched, err := grepFile(r, f)
			r.Close()
			if err != nil {
				return err
			}
			if matched {
				anyMatch = true
			}
		}
	}

	if !anyMatch && !invert {
		return interp.ExitStatus(1)
	}
	return nil
}

func (s *Shell) builtinFind(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	searchPath := s.cwd
	var namePattern string
	var fileType string
	maxDepth := -1

	i := 1
	for i < len(args) {
		switch args[i] {
		case "-name":
			if i+1 >= len(args) {
				return fmt.Errorf("find: missing argument to '-name'")
			}
			i++
			namePattern = args[i]
		case "-type":
			if i+1 >= len(args) {
				return fmt.Errorf("find: missing argument to '-type'")
			}
			i++
			fileType = args[i]
		case "-maxdepth":
			if i+1 >= len(args) {
				return fmt.Errorf("find: missing argument to '-maxdepth'")
			}
			i++
			d, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("find: invalid argument '%s' to '-maxdepth'", args[i])
			}
			maxDepth = d
		default:
			if !strings.HasPrefix(args[i], "-") {
				searchPath = s.resolvePath(args[i])
			}
		}
		i++
	}

	return afero.Walk(s.fs, searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath := path
		if filepath.IsAbs(searchPath) {
			relPath, _ = filepath.Rel(searchPath, path)
		}

		depth := strings.Count(relPath, string(filepath.Separator))
		if relPath == "." {
			depth = 0
		}
		if maxDepth >= 0 && depth > maxDepth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if fileType == "f" && info.IsDir() {
			return nil
		}
		if fileType == "d" && !info.IsDir() {
			return nil
		}

		if namePattern != "" {
			matched, _ := filepath.Match(namePattern, filepath.Base(path))
			if !matched {
				return nil
			}
		}

		fmt.Fprintln(hc.Stdout, path)
		return nil
	})
}

func (s *Shell) builtinSed(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	if len(args) < 2 {
		return fmt.Errorf("sed: missing expression")
	}

	expr := args[1]
	var files []string
	for _, a := range args[2:] {
		files = append(files, a)
	}

	if !strings.HasPrefix(expr, "s") || len(expr) < 3 {
		return fmt.Errorf("sed: unsupported expression '%s' (only s/// is supported)", expr)
	}

	sep := expr[1]
	parts := strings.Split(expr[2:], string(sep))
	if len(parts) < 2 {
		return fmt.Errorf("sed: invalid substitution expression")
	}
	pattern := parts[0]
	replacement := parts[1]
	global := false
	if len(parts) > 2 && strings.Contains(parts[len(parts)-1], "g") {
		global = true
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("sed: invalid pattern: %w", err)
	}

	lines, err := readLines(s, hc, files)
	if err != nil {
		return err
	}

	for _, line := range lines {
		if global {
			fmt.Fprintln(hc.Stdout, re.ReplaceAllString(line, replacement))
		} else {
			fmt.Fprintln(hc.Stdout, re.ReplaceAllString(line, replacement))
		}
	}
	return nil
}

func (s *Shell) builtinRead(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	varNames := args[1:]
	if len(varNames) == 0 {
		varNames = []string{"REPLY"}
	}

	scanner := scanWithContext(ctx, hc.Stdin)
	if !scanner.Scan() {
		if ctx.Err() != nil {
			return nil // Context cancelled, no error
		}
		return fmt.Errorf("read: no input")
	}
	line := scanner.Text()

	fields := strings.Fields(line)
	for i, name := range varNames {
		if i < len(fields) {
			if i == len(varNames)-1 && len(fields) > len(varNames) {
				s.env[name] = strings.Join(fields[i:], " ")
			} else {
				s.env[name] = fields[i]
			}
		} else {
			s.env[name] = ""
		}
	}
	return nil
}

func (s *Shell) builtinSeq(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	var start, step, stop int
	var err error

	switch len(args) {
	case 1:
		return nil
	case 2:
		start, stop = 1, 0
		stop, err = strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("seq: invalid number '%s'", args[1])
		}
	case 3:
		start, err = strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("seq: invalid number '%s'", args[1])
		}
		stop, err = strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("seq: invalid number '%s'", args[2])
		}
	default:
		start, err = strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("seq: invalid number '%s'", args[1])
		}
		step, err = strconv.Atoi(args[2])
		if err != nil || step == 0 {
			return fmt.Errorf("seq: invalid increment '%s'", args[2])
		}
		stop, err = strconv.Atoi(args[3])
		if err != nil {
			return fmt.Errorf("seq: invalid number '%s'", args[3])
		}
	}

	if step == 0 {
		if start <= stop {
			step = 1
		} else {
			step = -1
		}
	}

	for i := start; ; i += step {
		fmt.Fprintln(hc.Stdout, i)
		if (step > 0 && i >= stop) || (step < 0 && i <= stop) {
			break
		}
	}
	return nil
}

func (s *Shell) builtinDate(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	format := ""
	for i := 1; i < len(args); i++ {
		if args[i] == "+%s" || args[i] == "+%F" || args[i] == "+%T" || args[i] == "+%Y-%m-%d" || args[i] == "+%H:%M:%S" {
			format = args[i][1:]
		} else if args[i] == "-u" || args[i] == "--utc" || args[i] == "--universal" {
		}
	}

	now := time.Now()
	if format == "" {
		fmt.Fprintln(hc.Stdout, now.Format("Mon Jan 2 15:04:05 UTC 2006"))
	} else {
		fmt.Fprintln(hc.Stdout, now.Format(format))
	}
	return nil
}

func (s *Shell) builtinSleep(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("sleep: missing operand")
	}
	d, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return fmt.Errorf("sleep: invalid time interval '%s'", args[1])
	}
	timer := time.NewTimer(time.Duration(d * float64(time.Second)))
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Shell) builtinYes(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	msg := "y"
	if len(args) > 1 {
		msg = strings.Join(args[1:], " ")
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if _, err := fmt.Fprintln(hc.Stdout, msg); err != nil {
			return nil
		}
	}
}

func (s *Shell) builtinClear(_ context.Context, args []string) error {
	// Write ANSI escape sequences: clear screen then move cursor to top-left.
	// Works in any VT100-compatible terminal and the memsh web terminal.
	_, err := fmt.Fprint(s.stdout, "\x1b[2J\x1b[H")
	return err
}

func (s *Shell) builtinEnv(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	for k, v := range s.env {
		fmt.Fprintf(hc.Stdout, "%s=%s\n", k, v)
	}
	return nil
}

func (s *Shell) builtinWhich(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	if len(args) < 2 {
		return nil
	}
	for _, name := range args[1:] {
		if v, ok := s.aliases[name]; ok {
			fmt.Fprintf(hc.Stdout, "%s: aliased to %s\n", name, v)
			continue
		}
		if _, ok := builtinHelp[name]; ok {
			fmt.Fprintf(hc.Stdout, "%s: shell built-in command\n", name)
			continue
		}
		if _, ok := s.builtins[name]; ok {
			fmt.Fprintf(hc.Stdout, "%s: native plugin\n", name)
			continue
		}
		if _, ok := s.plugins[name]; ok {
			fmt.Fprintf(hc.Stdout, "%s: WASM plugin\n", name)
			continue
		}
		fmt.Fprintf(hc.Stdout, "%s: not found\n", name)
	}
	return nil
}

func (s *Shell) builtinLn(_ context.Context, args []string) error {
	var positional []string
	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			positional = append(positional, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		if a == "--symbolic" || a == "--force" {
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 's', 'f':
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("ln: invalid option -- '%s'", unknown)
		}
	}
	if len(positional) < 2 {
		return fmt.Errorf("ln: missing operand")
	}

	if _, ok := s.fs.(interface {
		SymlinkIfPossible(oldname, newname string) error
	}); ok {
		return fmt.Errorf("ln: symbolic links not supported in virtual filesystem")
	}
	return fmt.Errorf("ln: not supported in virtual filesystem")
}

func (s *Shell) builtinXargs(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	if len(args) < 2 {
		return fmt.Errorf("xargs: missing command")
	}

	cmdName := args[1]
	cmdArgs := args[2:]

	scanner := scanWithContext(ctx, hc.Stdin)
	var items []string
	for scanner.Scan() {
		line := scanner.Text()
		items = append(items, strings.Fields(line)...)
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return nil // Context cancelled, no error
		}
		return err
	}

	if len(items) == 0 {
		return nil
	}

	fullArgs := append([]string{cmdName}, cmdArgs...)
	fullArgs = append(fullArgs, items...)

	// Use a not-found handler as the fallback instead of DefaultExecHandler
	// to prevent accidental real OS command execution.
	notFound := func(_ context.Context, args []string) error {
		return fmt.Errorf("%s: command not found", args[0])
	}
	return s.execHandler(notFound)(ctx, fullArgs)
}

// maxSourceDepth limits recursive `source` calls to prevent stack overflow.
const maxSourceDepth = 16

func (s *Shell) builtinSource(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("source: missing file argument")
	}

	s.sourceDepth++
	defer func() { s.sourceDepth-- }()
	if s.sourceDepth > maxSourceDepth {
		return fmt.Errorf("source: maximum recursion depth (%d) exceeded", maxSourceDepth)
	}

	absPath := s.resolvePath(args[1])
	data, err := afero.ReadFile(s.fs, absPath)
	if err != nil {
		return fmt.Errorf("source: %s: %w", args[1], err)
	}

	return s.Run(ctx, string(data))
}

func (s *Shell) builtinDu(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	humanReadable := false
	var targets []string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			targets = append(targets, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		if a == "--human-readable" || a == "--summary" {
			if a == "--human-readable" {
				humanReadable = true
			}
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'h':
				humanReadable = true
			case 's':
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("du: invalid option -- '%s'", unknown)
		}
	}

	if len(targets) == 0 {
		targets = []string{s.cwd}
	}

	formatSize := func(size int64) string {
		if humanReadable {
			const (
				KB = 1024
				MB = KB * 1024
				GB = MB * 1024
			)
			switch {
			case size >= GB:
				return fmt.Sprintf("%.1fG", float64(size)/float64(GB))
			case size >= MB:
				return fmt.Sprintf("%.1fM", float64(size)/float64(MB))
			case size >= KB:
				return fmt.Sprintf("%.1fK", float64(size)/float64(KB))
			}
		}
		return fmt.Sprintf("%d", size)
	}

	for _, target := range targets {
		absPath := s.resolvePath(target)
		var total int64
		afero.Walk(s.fs, absPath, func(_ string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				total += info.Size()
			}
			return nil
		})
		fmt.Fprintf(hc.Stdout, "%s\t%s\n", formatSize(total), target)
	}
	return nil
}

func (s *Shell) builtinDf(ctx context.Context, _ []string) error {
	hc := interp.HandlerCtx(ctx)
	fmt.Fprintln(hc.Stdout, "Filesystem     1K-blocks    Used Available Use% Mounted on")
	fmt.Fprintln(hc.Stdout, "memsh               0       0         0    - /")
	return nil
}

var builtinHelp = map[string][2]string{
	"cat":    {"concatenate and print files", "cat [file]..."},
	"cd":     {"change working directory", "cd [dir]"},
	"chmod":  {"change file permissions", "chmod [-R] <mode> <file>..."},
	"cp":     {"copy files or directories", "cp [-r] [-v] <src> <dst>"},
	"cut":    {"extract fields or characters", "cut -d <delim> -f <fields> [file]\n       cut -c <chars> [file]"},
	"date":   {"print the current date and time", "date [+format]"},
	"diff":   {"compare two files line by line", "diff [-u] <file1> <file2>"},
	"du":     {"estimate file space usage", "du [-h] [-s] [path]..."},
	"df":     {"show filesystem space", "df"},
	"echo":   {"print arguments", "echo [-n] [-e] [arg]..."},
	"env":    {"display environment variables", "env"},
	"find":   {"search virtual filesystem", "find [path] [-name <glob>] [-type f|d] [-maxdepth <n>]"},
	"grep":   {"search file contents for patterns", "grep [-i] [-n] [-v] [-r] [-c] [-l] [-w] [-E] <pattern> [file...]"},
	"head":   {"print first lines of a file", "head [-n N] [-c N] [file]"},
	"ln":     {"create links", "ln [-s] <target> <link>"},
	"ls":     {"list directory contents", "ls [-l] [-a] [-R] [path]"},
	"man":    {"show help for commands", "man [command]"},
	"mkdir":  {"create directories", "mkdir [-p] [-v] [-m mode] <dir>..."},
	"mv":     {"move or rename files", "mv <src> <dst>"},
	"printf": {"format and print data", "printf <format> [args...]"},
	"pwd":    {"print working directory", "pwd"},
	"read":   {"read a line from stdin", "read [var]..."},
	"rm":     {"remove files or directories", "rm [-f] [-r] [-R] [-v] [-d] [-i] [-I] [--] <path>..."},
	"rmdir":  {"remove empty directories", "rmdir <dir>..."},
	"sed":    {"stream editor (substitution)", "sed 's/pattern/replacement/[g]' [file]"},
	"seq":    {"print a sequence of numbers", "seq [first [increment]] last"},
	"sleep":  {"delay for a specified time", "sleep <seconds>"},
	"sort":   {"sort lines of text", "sort [-r] [-u] [-n] [file]"},
	"alias":   {"define or display aliases", "alias [name[=value] ...]"},
	"clear":   {"clear the terminal screen", "clear"},
	"unalias": {"remove alias definitions", "unalias [-a] name [name ...]"},
	"source":  {"execute commands from a file", "source <file>"},
	"stat":   {"show file status", "stat <file>..."},
	"tail":   {"print last lines of a file", "tail [-n N] [-c N] [file]"},
	"tee":    {"read stdin; write to stdout and files", "tee [-a] [file]..."},
	"touch":  {"create or update file timestamps", "touch [-c] [-r ref] <file>..."},
	"tr":     {"translate or delete characters", "tr [-d] [-s] [-c] <set1> [set2]"},
	"uniq":   {"filter adjacent duplicate lines", "uniq [-c] [-d] [-u] [file]"},
	"wc":     {"count lines, words, and bytes", "wc [-l] [-w] [-c] [file]..."},
	"which":  {"locate a command", "which <name>..."},
	"xargs":  {"build and execute command lines", "xargs <command> [args...]"},
	"yes":    {"output a string repeatedly", "yes [string]"},
	"awk":    {"pattern scanning and processing", "awk '<prog>' [file...]\n      awk -f <progfile> [file...]"},
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

func readLines(s *Shell, hc interp.HandlerContext, files []string) ([]string, error) {
	var lines []string
	if len(files) == 0 {
		sc := scanWithContext(context.Background(), hc.Stdin)
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

// builtinTimeout runs a command with a context deadline.
//
//	timeout DURATION CMD [ARGS...]
//	timeout -s SIGNAL DURATION CMD [ARGS...]   (signal ignored; always SIGTERM behaviour)
//	timeout -k KILL_AFTER DURATION CMD [ARGS...]
//
// Exit codes:
//
//	124  timed out
//	125  timeout itself failed (bad duration / missing command)
//	126  command found but not executable
//	127  command not found
//	otherwise: command's own exit code
func (s *Shell) builtinTimeout(ctx context.Context, next interp.ExecHandlerFunc, args []string) error {
	hc := interp.HandlerCtx(ctx)

	// strip recognised flags; we honour -k (ignored after kill) and -s (ignored signal)
	i := 1
	for i < len(args) {
		switch args[i] {
		case "--":
			i++
			goto doneFlags
		case "-s", "--signal":
			i += 2 // skip signal name
		case "-k", "--kill-after":
			i += 2 // skip kill-after duration
		case "--preserve-status", "--foreground":
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				// unknown flag
				fmt.Fprintf(hc.Stderr, "timeout: unknown flag %q\n", args[i])
				return interp.ExitStatus(125)
			}
			goto doneFlags
		}
	}
doneFlags:

	if i >= len(args) {
		fmt.Fprintln(hc.Stderr, "timeout: missing operand")
		return interp.ExitStatus(125)
	}

	dur, err := parseDuration(args[i])
	if err != nil {
		fmt.Fprintf(hc.Stderr, "timeout: invalid time interval %q\n", args[i])
		return interp.ExitStatus(125)
	}
	i++

	if i >= len(args) {
		fmt.Fprintln(hc.Stderr, "timeout: missing command")
		return interp.ExitStatus(125)
	}
	cmdArgs := args[i:]

	// a zero or negative duration means no timeout
	if dur <= 0 {
		return s.execHandler(next)(ctx, cmdArgs)
	}

	tctx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()

	err = s.execHandler(next)(tctx, cmdArgs)
	if err != nil {
		if errors.Is(tctx.Err(), context.DeadlineExceeded) {
			return interp.ExitStatus(124)
		}
	}
	return err
}

// parseDuration converts a timeout operand to time.Duration.
// Accepts Go duration strings (5s, 1m30s) and bare numbers (seconds).
func parseDuration(s string) (time.Duration, error) {
	// bare number → seconds (integer or float)
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(f * float64(time.Second)), nil
	}
	return time.ParseDuration(s)
}
