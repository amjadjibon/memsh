package native

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/shell/plugins"
)

// TarPlugin creates and extracts tar archives (optionally gzip/bzip2 compressed).
//
//	tar -czf archive.tar.gz file1 file2 dir/
//	tar -xf archive.tar.gz
//	tar -xf archive.tar.gz -C /dest
//	tar -tf archive.tar.gz
type TarPlugin struct{}

func (TarPlugin) Name() string        { return "tar" }
func (TarPlugin) Description() string { return "create and extract tar archives" }
func (TarPlugin) Usage() string {
	return "tar [-c|-x|-t] [-z|-j] [-v] [-f archive] [-C dir] [file...]"
}

func (TarPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	var (
		create, extract, list bool
		useGzip, useBzip2     bool
		verbose               bool
		archiveFile           string
		changeDir             string
		files                 []string
	)

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
		switch {
		case a == "--file" || a == "-f":
			if i+1 >= len(args) {
				return fmt.Errorf("tar: option requires an argument -- 'f'")
			}
			i++
			archiveFile = args[i]
			continue
		case strings.HasPrefix(a, "--file="):
			archiveFile = strings.TrimPrefix(a, "--file=")
			continue
		case a == "--directory" || a == "-C":
			if i+1 >= len(args) {
				return fmt.Errorf("tar: option requires an argument -- 'C'")
			}
			i++
			changeDir = args[i]
			continue
		case strings.HasPrefix(a, "--directory="):
			changeDir = strings.TrimPrefix(a, "--directory=")
			continue
		case a == "--create":
			create = true
			continue
		case a == "--extract" || a == "--get":
			extract = true
			continue
		case a == "--list":
			list = true
			continue
		case a == "--gzip":
			useGzip = true
			continue
		case a == "--bzip2":
			useBzip2 = true
			continue
		case a == "--verbose":
			verbose = true
			continue
		}

		// short flags (combined): -czf, -xzf, etc.
		flagStr := a[1:]
		unknown := ""
		for _, c := range flagStr {
			switch c {
			case 'c':
				create = true
			case 'x':
				extract = true
			case 't':
				list = true
			case 'z':
				useGzip = true
			case 'j':
				useBzip2 = true
			case 'v':
				verbose = true
			case 'f':
				// next arg (or rest of args) is the archive file
				// handled above for -f; when combined like czf we take args[i+1]
				if i+1 >= len(args) {
					return fmt.Errorf("tar: option requires an argument -- 'f'")
				}
				i++
				archiveFile = args[i]
			case 'C':
				if i+1 >= len(args) {
					return fmt.Errorf("tar: option requires an argument -- 'C'")
				}
				i++
				changeDir = args[i]
			case 'p', 'P', 'a':
				// ignore preserve-permissions, absolute-names, auto-compress
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("tar: invalid option -- '%s'", unknown)
		}
	}

	// auto-detect compression from extension when not specified
	if archiveFile != "" && !useGzip && !useBzip2 {
		lower := strings.ToLower(archiveFile)
		switch {
		case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
			useGzip = true
		case strings.HasSuffix(lower, ".tar.bz2") || strings.HasSuffix(lower, ".tbz2"):
			useBzip2 = true
		}
	}

	modeCount := 0
	for _, b := range []bool{create, extract, list} {
		if b {
			modeCount++
		}
	}
	if modeCount != 1 {
		return fmt.Errorf("tar: you must specify one of -c, -x, or -t")
	}

	switch {
	case create:
		return tarCreate(hc, sc, archiveFile, files, useGzip, verbose)
	case extract:
		return tarExtract(hc, sc, archiveFile, changeDir, useGzip, useBzip2, verbose)
	case list:
		return tarList(hc, sc, archiveFile, useGzip, useBzip2)
	}
	return nil
}

// tarCreate builds a (optionally gzip-compressed) tar archive on the virtual FS.
func tarCreate(hc interp.HandlerContext, sc plugins.ShellContext, archiveFile string, files []string, useGzip, verbose bool) error {
	if len(files) == 0 {
		return fmt.Errorf("tar: no files specified")
	}

	var dest io.Writer
	if archiveFile == "" || archiveFile == "-" {
		dest = hc.Stdout
	} else {
		f, err := sc.FS.Create(sc.ResolvePath(archiveFile))
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		defer f.Close()
		dest = f
	}

	var tw *tar.Writer
	if useGzip {
		gw := gzip.NewWriter(dest)
		defer gw.Close()
		tw = tar.NewWriter(gw)
	} else {
		tw = tar.NewWriter(dest)
	}
	defer tw.Close()

	for _, src := range files {
		abs := sc.ResolvePath(src)
		if err := tarAddPath(tw, sc.FS, abs, src, verbose, hc.Stdout); err != nil {
			return err
		}
	}
	return nil
}

// tarAddPath recursively adds a file or directory to the tar writer.
func tarAddPath(tw *tar.Writer, fs afero.Fs, abs, name string, verbose bool, out io.Writer) error {
	info, err := fs.Stat(abs)
	if err != nil {
		return fmt.Errorf("tar: %s: %w", name, err)
	}

	if info.IsDir() {
		entries, err := afero.ReadDir(fs, abs)
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		for _, e := range entries {
			childAbs := abs + "/" + e.Name()
			childName := name + "/" + e.Name()
			if err := tarAddPath(tw, fs, childAbs, childName, verbose, out); err != nil {
				return err
			}
		}
		return nil
	}

	hdr := &tar.Header{
		Name:    name,
		Size:    info.Size(),
		Mode:    int64(info.Mode()),
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("tar: %w", err)
	}
	if verbose {
		fmt.Fprintln(out, name)
	}
	f, err := fs.Open(abs)
	if err != nil {
		return fmt.Errorf("tar: %w", err)
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}

// tarExtract unpacks a tar archive into the virtual FS.
func tarExtract(hc interp.HandlerContext, sc plugins.ShellContext, archiveFile, changeDir string, useGzip, useBzip2, verbose bool) error {
	var src io.Reader
	if archiveFile == "" || archiveFile == "-" {
		src = hc.Stdin
	} else {
		f, err := sc.FS.Open(sc.ResolvePath(archiveFile))
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		defer f.Close()
		src = f
	}

	if useGzip {
		gr, err := gzip.NewReader(src)
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		defer gr.Close()
		src = gr
	} else if useBzip2 {
		src = bzip2.NewReader(src)
	}

	destDir := "/"
	if changeDir != "" {
		destDir = sc.ResolvePath(changeDir)
	}

	tr := tar.NewReader(src)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}

		// sanitise path — strip leading / and ..
		name := filepath.Clean(hdr.Name)
		if strings.HasPrefix(name, "..") {
			continue
		}
		target := destDir + "/" + name

		if verbose {
			fmt.Fprintln(hc.Stdout, hdr.Name)
		}

		if hdr.Typeflag == tar.TypeDir {
			if err := sc.FS.MkdirAll(target, os.FileMode(hdr.Mode)|0o700); err != nil {
				return fmt.Errorf("tar: %w", err)
			}
			continue
		}

		// ensure parent dirs exist
		if err := sc.FS.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		f, err := sc.FS.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)|0o600)
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		_, copyErr := io.Copy(f, tr)
		f.Close()
		if copyErr != nil {
			return fmt.Errorf("tar: %w", copyErr)
		}
	}
	return nil
}

// tarList prints the contents of a tar archive without extracting.
func tarList(hc interp.HandlerContext, sc plugins.ShellContext, archiveFile string, useGzip, useBzip2 bool) error {
	var src io.Reader
	if archiveFile == "" || archiveFile == "-" {
		src = hc.Stdin
	} else {
		f, err := sc.FS.Open(sc.ResolvePath(archiveFile))
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		defer f.Close()
		src = f
	}

	if useGzip {
		gr, err := gzip.NewReader(src)
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		defer gr.Close()
		src = gr
	} else if useBzip2 {
		src = bzip2.NewReader(src)
	}

	tr := tar.NewReader(src)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		fmt.Fprintln(hc.Stdout, hdr.Name)
	}
	return nil
}

// ensure TarPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = TarPlugin{}
