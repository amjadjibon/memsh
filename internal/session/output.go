package session

import (
	"fmt"
	"strings"
	"sync"
)

// LimitedBuffer is a strings.Builder-like writer that stops accepting data
// after max bytes, so a single command cannot buffer unbounded output in
// server memory. Writes past the limit fail; String() reports a truncation
// marker.
type LimitedBuffer struct {
	mu        sync.Mutex
	b         strings.Builder
	max       int
	truncated bool
}

// NewLimitedBuffer returns a buffer that holds at most max bytes (max <= 0
// means unlimited).
func NewLimitedBuffer(max int) *LimitedBuffer {
	return &LimitedBuffer{max: max}
}

func (l *LimitedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.max <= 0 {
		return l.b.Write(p)
	}
	room := l.max - l.b.Len()
	if room <= 0 {
		l.truncated = true
		return 0, fmt.Errorf("output limit of %d bytes exceeded", l.max)
	}
	if len(p) > room {
		l.b.Write(p[:room])
		l.truncated = true
		return room, fmt.Errorf("output limit of %d bytes exceeded", l.max)
	}
	return l.b.Write(p)
}

// String returns the buffered output, with a note appended if it was cut.
func (l *LimitedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.truncated {
		return l.b.String() + fmt.Sprintf("\n[output truncated at %d bytes]\n", l.max)
	}
	return l.b.String()
}

// Reset clears the buffer.
func (l *LimitedBuffer) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b.Reset()
	l.truncated = false
}
