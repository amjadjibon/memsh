package tests

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/amjadjibon/memsh/pkg/shell"
	"github.com/spf13/afero"
)

// freePort returns a random available TCP port on localhost.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestNc(t *testing.T) {
	t.Run("client connects and receives data", func(t *testing.T) {
		// Start a real TCP listener that sends a greeting and closes.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		addr := ln.Addr().(*net.TCPAddr)
		go func() {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			io.WriteString(conn, "hello from server\n")
		}()
		defer ln.Close()

		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		cmd := fmt.Sprintf("nc -w 2 127.0.0.1 %d", addr.Port)
		_ = s.Run(ctx, cmd)

		if !strings.Contains(buf.String(), "hello from server") {
			t.Errorf("expected server greeting, got %q", buf.String())
		}
	})

	t.Run("listen mode accepts one connection", func(t *testing.T) {
		port := freePort(t)

		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		// Run nc -l in background; connect a client after a short delay.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		go func() {
			time.Sleep(100 * time.Millisecond)
			conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				return
			}
			io.WriteString(conn, "from client\n")
			conn.Close()
		}()

		cmd := fmt.Sprintf("nc -l -w 2 127.0.0.1 %d", port)
		_ = s.Run(ctx, cmd)

		if !strings.Contains(buf.String(), "from client") {
			t.Errorf("expected client data, got %q", buf.String())
		}
	})

	t.Run("-z port scan reports open port", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		defer ln.Close()

		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		cmd := fmt.Sprintf("nc -z -w 1 127.0.0.1 %d", port)
		if err := s.Run(ctx, cmd); err != nil {
			t.Errorf("expected exit 0 for open port, got %v", err)
		}
		if !strings.Contains(buf.String(), "open") {
			t.Errorf("expected 'open' in output, got %q", buf.String())
		}
	})

	t.Run("-z port scan reports closed port via exit code", func(t *testing.T) {
		port := freePort(t) // Nothing listening on this port.

		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		cmd := fmt.Sprintf("nc -z -w 1 127.0.0.1 %d", port)
		if err := s.Run(ctx, cmd); err == nil {
			t.Error("expected non-zero exit for closed port")
		}
	})

	t.Run("-z port range scans multiple ports", func(t *testing.T) {
		// Open one port in the range.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		openPort := ln.Addr().(*net.TCPAddr).Port
		defer ln.Close()

		// Build a range that includes openPort.
		lo := openPort - 2
		if lo < 1 {
			lo = 1
		}
		hi := openPort + 2

		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cmd := fmt.Sprintf("nc -z -w 1 127.0.0.1 %d-%d", lo, hi)
		_ = s.Run(ctx, cmd)

		if !strings.Contains(buf.String(), fmt.Sprintf("port %d", openPort)) {
			t.Errorf("expected open port %d in output, got %q", openPort, buf.String())
		}
	})

	t.Run("missing host/port returns error", func(t *testing.T) {
		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))
		if err := s.Run(context.Background(), "nc"); err == nil {
			t.Error("expected error for missing operands")
		}
	})

	t.Run("client sends data to server", func(t *testing.T) {
		received := make(chan string, 1)
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		addr := ln.Addr().(*net.TCPAddr)
		go func() {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			data, _ := io.ReadAll(conn)
			received <- string(data)
		}()
		defer ln.Close()

		var buf strings.Builder
		s := NewTestShell(t, &buf, shell.WithFS(afero.NewMemMapFs()))

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		cmd := fmt.Sprintf(`echo "hello server" | nc -w 1 127.0.0.1 %d`, addr.Port)
		_ = s.Run(ctx, cmd)

		select {
		case data := <-received:
			if !strings.Contains(data, "hello server") {
				t.Errorf("server expected 'hello server', got %q", data)
			}
		case <-time.After(2 * time.Second):
			t.Error("server did not receive data in time")
		}
	})
}
