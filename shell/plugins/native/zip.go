package native

import (
	"archive/zip"
	"bytes"
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

// ZipPlugin creates or updates zip archives on the virtual FS.
//
//	zip archive.zip file1 file2
//	zip -r archive.zip dir/
//	zip -l archive.zip        (list, alias for unzip -l)
type ZipPlugin struct{ unzip bool }

// Zip returns a plugin registered as "zip".
func Zip() ZipPlugin { return ZipPlugin{unzip: false} }

// Unzip returns a plugin registered as "unzip".
func Unzip() ZipPlugin { return ZipPlugin{unzip: true} }

func (z ZipPlugin) Name() string {
	if z.unzip {
		return "unzip"
	}
	return "zip"
}
func (z ZipPlugin) Description() string {
	if z.unzip {
		return "extract files from a zip archive"
	}
	return "create or update zip archives"
}
func (ZipPlugin) Usage() string {
	return "zip [-r] [-0..-9] <archive.zip> <file...> | unzip [-l] [-d dir] <archive.zip>"
}

func (z ZipPlugin) Run(ctx context.Context, args []string) error {
	if z.unzip {
		return runUnzip(ctx, args)
	}
	return runZip(ctx, args)
}

// runZip implements: zip [-r] [-0..-9] archive.zip files...
func runZip(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	recursive := false
	level := zip.Deflate // compression method
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
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'r', 'R':
				recursive = true
			case '0':
				level = zip.Store
			case '1', '2', '3', '4', '5', '6', '7', '8', '9':
				level = zip.Deflate // Go's zip only has Store/Deflate
			case 'q':
				// quiet
			case 'v':
				// verbose — no-op
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("zip: invalid option -- '%s'", unknown)
		}
	}

	if len(positional) < 2 {
		return fmt.Errorf("zip: usage: zip [-r] archive.zip file...")
	}
	archivePath := sc.ResolvePath(positional[0])
	sources := positional[1:]

	// open or create archive; preserve existing entries if file exists
	var existingEntries []*zip.File
	if existing, err := sc.FS.Open(archivePath); err == nil {
		data, _ := io.ReadAll(existing)
		existing.Close()
		if zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err == nil {
			existingEntries = zr.File
		}
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// copy existing entries that aren't being replaced
	sourceSet := make(map[string]bool)
	for _, s := range sources {
		sourceSet[s] = true
	}
	for _, f := range existingEntries {
		if !sourceSet[f.Name] {
			if err := zipCopyEntry(zw, f); err != nil {
				return fmt.Errorf("zip: %w", err)
			}
		}
	}

	// add new entries
	for _, src := range sources {
		abs := sc.ResolvePath(src)
		info, err := sc.FS.Stat(abs)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "zip: %s: no such file\n", src)
			continue
		}
		if info.IsDir() {
			if !recursive {
				fmt.Fprintf(hc.Stderr, "zip: %s is a directory (use -r)\n", src)
				continue
			}
			if err := zipAddDir(zw, sc, abs, src, level); err != nil {
				return fmt.Errorf("zip: %w", err)
			}
		} else {
			if err := zipAddFile(zw, sc, abs, src, level); err != nil {
				return fmt.Errorf("zip: %w", err)
			}
		}
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("zip: %w", err)
	}

	f, err := sc.FS.Create(archivePath)
	if err != nil {
		return fmt.Errorf("zip: %w", err)
	}
	defer f.Close()
	_, err = f.Write(buf.Bytes())
	return err
}

// runUnzip implements: unzip [-l] [-d dir] archive.zip [files...]
func runUnzip(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	listOnly := false
	destDir := ""
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
		switch a {
		case "--list":
			listOnly = true
			continue
		case "-d":
			if i+1 >= len(args) {
				return fmt.Errorf("unzip: option '-d' requires an argument")
			}
			i++
			destDir = args[i]
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'l':
				listOnly = true
			case 'o':
				// overwrite without prompting — default
			case 'q':
				// quiet
			case 'v':
				// verbose — no-op
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("unzip: invalid option -- '%s'", unknown)
		}
	}

	if len(positional) == 0 {
		return fmt.Errorf("unzip: usage: unzip [-l] [-d dir] archive.zip [files...]")
	}

	archivePath := sc.ResolvePath(positional[0])
	var filterFiles map[string]bool
	if len(positional) > 1 {
		filterFiles = make(map[string]bool)
		for _, f := range positional[1:] {
			filterFiles[f] = true
		}
	}

	data, err := readVirtualFile(sc, archivePath)
	if err != nil {
		return fmt.Errorf("unzip: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("unzip: %w", err)
	}

	if listOnly {
		fmt.Fprintf(hc.Stdout, "  Length      Date    Time    Name\n")
		fmt.Fprintf(hc.Stdout, "---------  ---------- -----   ----\n")
		total := int64(0)
		for _, f := range zr.File {
			total += int64(f.UncompressedSize64)
			t := f.Modified
			fmt.Fprintf(hc.Stdout, "%9d  %04d-%02d-%02d %02d:%02d   %s\n",
				f.UncompressedSize64,
				t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(),
				f.Name)
		}
		fmt.Fprintf(hc.Stdout, "---------                     -------\n")
		fmt.Fprintf(hc.Stdout, "%9d                     %d files\n", total, len(zr.File))
		return nil
	}

	base := "/"
	if destDir != "" {
		base = sc.ResolvePath(destDir)
		if err := sc.FS.MkdirAll(base, 0o755); err != nil {
			return fmt.Errorf("unzip: %w", err)
		}
	}

	for _, f := range zr.File {
		if filterFiles != nil && !filterFiles[f.Name] {
			continue
		}
		// sanitise path
		name := filepath.Clean(f.Name)
		if strings.HasPrefix(name, "..") {
			continue
		}
		target := base + "/" + name

		if f.FileInfo().IsDir() {
			sc.FS.MkdirAll(target, os.FileMode(f.Mode())|0o700)
			continue
		}
		if err := sc.FS.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("unzip: %w", err)
		}
		out, err := sc.FS.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(f.Mode())|0o600)
		if err != nil {
			return fmt.Errorf("unzip: %w", err)
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return fmt.Errorf("unzip: %w", err)
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if copyErr != nil {
			return fmt.Errorf("unzip: %w", copyErr)
		}
	}
	return nil
}

func zipAddFile(zw *zip.Writer, sc plugins.ShellContext, abs, name string, method uint16) error {
	data, err := readVirtualFile(sc, abs)
	if err != nil {
		return err
	}
	info, _ := sc.FS.Stat(abs)
	fh, _ := zip.FileInfoHeader(info)
	fh.Name = name
	fh.Method = method
	w, err := zw.CreateHeader(fh)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func zipAddDir(zw *zip.Writer, sc plugins.ShellContext, abs, name string, method uint16) error {
	entries, err := afero.ReadDir(sc.FS, abs)
	if err != nil {
		return err
	}
	for _, e := range entries {
		childAbs := abs + "/" + e.Name()
		childName := name + "/" + e.Name()
		if e.IsDir() {
			if err := zipAddDir(zw, sc, childAbs, childName, method); err != nil {
				return err
			}
		} else {
			if err := zipAddFile(zw, sc, childAbs, childName, method); err != nil {
				return err
			}
		}
	}
	return nil
}

func zipCopyEntry(zw *zip.Writer, src *zip.File) error {
	w, err := zw.CreateHeader(&src.FileHeader)
	if err != nil {
		return err
	}
	rc, err := src.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(w, rc)
	return err
}

// ensure ZipPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = ZipPlugin{}
