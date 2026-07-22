package ssh

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/amjadjibon/memsh/internal/session"
	"github.com/amjadjibon/memsh/pkg/shell"
)

// fakeAddr is a minimal net.Addr for test sessions.
type fakeAddr struct{}

func (fakeAddr) Network() string { return "tcp" }
func (fakeAddr) String() string  { return "127.0.0.1:0" }

// fakeContext is a minimal gliderssh.Context for tests.
type fakeContext struct {
	context.Context
	sync.Mutex
	user string
}

func (c *fakeContext) User() string          { return c.user }
func (c *fakeContext) SessionID() string     { return "test-session" }
func (c *fakeContext) ClientVersion() string { return "test-client" }
func (c *fakeContext) ServerVersion() string { return "test-server" }
func (c *fakeContext) RemoteAddr() net.Addr  { return fakeAddr{} }
func (c *fakeContext) LocalAddr() net.Addr   { return fakeAddr{} }
func (c *fakeContext) Permissions() *gliderssh.Permissions {
	return &gliderssh.Permissions{Permissions: &gossh.Permissions{}}
}
func (c *fakeContext) SetValue(key, value interface{}) {}

// fakeSession is a minimal gliderssh.Session implementation backed by an
// in-memory reader/writer, used to drive handleSession end-to-end without a
// real network connection.
type fakeSession struct {
	in       io.Reader
	out      *bytes.Buffer
	stderr   *bytes.Buffer
	user     string
	command  []string
	ctx      gliderssh.Context
	exitCode int
	exited   bool
	mu       sync.Mutex
}

func newFakeSession(user string, input string, command []string) *fakeSession {
	return &fakeSession{
		in:      strings.NewReader(input),
		out:     &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
		user:    user,
		command: command,
		ctx:     &fakeContext{Context: context.Background(), user: user},
	}
}

func (s *fakeSession) Read(p []byte) (int, error)  { return s.in.Read(p) }
func (s *fakeSession) Write(p []byte) (int, error) { return s.out.Write(p) }
func (s *fakeSession) Close() error                { return nil }
func (s *fakeSession) CloseWrite() error           { return nil }
func (s *fakeSession) SendRequest(name string, wantReply bool, payload []byte) (bool, error) {
	return false, nil
}
func (s *fakeSession) Stderr() io.ReadWriter { return stderrRW{s.stderr} }

type stderrRW struct{ buf *bytes.Buffer }

func (rw stderrRW) Read(p []byte) (int, error)  { return rw.buf.Read(p) }
func (rw stderrRW) Write(p []byte) (int, error) { return rw.buf.Write(p) }

func (s *fakeSession) User() string         { return s.user }
func (s *fakeSession) RemoteAddr() net.Addr { return fakeAddr{} }
func (s *fakeSession) LocalAddr() net.Addr  { return fakeAddr{} }
func (s *fakeSession) Environ() []string    { return nil }
func (s *fakeSession) Exit(code int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exitCode = code
	s.exited = true
	return nil
}
func (s *fakeSession) Command() []string              { return s.command }
func (s *fakeSession) RawCommand() string             { return strings.Join(s.command, " ") }
func (s *fakeSession) Subsystem() string              { return "" }
func (s *fakeSession) PublicKey() gliderssh.PublicKey { return nil }
func (s *fakeSession) Context() gliderssh.Context     { return s.ctx }
func (s *fakeSession) Permissions() gliderssh.Permissions {
	return gliderssh.Permissions{Permissions: &gossh.Permissions{}}
}
func (s *fakeSession) Pty() (gliderssh.Pty, <-chan gliderssh.Window, bool) {
	return gliderssh.Pty{}, nil, false
}
func (s *fakeSession) Signals(c chan<- gliderssh.Signal) {}
func (s *fakeSession) Break(c chan<- bool)               {}

var _ gliderssh.Session = (*fakeSession)(nil)

func testBaseOpts() []shell.Option {
	return []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)}
}

func TestHandleSessionExecSingleCommand(t *testing.T) {
	store := session.New(context.Background(), time.Hour, 0)
	sess := newFakeSession("execuser1", "", []string{"echo", "hello from exec"})

	handleSession(sess, store, testBaseOpts(), 5*time.Second, 5*time.Second, session.Limits{})

	if !sess.exited {
		t.Fatal("expected Exit to be called")
	}
	if sess.exitCode != 0 {
		t.Errorf("exitCode = %d, want 0; stderr=%q", sess.exitCode, sess.stderr.String())
	}
	if !strings.Contains(sess.out.String(), "hello from exec") {
		t.Errorf("output = %q, want it to contain the echoed text", sess.out.String())
	}
}

func TestHandleSessionExecCommandFailure(t *testing.T) {
	store := session.New(context.Background(), time.Hour, 0)
	sess := newFakeSession("execuser2", "", []string{"nonexistent-cmd-xyz"})

	handleSession(sess, store, testBaseOpts(), 5*time.Second, 5*time.Second, session.Limits{})

	if !sess.exited {
		t.Fatal("expected Exit to be called")
	}
	if sess.exitCode != 1 {
		t.Errorf("exitCode = %d, want 1 for unknown command", sess.exitCode)
	}
}

func TestHandleSessionExecMaxSessionsReached(t *testing.T) {
	store := session.New(context.Background(), time.Hour, 1)
	// Fill the only slot with a different session first.
	store.Get("existing-session")

	sess := newFakeSession("brand-new-user", "", []string{"echo", "hi"})
	handleSession(sess, store, testBaseOpts(), 5*time.Second, 5*time.Second, session.Limits{})

	if !sess.exited || sess.exitCode != 1 {
		t.Errorf("exitCode = %d, exited = %v, want 1/true", sess.exitCode, sess.exited)
	}
	if !strings.Contains(sess.stderr.String(), "maximum number of sessions") {
		t.Errorf("stderr = %q, want max-sessions message", sess.stderr.String())
	}
}

func TestHandleSessionExecRuntimeLimitExceeded(t *testing.T) {
	store := session.New(context.Background(), time.Hour, 0)
	entry, ok := store.Get("runtime-limited-user")
	if !ok {
		t.Fatal("Get failed")
	}
	entry.Runtime = time.Hour // already over any small limit

	sess := newFakeSession("runtime-limited-user", "", []string{"echo", "hi"})
	handleSession(sess, store, testBaseOpts(), 5*time.Second, 5*time.Second, session.Limits{MaxRuntime: time.Second})

	if !sess.exited || sess.exitCode != 1 {
		t.Errorf("exitCode = %d, exited = %v, want 1/true", sess.exitCode, sess.exited)
	}
	if !strings.Contains(sess.stderr.String(), "runtime limit exceeded") {
		t.Errorf("stderr = %q, want runtime limit message", sess.stderr.String())
	}
}

func TestHandleSessionInteractiveREPL(t *testing.T) {
	store := session.New(context.Background(), time.Hour, 0)
	// Interactive: Command() is empty, input drives a line-based REPL.
	input := "echo repl-hello\nexit\n"
	sess := newFakeSession("repluser", input, nil)

	handleSession(sess, store, testBaseOpts(), 5*time.Second, 5*time.Second, session.Limits{})

	out := sess.out.String()
	if !strings.Contains(out, "memsh:") {
		t.Errorf("output = %q, want a memsh prompt", out)
	}
	if !strings.Contains(out, "repl-hello") {
		t.Errorf("output = %q, want echoed command output", out)
	}
	if !strings.Contains(out, "logout") {
		t.Errorf("output = %q, want logout message on exit", out)
	}
}

func TestHandleSessionInteractiveEOF(t *testing.T) {
	store := session.New(context.Background(), time.Hour, 0)
	// No trailing newline / Ctrl-D equivalent: sshReadLine returns an error
	// immediately, so the REPL loop should exit cleanly via the outer break.
	sess := newFakeSession("eofuser", "", nil)

	handleSession(sess, store, testBaseOpts(), 5*time.Second, 5*time.Second, session.Limits{})

	if !sess.exited || sess.exitCode != 0 {
		t.Errorf("exitCode = %d, exited = %v, want 0/true", sess.exitCode, sess.exited)
	}
}

func TestHandleSessionDefaultUserGetsGeneratedID(t *testing.T) {
	store := session.New(context.Background(), time.Hour, 0)
	sess := newFakeSession("memsh", "", []string{"pwd"})

	handleSession(sess, store, testBaseOpts(), 5*time.Second, 5*time.Second, session.Limits{})

	if !sess.exited || sess.exitCode != 0 {
		t.Errorf("exitCode = %d, exited = %v, want 0/true; stderr=%q", sess.exitCode, sess.exited, sess.stderr.String())
	}
	// A random session ID should have been minted rather than reusing "memsh".
	if store.Count() != 1 {
		t.Errorf("store.Count() = %d, want 1", store.Count())
	}
}
