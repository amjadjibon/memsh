package cmd

import (
	"testing"

	"github.com/amjadjibon/memsh/pkg/network"
)

func TestParseNetworkPolicy(t *testing.T) {
	p, err := parseNetworkPolicy(networkFlagConfig{
		Mode:         "allowlist",
		AllowDomains: []string{"*.example.com"},
		AllowCIDRs:   []string{"203.0.113.0/24"},
		AllowPorts:   []string{"443", "8443"},
	})
	if err != nil {
		t.Fatalf("parseNetworkPolicy: %v", err)
	}
	if p.Mode != network.ModeAllowlist {
		t.Fatalf("mode = %q, want %q", p.Mode, network.ModeAllowlist)
	}
	if len(p.AllowDomains) != 1 || p.AllowDomains[0] != "*.example.com" {
		t.Fatalf("allow domains = %#v", p.AllowDomains)
	}
	if len(p.AllowCIDRs) != 1 {
		t.Fatalf("allow cidrs = %#v", p.AllowCIDRs)
	}
	if _, ok := p.AllowPorts[443]; !ok {
		t.Fatalf("allow ports missing 443: %#v", p.AllowPorts)
	}
}

func TestParseNetworkPolicy_InvalidMode(t *testing.T) {
	_, err := parseNetworkPolicy(networkFlagConfig{Mode: "weird"})
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestParseNetworkPolicy_InvalidPort(t *testing.T) {
	_, err := parseNetworkPolicy(networkFlagConfig{
		Mode:       "allowlist",
		AllowPorts: []string{"70000"},
	})
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}
