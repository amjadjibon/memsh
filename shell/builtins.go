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
			return next(ctx, args)
		}
	}
}

func (s *Shell) builtinCd(ctx context.Context, args []string) error {
	if len(args) > 2 {
		return fmt.Errorf("cd: too many arguments")
	}

	var dir string
	if len(args) == 1 {
		// No args -> go home
		// Let's assume / is home for now
		dir = "/"
	} else {
		dir = args[1]
	}

	target := s.resolvePath(dir)

	// Verify the directory exists in the virtual filesystem
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
	// We need to sync this to interp's Dir tracker if possible,
	// though mvdan.cc/sh uses os.Chdir by default unless overriden.
	// interp.Dir() configures the starting dir, but doesn't auto-track.
	// If the shell script calls "cd", mvdan's DefaultExecHandler would handle it.
	// Since we intercept it here, we're changing our `s.cwd`.
	return nil
}

func (s *Shell) builtinPwd(ctx context.Context, args []string) error {
	_, err := fmt.Fprintln(s.stdout, s.cwd)
	return err
}

func (s *Shell) builtinMkdir(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("mkdir: missing operand")
	}
	for _, dir := range args[1:] {
		target := s.resolvePath(dir)
		if err := s.fs.MkdirAll(target, 0755); err != nil {
			return fmt.Errorf("mkdir: cannot create directory '%s': %w", dir, err)
		}
	}
	return nil
}

func (s *Shell) builtinRm(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("rm: missing operand")
	}
	for _, target := range args[1:] {
		absPath := s.resolvePath(target)
		if err := s.fs.RemoveAll(absPath); err != nil {
			return fmt.Errorf("rm: cannot remove '%s': %w", target, err)
		}
	}
	return nil
}

func (s *Shell) builtinTouch(ctx context.Context, args []string) error {
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
		fmt.Fprintln(s.stdout, filepath.Base(target))
		return nil
	}

	names, err := f.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, name := range names {
		fmt.Fprintln(s.stdout, name)
	}
	return nil
}

func (s *Shell) builtinCat(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("cat: missing operand")
	}
	for _, target := range args[1:] {
		absPath := s.resolvePath(target)
		f, err := s.fs.Open(absPath)
		if err != nil {
			return fmt.Errorf("cat: %s: No such file or directory", target)
		}
		io.Copy(s.stdout, f)
		f.Close()
	}
	return nil
}

func (s *Shell) builtinEcho(ctx context.Context, args []string) error {
	for i, arg := range args[1:] {
		if i > 0 {
			fmt.Fprint(s.stdout, " ")
		}
		fmt.Fprint(s.stdout, arg)
	}
	fmt.Fprintln(s.stdout)
	return nil
}
