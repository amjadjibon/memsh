package native

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/shell/plugins"
)

// GzipPlugin compresses or decompresses files using gzip.
//
//	gzip file           compress → file.gz, remove original
//	gzip -d file.gz     decompress → file, remove .gz
//	gzip -k file        keep original
//	gzip -c file        write to stdout
//	gzip -l file.gz     list compressed file info
//	gunzip file.gz      decompress (alias for gzip -d)
type GzipPlugin struct{ decompress bool }

// Gzip returns a plugin registered as "gzip".
func Gzip() GzipPlugin { return GzipPlugin{decompress: false} }

// Gunzip returns a plugin registered as "gunzip".
func Gunzip() GzipPlugin { return GzipPlugin{decompress: true} }

func (g GzipPlugin) Name() string {
	if g.decompress {
		return "gunzip"
	}
	return "gzip"
}
func (g GzipPlugin) Description() string {
	if g.decompress {
		return "decompress gzip files"
	}
	return "compress or decompress files using gzip"
}
func (GzipPlugin) Usage() string {
	return "gzip [-d] [-k] [-c] [-l] [-1..-9] [file...]"
}

func (g GzipPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	decompress := g.decompress
	keep := false
	stdout := false
	listMode := false
	level := gzip.DefaultCompression
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
		switch a {
		case "--decompress", "--uncompress":
			decompress = true
			continue
		case "--keep":
			keep = true
			continue
		case "--stdout", "--to-stdout":
			stdout = true
			continue
		case "--list":
			listMode = true
			continue
		case "--best":
			level = gzip.BestCompression
			continue
		case "--fast":
			level = gzip.BestSpeed
			continue
		}
		// short flags (combined)
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'd':
				decompress = true
			case 'k':
				keep = true
			case 'c':
				stdout = true
			case 'l':
				listMode = true
			case '1':
				level = gzip.BestSpeed
			case '9':
				level = gzip.BestCompression
			case '2', '3', '4', '5', '6', '7', '8':
				level = int(c - '0')
			case 'f':
				// force — no-op (we always overwrite in virtual FS)
			case 'q':
				// quiet — no-op
			case 'v':
				// verbose — no-op for now
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("gzip: invalid option -- '%s'", unknown)
		}
	}

	// stdin → stdout when no files given
	if len(files) == 0 {
		if decompress {
			return gzipDecompressReader(hc.Stdin, hc.Stdout)
		}
		return gzipCompressReader(hc.Stdin, hc.Stdout, level)
	}

	for _, f := range files {
		abs := sc.ResolvePath(f)

		if listMode {
			if err := gzipList(hc, sc, abs, f); err != nil {
				fmt.Fprintf(hc.Stderr, "gzip: %v\n", err)
			}
			continue
		}

		if stdout {
			data, err := readVirtualFile(sc, abs)
			if err != nil {
				fmt.Fprintf(hc.Stderr, "gzip: %s: %v\n", f, err)
				continue
			}
			if decompress {
				if err := gzipDecompressReader(newBytesReader(data), hc.Stdout); err != nil {
					fmt.Fprintf(hc.Stderr, "gzip: %s: %v\n", f, err)
				}
			} else {
				if err := gzipCompressReader(newBytesReader(data), hc.Stdout, level); err != nil {
					fmt.Fprintf(hc.Stderr, "gzip: %s: %v\n", f, err)
				}
			}
			continue
		}

		if decompress {
			outName := gzipDecompressedName(f)
			outAbs := sc.ResolvePath(outName)
			data, err := readVirtualFile(sc, abs)
			if err != nil {
				fmt.Fprintf(hc.Stderr, "gzip: %s: %v\n", f, err)
				continue
			}
			outFile, err := sc.FS.Create(outAbs)
			if err != nil {
				fmt.Fprintf(hc.Stderr, "gzip: %s: %v\n", outName, err)
				continue
			}
			writeErr := gzipDecompressReader(newBytesReader(data), outFile)
			outFile.Close()
			if writeErr != nil {
				fmt.Fprintf(hc.Stderr, "gzip: %s: %v\n", f, writeErr)
				continue
			}
			if !keep {
				sc.FS.Remove(abs)
			}
		} else {
			// compress
			if !strings.HasSuffix(f, ".gz") {
				outName := f + ".gz"
				outAbs := sc.ResolvePath(outName)
				data, err := readVirtualFile(sc, abs)
				if err != nil {
					fmt.Fprintf(hc.Stderr, "gzip: %s: %v\n", f, err)
					continue
				}
				outFile, err := sc.FS.Create(outAbs)
				if err != nil {
					fmt.Fprintf(hc.Stderr, "gzip: %s: %v\n", outName, err)
					continue
				}
				writeErr := gzipCompressReader(newBytesReader(data), outFile, level)
				outFile.Close()
				if writeErr != nil {
					fmt.Fprintf(hc.Stderr, "gzip: %s: %v\n", f, writeErr)
					continue
				}
				if !keep {
					sc.FS.Remove(abs)
				}
			} else {
				fmt.Fprintf(hc.Stderr, "gzip: %s already has .gz suffix\n", f)
			}
		}
	}
	return nil
}

func gzipCompressReader(r io.Reader, w io.Writer, level int) error {
	gw, err := gzip.NewWriterLevel(w, level)
	if err != nil {
		return err
	}
	if _, err := io.Copy(gw, r); err != nil {
		return err
	}
	return gw.Close()
}

func gzipDecompressReader(r io.Reader, w io.Writer) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("not a gzip file: %w", err)
	}
	defer gr.Close()
	_, err = io.Copy(w, gr)
	return err
}

func gzipDecompressedName(name string) string {
	switch {
	case strings.HasSuffix(name, ".tar.gz"):
		return strings.TrimSuffix(name, ".gz")
	case strings.HasSuffix(name, ".tgz"):
		return strings.TrimSuffix(name, ".tgz") + ".tar"
	case strings.HasSuffix(name, ".gz"):
		return strings.TrimSuffix(name, ".gz")
	default:
		return name
	}
}

func gzipList(hc interp.HandlerContext, sc plugins.ShellContext, abs, name string) error {
	data, err := readVirtualFile(sc, abs)
	if err != nil {
		return err
	}
	gr, err := gzip.NewReader(newBytesReader(data))
	if err != nil {
		return fmt.Errorf("not a gzip file")
	}
	defer gr.Close()
	// drain to get uncompressed size
	n, _ := io.Copy(io.Discard, gr)
	fmt.Fprintf(hc.Stdout, "%10d %10d %5.1f%% %s\n",
		int64(len(data)), n,
		100*(1-float64(len(data))/float64(max64(n, 1))),
		name)
	return nil
}

func readVirtualFile(sc plugins.ShellContext, abs string) ([]byte, error) {
	f, err := sc.FS.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// ensure GzipPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = GzipPlugin{}
