package network

import (
	"context"
	"net/netip"
	"testing"
)

type fakeResolver struct {
	records map[string][]netip.Addr
}

func (f fakeResolver) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	return f.records[host], nil
}

func TestPolicyEvaluateAddress_ModeOff(t *testing.T) {
	p := Policy{Mode: ModeOff}
	_, _, _, err := p.EvaluateAddress(context.Background(), "example.com:443", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPolicyEvaluateAddress_AllowlistDomain(t *testing.T) {
	p := Policy{
		Mode:              ModeAllowlist,
		AllowDomains:      []string{"*.example.com"},
		DenyPrivateRanges: true,
	}
	r := fakeResolver{
		records: map[string][]netip.Addr{
			"api.example.com": {netip.MustParseAddr("93.184.216.34")},
		},
	}

	_, port, host, err := p.EvaluateAddress(context.Background(), "api.example.com:443", r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "api.example.com" {
		t.Fatalf("host = %q, want api.example.com", host)
	}
	if port != 443 {
		t.Fatalf("port = %d, want 443", port)
	}
}

func TestPolicyEvaluateAddress_AllowlistMixedDNSDenied(t *testing.T) {
	p := Policy{
		Mode:              ModeAllowlist,
		AllowDomains:      []string{"api.example.com"},
		DenyPrivateRanges: true,
	}
	r := fakeResolver{
		records: map[string][]netip.Addr{
			"api.example.com": {
				netip.MustParseAddr("93.184.216.34"),
				netip.MustParseAddr("127.0.0.1"),
			},
		},
	}
	_, _, _, err := p.EvaluateAddress(context.Background(), "api.example.com:443", r)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPolicyEvaluateAddress_PortAllowlist(t *testing.T) {
	p := Policy{
		Mode:       ModeFull,
		AllowPorts: map[int]struct{}{443: {}},
	}
	_, _, _, err := p.EvaluateAddress(context.Background(), "example.com:80", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
