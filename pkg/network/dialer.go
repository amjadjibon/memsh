package network

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"
)

type DialerConfig struct {
	Policy   Policy
	Resolver Resolver
	Base     *net.Dialer
	Meter    *Meter
}

// Dialer enforces policy before opening outbound connections.
type Dialer struct {
	policy   Policy
	resolver Resolver
	base     *net.Dialer
	meter    *Meter
}

func NewDialer(cfg DialerConfig) *Dialer {
	base := cfg.Base
	if base == nil {
		base = &net.Dialer{Timeout: 30 * time.Second}
	}
	p := cfg.Policy
	if p.Mode == "" {
		p = DefaultPolicy()
	}
	return &Dialer{
		policy:   p,
		resolver: cfg.Resolver,
		base:     base,
		meter:    cfg.Meter,
	}
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if d == nil {
		return nil, fmt.Errorf("network dialer is not configured")
	}
	if err := d.meter.beforeRequest(); err != nil {
		return nil, err
	}

	ip, port, host, err := d.policy.EvaluateAddress(ctx, address, d.resolver)
	if err != nil {
		return nil, err
	}

	target := address
	if ip.IsValid() {
		target = net.JoinHostPort(ip.String(), fmt.Sprintf("%d", port))
	}
	conn, err := d.base.DialContext(ctx, network, target)
	if err != nil {
		return nil, err
	}
	return &meteredConn{
		Conn:   conn,
		meter:  d.meter,
		host:   host,
		ip:     ip,
		start:  time.Now(),
		closed: false,
	}, nil
}

type meteredConn struct {
	net.Conn
	meter *Meter
	host  string
	ip    netip.Addr
	start time.Time

	mu     sync.Mutex
	closed bool
}

func (c *meteredConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if meterErr := c.meter.addReceived(int64(n)); meterErr != nil {
		_ = c.Conn.Close()
		return n, meterErr
	}
	return n, err
}

func (c *meteredConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if meterErr := c.meter.addSent(int64(n)); meterErr != nil {
		_ = c.Conn.Close()
		return n, meterErr
	}
	return n, err
}

func (c *meteredConn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	_ = c.meter.addRuntime(time.Since(c.start))
	return c.Conn.Close()
}
