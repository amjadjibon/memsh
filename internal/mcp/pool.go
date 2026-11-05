package mcp

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/internal/session"
	"github.com/amjadjibon/memsh/pkg/shell"
)

// sessionGrace is how long a freshly created server may have zero sessions
// before the pool reaps it. The SDK calls getServer before the session is
// bound, so a brand-new server is briefly sessionless.
const sessionGrace = time.Minute

// MaxOutputSize caps the output buffered for a single tool call.
const MaxOutputSize = 10 << 20

// Pool hands out one isolated shell-backed MCP server per client session for
// the HTTP and SSE transports, caps how many exist at once, and closes each
// shell once its session has gone away.
type Pool struct {
	mu          sync.Mutex
	entries     []*poolEntry
	maxSessions int
	limits      session.Limits
	timeout     time.Duration
	shellOpts   []shell.Option
	now         func() time.Time
}

type poolEntry struct {
	server  *mcp.Server
	sh      *shell.Shell
	created time.Time
}

// NewPool creates a pool. maxSessions <= 0 means unlimited. limits caps
// each session's virtual filesystem as it is written. shellOpts are applied
// to every shell; the pool adds its own WithFS and WithStdIO.
func NewPool(maxSessions int, limits session.Limits, timeout time.Duration, shellOpts ...shell.Option) *Pool {
	return &Pool{
		maxSessions: maxSessions,
		limits:      limits,
		timeout:     timeout,
		shellOpts:   shellOpts,
		now:         time.Now,
	}
}

// GetServer is the getServer callback for mcp.NewStreamableHTTPHandler and
// mcp.NewSSEHandler. It returns nil (which the SDK turns into an HTTP error)
// when the session cap is reached or the shell cannot be created.
func (p *Pool) GetServer(_ *http.Request) *mcp.Server {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sweepLocked()

	if p.maxSessions > 0 && len(p.entries) >= p.maxSessions {
		log.Printf("session cap reached (%d); rejecting new session", p.maxSessions)
		return nil
	}

	fs, err := p.limits.WrapFS(afero.NewMemMapFs())
	if err != nil {
		log.Printf("session fs init error: %v", err)
		return nil
	}
	buf := session.NewLimitedBuffer(MaxOutputSize)
	opts := make([]shell.Option, len(p.shellOpts), len(p.shellOpts)+2)
	copy(opts, p.shellOpts)
	opts = append(opts, shell.WithFS(fs), shell.WithStdIO(strings.NewReader(""), buf, buf))
	sh, err := shell.New(opts...)
	if err != nil {
		log.Printf("session shell init error: %v", err)
		return nil
	}
	srv := NewServer(sh, buf, p.timeout)
	p.entries = append(p.entries, &poolEntry{server: srv, sh: sh, created: p.now()})
	return srv
}

// Len reports how many servers the pool currently holds.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}

// Sweep closes shells whose sessions have ended.
func (p *Pool) Sweep() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sweepLocked()
}

func (p *Pool) sweepLocked() {
	kept := p.entries[:0]
	for _, e := range p.entries {
		if p.now().Sub(e.created) > sessionGrace && !hasSessions(e.server) {
			e.sh.Close()
			continue
		}
		kept = append(kept, e)
	}
	for i := len(kept); i < len(p.entries); i++ {
		p.entries[i] = nil
	}
	p.entries = kept
}

// Run sweeps periodically until ctx is done, then closes every shell.
func (p *Pool) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			p.Close()
			return
		case <-t.C:
			p.Sweep()
		}
	}
}

// Close closes every shell in the pool.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.entries {
		e.sh.Close()
	}
	p.entries = nil
}

func hasSessions(s *mcp.Server) bool {
	for range s.Sessions() {
		return true
	}
	return false
}
