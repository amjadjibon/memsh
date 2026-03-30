package shell

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

