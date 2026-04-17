package network

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestDialerBlocksPrivateWhenDenied(t *testing.T) {
	d := NewDialer(DialerConfig{
		Policy: Policy{
			Mode:              ModeAllowlist,
			DenyPrivateRanges: true,
		},
		Base: &net.Dialer{Timeout: 500 * time.Millisecond},
	})

	_, err := d.DialContext(context.Background(), "tcp", "127.0.0.1:80")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDialerAllowsExplicitCIDRAndPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_ = conn.Close()
	}()

	addr, err := netip.ParseAddr("127.0.0.1")
	if err != nil {
		t.Fatalf("parse addr: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	pfx := netip.PrefixFrom(addr, 8)

	d := NewDialer(DialerConfig{
		Policy: Policy{
			Mode:              ModeAllowlist,
			AllowCIDRs:        []netip.Prefix{pfx},
			AllowPorts:        map[int]struct{}{port: {}},
			DenyPrivateRanges: true,
		},
		Base: &net.Dialer{Timeout: 1 * time.Second},
	})

	conn, err := d.DialContext(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()
	<-done
}
