package tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// startScriptServer starts an httptest.Server that serves the given scripts
// keyed by path, e.g. {"/hello.sh": "echo hello"}.
func startScriptServer(t *testing.T, scripts map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range scripts {
		body := body // capture
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, body)
		})
	}
	return newLoopbackHTTPServer(t, mux)
}

func TestSourceURL(t *testing.T) {
	srv := startScriptServer(t, map[string]string{
		"/hello.sh": "echo hello_from_url",
	})

	var buf strings.Builder
	s := NewTestShell(t, &buf)
	defer s.Close()

	err := s.Run(context.Background(), fmt.Sprintf("source %s/hello.sh", srv.URL))
	if err != nil {
		t.Fatalf("source URL: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "hello_from_url" {
		t.Errorf("output = %q, want %q", got, "hello_from_url")
	}
}

func TestSourceURLDotAlias(t *testing.T) {
	srv := startScriptServer(t, map[string]string{
		"/vars.sh": "X=42",
	})

	var buf strings.Builder
	s := NewTestShell(t, &buf)
	defer s.Close()

	ctx := context.Background()
	// `. url` is the POSIX alias for `source url`
	err := s.Run(ctx, fmt.Sprintf(". %s/vars.sh && echo $X", srv.URL))
	if err != nil {
		t.Fatalf(". URL: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "42" {
		t.Errorf("output = %q, want 42", got)
	}
}

func TestSourceURLSetsVariables(t *testing.T) {
	srv := startScriptServer(t, map[string]string{
		"/env.sh": `
GREETING=hello
TARGET=world
`,
	})

	var buf strings.Builder
	s := NewTestShell(t, &buf)
	defer s.Close()

	ctx := context.Background()
	err := s.Run(ctx, fmt.Sprintf(`source %s/env.sh && echo "$GREETING $TARGET"`, srv.URL))
	if err != nil {
		t.Fatalf("source URL: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "hello world" {
		t.Errorf("output = %q, want %q", got, "hello world")
	}
}

func TestSourceURLDefinesFunction(t *testing.T) {
	srv := startScriptServer(t, map[string]string{
		"/lib.sh": `greet() { echo "hi $1"; }`,
	})

	var buf strings.Builder
	s := NewTestShell(t, &buf)
	defer s.Close()

	ctx := context.Background()
	err := s.Run(ctx, fmt.Sprintf("source %s/lib.sh && greet world", srv.URL))
	if err != nil {
		t.Fatalf("source URL: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "hi world" {
		t.Errorf("output = %q, want %q", got, "hi world")
	}
}

func TestSourceURLNotFound(t *testing.T) {
	srv := startScriptServer(t, map[string]string{})

	var buf strings.Builder
	s := NewTestShell(t, &buf)
	defer s.Close()

	// A 404 response should cause source to fail (non-nil error or non-zero exit).
	// mvdan.cc/sh's internal source handler wraps the open error as "exit status 1"
	// rather than propagating the message, so we only check that it fails.
	err := s.Run(context.Background(), fmt.Sprintf("source %s/missing.sh", srv.URL))
	if err == nil {
		t.Error("expected error for missing URL, got nil")
	}
}

func TestSourceURLChained(t *testing.T) {
	// Script A sources script B from the same server.
	srv := startScriptServer(t, map[string]string{})
	// We need the server URL before building the scripts, so set up dynamically.
	scriptB := "MSG=from_b"
	var srvReal *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/b.sh", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, scriptB)
	})
	mux.HandleFunc("/a.sh", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "source %s/b.sh", srvReal.URL)
	})
	srvReal = newLoopbackHTTPServer(t, mux)
	_ = srv // unused placeholder, srvReal is the real server

	var buf strings.Builder
	s := NewTestShell(t, &buf)
	defer s.Close()

	ctx := context.Background()
	err := s.Run(ctx, fmt.Sprintf("source %s/a.sh && echo $MSG", srvReal.URL))
	if err != nil {
		t.Fatalf("chained source: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "from_b" {
		t.Errorf("output = %q, want from_b", got)
	}
}

func TestSourceURLMixedWithFile(t *testing.T) {
	srv := startScriptServer(t, map[string]string{
		"/remote.sh": "REMOTE=yes",
	})

	var buf strings.Builder
	s := NewTestShell(t, &buf)
	defer s.Close()

	ctx := context.Background()
	// Write a local file to the virtual FS, then source both.
	err := s.Run(ctx, fmt.Sprintf(`
echo 'LOCAL=yes' > /local.sh
source /local.sh
source %s/remote.sh
echo "$LOCAL $REMOTE"
`, srv.URL))
	if err != nil {
		t.Fatalf("mixed source: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "yes yes" {
		t.Errorf("output = %q, want %q", got, "yes yes")
	}
}
