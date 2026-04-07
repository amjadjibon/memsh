package native

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/amjadjibon/memsh/shell/plugins"
	"github.com/spf13/afero"
)

type CpPlugin struct{}

func (CpPlugin) Name() string                                 { return "cp" }
func (CpPlugin) Description() string                          { return "copy files or directories" }
func (CpPlugin) Usage() string                                { return "cp [-r] [-v] <src> <dst>" }
func (CpPlugin) Run(ctx context.Context, args []string) error { return runCp(ctx, args) }

func runCp(ctx context.Context, args []string) error {
	sc := plugins.ShellCtx(ctx)
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
	dst := sc.ResolvePath(positional[len(positional)-1])
	sources := positional[:len(positional)-1]
	for _, src := range sources {
		absSrc := sc.ResolvePath(src)
		srcInfo, err := sc.FS.Stat(absSrc)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("cp: cannot stat '%s': No such file or directory", src)
			}
			return fmt.Errorf("cp: %w", err)
		}
		target := dst
		dstInfo, _ := sc.FS.Stat(dst)
		if dstInfo != nil && dstInfo.IsDir() {
			target = filepath.Join(dst, filepath.Base(absSrc))
		}
		if srcInfo.IsDir() {
			if !recursive {
				return fmt.Errorf("cp: -r not specified; omitting directory '%s'", src)
			}
			if err := cpDir(sc.FS, absSrc, target); err != nil {
				return err
			}
		} else if err := cpFile(sc.FS, absSrc, target); err != nil {
			return err
		}
	}
	return nil
}

func cpFile(fs afero.Fs, src, dst string) error {
	in, err := fs.Open(src)
	if err != nil {
		return fmt.Errorf("cp: %w", err)
	}
	defer in.Close()
	out, err := fs.Create(dst)
	if err != nil {
		return fmt.Errorf("cp: %w", err)
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func cpDir(fs afero.Fs, src, dst string) error {
	return afero.Walk(fs, src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return fs.MkdirAll(target, 0755)
		}
		return cpFile(fs, path, target)
	})
}
