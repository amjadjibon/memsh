package shell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

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
		default:
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
