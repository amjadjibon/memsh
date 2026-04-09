package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell"
)

// newCurlTestServer returns a test HTTP server with a few endpoints:
//
//	GET  /hello          → "hello world"
//	POST /echo           → echoes request body
//	GET  /redirect       → 302 → /hello
//	GET  /headers        → dumps selected request headers
func newCurlTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	})
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sb.String()))
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/hello", http.StatusFound)
	})
	mux.HandleFunc("/headers", func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		auth := r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ua=" + ua + " auth=" + auth))
	})
	return newLoopbackHTTPServer(t, mux)
}

func TestCurl(t *testing.T) {
	ctx := context.Background()
	srv := newCurlTestServer(t)

	t.Run("GET request outputs body", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "curl "+srv.URL+"/hello"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := strings.TrimSpace(buf.String()); got != "hello world" {
			t.Errorf("got %q, want %q", got, "hello world")
		}
	})

	t.Run("POST with -d sends body", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `curl -X POST -d 'payload' `+srv.URL+`/echo`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := strings.TrimSpace(buf.String()); got != "payload" {
			t.Errorf("got %q, want %q", got, "payload")
		}
	})

	t.Run("-d implies POST", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `curl -d 'auto-post' `+srv.URL+`/echo`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := strings.TrimSpace(buf.String()); got != "auto-post" {
			t.Errorf("got %q, want %q", got, "auto-post")
		}
	})

	t.Run("-L follows redirect", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "curl -L "+srv.URL+"/redirect"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := strings.TrimSpace(buf.String()); got != "hello world" {
			t.Errorf("got %q, want %q", got, "hello world")
		}
	})

	t.Run("-o saves to virtual FS", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "curl -o /out.txt "+srv.URL+"/hello"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := afero.ReadFile(fs, "/out.txt")
		if err != nil {
			t.Fatalf("output file not created: %v", err)
		}
		if got := strings.TrimSpace(string(data)); got != "hello world" {
			t.Errorf("file content %q, want %q", got, "hello world")
		}
	})

	t.Run("-O saves to basename", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(fs))
		if err := s.Run(ctx, "curl -O "+srv.URL+"/hello"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := afero.ReadFile(fs, "/hello")
		if err != nil {
			t.Fatalf("output file not created: %v", err)
		}
		if got := strings.TrimSpace(string(data)); got != "hello world" {
			t.Errorf("file content %q, want %q", got, "hello world")
		}
	})

	t.Run("-H sets custom header", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `curl -A 'myagent' `+srv.URL+`/headers`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := buf.String(); !strings.Contains(got, "ua=myagent") {
			t.Errorf("expected user-agent in response, got %q", got)
		}
	})

	t.Run("-u sets basic auth", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, `curl -u user:pass `+srv.URL+`/headers`); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := buf.String(); !strings.Contains(got, "auth=Basic") {
			t.Errorf("expected Basic auth header, got %q", got)
		}
	})

	t.Run("-i includes response headers", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "curl -i "+srv.URL+"/hello"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := buf.String()
		if !strings.Contains(got, "HTTP/1.1 200") {
			t.Errorf("expected status line in output, got %q", got)
		}
		if !strings.Contains(got, "hello world") {
			t.Errorf("expected body in output, got %q", got)
		}
	})

	t.Run("-I HEAD request", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "curl -I "+srv.URL+"/hello"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := buf.String()
		if !strings.Contains(got, "HTTP/1.1 200") {
			t.Errorf("expected status line, got %q", got)
		}
		if strings.Contains(got, "hello world") {
			t.Errorf("HEAD should not return body, got %q", got)
		}
	})

	t.Run("pipe curl into wc", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(ctx, "curl "+srv.URL+"/hello | wc -c"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// "hello world" is 11 bytes
		if got := strings.TrimSpace(buf.String()); got != "11" {
			t.Errorf("got byte count %q, want 11", got)
		}
	})
}
