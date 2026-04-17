package network

import (
	"fmt"
	"sync"
	"time"
)

// Limits defines optional per-session network caps.
type Limits struct {
	MaxRequests      int
	MaxBytesSent     int64
	MaxBytesReceived int64
	MaxRuntime       time.Duration
}

// Usage captures consumed network resources.
type Usage struct {
	Requests      int
	BytesSent     int64
	BytesReceived int64
	Runtime       time.Duration
}

// Meter tracks and enforces network usage limits.
type Meter struct {
	mu     sync.Mutex
	limits Limits
	usage  Usage
}

func NewMeter(limits Limits) *Meter {
	return &Meter{limits: limits}
}

func NewMeterFromUsage(limits Limits, usage Usage) *Meter {
	return &Meter{
		limits: limits,
		usage:  usage,
	}
}

func (m *Meter) Snapshot() Usage {
	if m == nil {
		return Usage{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.usage
}

func (m *Meter) beforeRequest() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.limits.MaxRequests > 0 && m.usage.Requests >= m.limits.MaxRequests {
		return fmt.Errorf("session network request limit exceeded: used %d, max %d", m.usage.Requests, m.limits.MaxRequests)
	}
	m.usage.Requests++
	return nil
}

func (m *Meter) addSent(n int64) error {
	if m == nil || n <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usage.BytesSent += n
	if m.limits.MaxBytesSent > 0 && m.usage.BytesSent > m.limits.MaxBytesSent {
		return fmt.Errorf("session network sent-byte limit exceeded: %d bytes (max %d)", m.usage.BytesSent, m.limits.MaxBytesSent)
	}
	return nil
}

func (m *Meter) addReceived(n int64) error {
	if m == nil || n <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usage.BytesReceived += n
	if m.limits.MaxBytesReceived > 0 && m.usage.BytesReceived > m.limits.MaxBytesReceived {
		return fmt.Errorf("session network receive-byte limit exceeded: %d bytes (max %d)", m.usage.BytesReceived, m.limits.MaxBytesReceived)
	}
	return nil
}

func (m *Meter) addRuntime(d time.Duration) error {
	if m == nil || d <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usage.Runtime += d
	if m.limits.MaxRuntime > 0 && m.usage.Runtime > m.limits.MaxRuntime {
		return fmt.Errorf("session network runtime limit exceeded: used %s, max %s", m.usage.Runtime.Round(time.Millisecond), m.limits.MaxRuntime)
	}
	return nil
}
