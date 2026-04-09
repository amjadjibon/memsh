package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestTar(t *testing.T) {
	ctx := context.Background()

	// seed helpers
	seedFiles := func(fs afero.Fs) {
		afero.WriteFile(fs, "/src/a.txt", []byte("hello"), 0644)
		afero.WriteFile(fs, "/src/b.txt", []byte("world"), 0644)
	}

	t.Run("create and list tar", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		seedFiles(fs)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "tar -cf /arch.tar /src/a.txt /src/b.txt"); err != nil {
			t.Fatalf("create: %v", err)
		}
		buf.Reset()
		if err := s.Run(ctx, "tar -tf /arch.tar"); err != nil {
			t.Fatalf("list: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "a.txt") || !strings.Contains(out, "b.txt") {
			t.Errorf("list output missing files: %q", out)
		}
	})

	t.Run("create and extract tar", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		seedFiles(fs)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "tar -cf /arch.tar /src/a.txt /src/b.txt"); err != nil {
			t.Fatalf("create: %v", err)
		}
		fs.MkdirAll("/dest", 0755)
		if err := s.Run(ctx, "tar -xf /arch.tar -C /dest"); err != nil {
			t.Fatalf("extract: %v", err)
		}
		data, err := afero.ReadFile(fs, "/dest/src/a.txt")
		if err != nil {
			t.Fatalf("extracted file missing: %v", err)
		}
		if string(data) != "hello" {
			t.Errorf("got %q, want %q", data, "hello")
		}
	})

	t.Run("create and extract tar.gz", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		seedFiles(fs)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "tar -czf /arch.tar.gz /src/a.txt /src/b.txt"); err != nil {
			t.Fatalf("create gz: %v", err)
		}
		fs.MkdirAll("/dest2", 0755)
		if err := s.Run(ctx, "tar -xzf /arch.tar.gz -C /dest2"); err != nil {
			t.Fatalf("extract gz: %v", err)
		}
		data, err := afero.ReadFile(fs, "/dest2/src/b.txt")
		if err != nil {
			t.Fatalf("extracted gz file missing: %v", err)
		}
		if string(data) != "world" {
			t.Errorf("got %q, want %q", data, "world")
		}
	})

	t.Run("auto-detect gzip from extension", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		seedFiles(fs)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		// no -z flag — extension should trigger gzip
		if err := s.Run(ctx, "tar -cf /noflags.tgz /src/a.txt"); err != nil {
			t.Fatalf("create: %v", err)
		}
		buf.Reset()
		if err := s.Run(ctx, "tar -tf /noflags.tgz"); err != nil {
			t.Fatalf("list: %v", err)
		}
		if !strings.Contains(buf.String(), "a.txt") {
			t.Errorf("list output missing file: %q", buf.String())
		}
	})

	t.Run("verbose flag prints file names", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		seedFiles(fs)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "tar -cvf /v.tar /src/a.txt"); err != nil {
			t.Fatalf("create verbose: %v", err)
		}
		if !strings.Contains(buf.String(), "a.txt") {
			t.Errorf("verbose output missing filename: %q", buf.String())
		}
	})
}

func TestGzip(t *testing.T) {
	ctx := context.Background()

	t.Run("compress file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/hello.txt", []byte("hello gzip"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "gzip /hello.txt"); err != nil {
			t.Fatalf("gzip: %v", err)
		}
		if _, err := fs.Stat("/hello.txt.gz"); err != nil {
			t.Fatalf("compressed file not created: %v", err)
		}
		if _, err := fs.Stat("/hello.txt"); err == nil {
			t.Error("original file should be removed")
		}
	})

	t.Run("compress -k keeps original", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/data.txt", []byte("keep me"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "gzip -k /data.txt"); err != nil {
			t.Fatalf("gzip -k: %v", err)
		}
		if _, err := fs.Stat("/data.txt"); err != nil {
			t.Error("original should be kept with -k")
		}
		if _, err := fs.Stat("/data.txt.gz"); err != nil {
			t.Error("compressed file should be created")
		}
	})

	t.Run("decompress with gzip -d", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/msg.txt", []byte("round trip"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "gzip /msg.txt && gzip -d /msg.txt.gz"); err != nil {
			t.Fatalf("compress+decompress: %v", err)
		}
		data, err := afero.ReadFile(fs, "/msg.txt")
		if err != nil {
			t.Fatalf("decompressed file missing: %v", err)
		}
		if string(data) != "round trip" {
			t.Errorf("got %q, want %q", data, "round trip")
		}
	})

	t.Run("gunzip alias", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/alias.txt", []byte("gunzip test"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "gzip /alias.txt && gunzip /alias.txt.gz"); err != nil {
			t.Fatalf("gunzip: %v", err)
		}
		data, _ := afero.ReadFile(fs, "/alias.txt")
		if string(data) != "gunzip test" {
			t.Errorf("got %q", data)
		}
	})

	t.Run("gzip -c writes to stdout", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/pipe.txt", []byte("pipe content"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "gzip -c /pipe.txt > /piped.gz"); err != nil {
			t.Fatalf("gzip -c: %v", err)
		}
		if _, err := fs.Stat("/piped.gz"); err != nil {
			t.Fatalf("piped gz not created: %v", err)
		}
		// original untouched
		if _, err := fs.Stat("/pipe.txt"); err != nil {
			t.Error("original should be preserved with -c")
		}
	})

	t.Run("stdin to stdout", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/in.txt", []byte("stdin data"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "cat /in.txt | gzip | gunzip"); err != nil {
			t.Fatalf("pipe gzip/gunzip: %v", err)
		}
		if got := strings.TrimSpace(buf.String()); got != "stdin data" {
			t.Errorf("got %q, want %q", got, "stdin data")
		}
	})
}

func TestZip(t *testing.T) {
	ctx := context.Background()

	t.Run("create and list zip", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/a.txt", []byte("aaa"), 0644)
		afero.WriteFile(fs, "/b.txt", []byte("bbb"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "zip /out.zip /a.txt /b.txt"); err != nil {
			t.Fatalf("zip: %v", err)
		}
		buf.Reset()
		if err := s.Run(ctx, "unzip -l /out.zip"); err != nil {
			t.Fatalf("unzip -l: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "a.txt") || !strings.Contains(out, "b.txt") {
			t.Errorf("listing missing files: %q", out)
		}
	})

	t.Run("extract zip", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/src.txt", []byte("zip content"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "zip /arch.zip /src.txt"); err != nil {
			t.Fatalf("zip: %v", err)
		}
		fs.MkdirAll("/out", 0755)
		if err := s.Run(ctx, "unzip -d /out /arch.zip"); err != nil {
			t.Fatalf("unzip: %v", err)
		}
		data, err := afero.ReadFile(fs, "/out/src.txt")
		if err != nil {
			t.Fatalf("extracted file missing: %v", err)
		}
		if string(data) != "zip content" {
			t.Errorf("got %q, want %q", data, "zip content")
		}
	})

	t.Run("zip -r adds directory recursively", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/dir", 0755)
		afero.WriteFile(fs, "/dir/x.txt", []byte("x"), 0644)
		afero.WriteFile(fs, "/dir/y.txt", []byte("y"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "zip -r /rec.zip /dir"); err != nil {
			t.Fatalf("zip -r: %v", err)
		}
		buf.Reset()
		if err := s.Run(ctx, "unzip -l /rec.zip"); err != nil {
			t.Fatalf("unzip -l: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "x.txt") || !strings.Contains(out, "y.txt") {
			t.Errorf("recursive listing missing files: %q", out)
		}
	})

	t.Run("unzip extracts to cwd by default", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/f.txt", []byte("cwd extract"), 0644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))

		if err := s.Run(ctx, "zip /test.zip /f.txt && unzip /test.zip"); err != nil {
			t.Fatalf("zip+unzip: %v", err)
		}
		// file should be at /f.txt (re-extracted over itself or at root)
		data, err := afero.ReadFile(fs, "/f.txt")
		if err != nil {
			t.Fatalf("file missing after unzip: %v", err)
		}
		if string(data) != "cwd extract" {
			t.Errorf("got %q", data)
		}
	})
}
