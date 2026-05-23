package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

func TestGo(t *testing.T) {
	ctx := context.Background()

	// ── go run ────────────────────────────────────────────────────────────────

	t.Run("go run executes a .go file from virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/main.go", []byte(`package main
func main() { fmt.Println("hello from go run") }
`), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, `go run /main.go`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "hello from go run") {
			t.Errorf("expected output, got %q", buf.String())
		}
	})

	t.Run("go run with explicit import", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/counter.go", []byte(`package main
import "fmt"
func main() {
	for i := 1; i <= 3; i++ {
		fmt.Println(i)
	}
}
`), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, `go run /counter.go`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := strings.TrimSpace(buf.String())
		if out != "1\n2\n3" {
			t.Errorf("expected '1\\n2\\n3', got %q", out)
		}
	})

	t.Run("go run reports missing file", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `go run /nope.go`); err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	// ── go run via echo pipe ──────────────────────────────────────────────────

	t.Run("echo creates file and go run executes it", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		script := `
echo 'package main' > /hello.go
echo 'func main() { fmt.Println("created and run") }' >> /hello.go
go run /hello.go
`
		if err := s.Run(ctx, script); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "created and run") {
			t.Errorf("expected output, got %q", buf.String())
		}
	})

	// ── go test ───────────────────────────────────────────────────────────────

	t.Run("go test runs Test* functions from virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/math_test.go", []byte(`package main
import "testing"
func TestAdd(t *testing.T) {
	if 1+1 != 2 {
		t.Error("math is broken")
	}
}
`), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, `go test /`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "ok") {
			t.Errorf("expected 'ok', got %q", buf.String())
		}
	})

	t.Run("go test ./... recurses into subdirs", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		fs.MkdirAll("/pkg", 0o755)
		afero.WriteFile(fs, "/pkg/foo_test.go", []byte(`package main
import "testing"
func TestFoo(t *testing.T) {}
`), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, `go test ./...`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "ok") {
			t.Errorf("expected 'ok', got %q", buf.String())
		}
	})

	// ── go fmt ────────────────────────────────────────────────────────────────

	t.Run("go fmt formats Go source in virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/ugly.go", []byte(`package main
func main(){fmt.Println("hello")}
`), 0o644)
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, `go fmt /ugly.go`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, _ := afero.ReadFile(fs, "/ugly.go")
		if !strings.Contains(string(data), "func main() {") {
			t.Errorf("file was not formatted: %s", data)
		}
	})

	// ── go version ────────────────────────────────────────────────────────────

	t.Run("go version prints version string", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `go version`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "mvm") {
			t.Errorf("expected version string, got %q", buf.String())
		}
	})

	// ── stdin pipe ────────────────────────────────────────────────────────────

	t.Run("echo pipe to go executes source from stdin", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `echo 'fmt.Println("via stdin pipe")' | go`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "via stdin pipe") {
			t.Errorf("expected output, got %q", buf.String())
		}
	})
}
