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

func TestPolicyEvaluateAddress_ModeFull(t *testing.T) {
	p := Policy{Mode: ModeFull}
	_, port, host, err := p.EvaluateAddress(context.Background(), "example.com:443", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "example.com" {
		t.Errorf("host = %q, want example.com", host)
	}
	if port != 443 {
		t.Errorf("port = %d, want 443", port)
	}
}

func TestPolicyEvaluateAddress_EmptyModeDefaultsToFull(t *testing.T) {
	p := Policy{}
	_, _, _, err := p.EvaluateAddress(context.Background(), "example.com:443", nil)
	if err != nil {
		t.Fatalf("empty mode should default to full: %v", err)
	}
}

func TestPolicyEvaluateAddress_IPLiteral(t *testing.T) {
	p := Policy{Mode: ModeFull, DenyPrivateRanges: true}
	ip, port, host, err := p.EvaluateAddress(context.Background(), "1.2.3.4:8080", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ip.IsValid() {
		t.Error("expected valid IP")
	}
	if port != 8080 {
		t.Errorf("port = %d, want 8080", port)
	}
	if host != "1.2.3.4" {
		t.Errorf("host = %q, want 1.2.3.4", host)
	}
}

func TestPolicyEvaluateAddress_PrivateIPDenied(t *testing.T) {
	p := Policy{Mode: ModeFull, DenyPrivateRanges: true}
	_, _, _, err := p.EvaluateAddress(context.Background(), "127.0.0.1:80", nil)
	if err == nil {
		t.Fatal("expected error for private IP")
	}
}

func TestPolicyEvaluateAddress_AllowlistIPDenied(t *testing.T) {
	p := Policy{Mode: ModeAllowlist}
	_, _, _, err := p.EvaluateAddress(context.Background(), "1.2.3.4:443", nil)
	if err == nil {
		t.Fatal("expected error — IP not in allowlist CIDRs")
	}
}

func TestPolicyEvaluateAddress_AllowlistCIDR(t *testing.T) {
	p := Policy{
		Mode:       ModeAllowlist,
		AllowCIDRs: []netip.Prefix{netip.MustParsePrefix("93.184.216.0/24")},
	}
	_, _, _, err := p.EvaluateAddress(context.Background(), "93.184.216.34:443", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyEvaluateAddress_InvalidAddress(t *testing.T) {
	p := Policy{Mode: ModeFull}
	_, _, _, err := p.EvaluateAddress(context.Background(), "noport", nil)
	if err == nil {
		t.Fatal("expected error for invalid address")
	}
}

func TestPolicyEvaluateAddress_DNSLookupFail(t *testing.T) {
	p := Policy{Mode: ModeAllowlist, AllowDomains: []string{"unknown.example.com"}}
	r := fakeResolver{records: map[string][]netip.Addr{}}
	_, _, _, err := p.EvaluateAddress(context.Background(), "unknown.example.com:443", r)
	if err == nil {
		t.Fatal("expected error for DNS lookup failure")
	}
}

func TestPolicyEvaluateAddress_DenyPrivateDNS(t *testing.T) {
	p := Policy{Mode: ModeFull, DenyPrivateRanges: true}
	r := fakeResolver{
		records: map[string][]netip.Addr{
			"internal.local": {netip.MustParseAddr("10.0.0.1")},
		},
	}
	_, _, _, err := p.EvaluateAddress(context.Background(), "internal.local:443", r)
	if err == nil {
		t.Fatal("expected error for DNS-resolved private IP")
	}
}

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy()
	if p.Mode != ModeFull {
		t.Errorf("DefaultPolicy Mode = %q, want %q", p.Mode, ModeFull)
	}
}

func TestNormalizePort(t *testing.T) {
	tests := []struct {
		input string
		want  int
		ok    bool
	}{
		{"443", 443, true},
		{"80", 80, true},
		{"0", 0, false},
		{"65536", 0, false},
		{"abc", 0, false},
	}
	for _, tc := range tests {
		got, err := normalizePort(tc.input)
		if tc.ok && err != nil {
			t.Errorf("normalizePort(%q) error: %v", tc.input, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("normalizePort(%q) expected error", tc.input)
		}
		if got != tc.want {
			t.Errorf("normalizePort(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeHost(t *testing.T) {
	tests := []struct{ input, want string }{
		{"example.com", "example.com"},
		{"[::1]", "::1"},
		{"  host  ", "host"},
	}
	for _, tc := range tests {
		got := normalizeHost(tc.input)
		if got != tc.want {
			t.Errorf("normalizeHost(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDomainAllowed(t *testing.T) {
	tests := []struct {
		host   string
		allow  []string
		want   bool
	}{
		{"example.com", []string{"example.com"}, true},
		{"api.example.com", []string{"*.example.com"}, true},
		{"other.com", []string{"*.example.com"}, false},
		{"example.com.", []string{"example.com"}, true},
		{"", []string{"example.com"}, false},
	}
	for _, tc := range tests {
		got := domainAllowed(tc.host, tc.allow)
		if got != tc.want {
			t.Errorf("domainAllowed(%q, %v) = %v, want %v", tc.host, tc.allow, got, tc.want)
		}
	}
}

func TestPortAllowed(t *testing.T) {
	if !portAllowed(443, map[int]struct{}{443: {}}) {
		t.Error("443 should be allowed")
	}
	if portAllowed(80, map[int]struct{}{443: {}}) {
		t.Error("80 should not be allowed")
	}
	if !portAllowed(80, nil) {
		t.Error("empty allowlist should allow all ports")
	}
}
