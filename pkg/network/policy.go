package network

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// Mode controls outbound networking behavior.
type Mode string

const (
	// ModeOff blocks all outbound networking.
	ModeOff Mode = "off"
	// ModeAllowlist permits outbound networking only to allowlisted destinations.
	ModeAllowlist Mode = "allowlist"
	// ModeFull permits outbound networking without destination restrictions.
	ModeFull Mode = "full"
)

// Policy defines destination-level networking restrictions.
type Policy struct {
	Mode              Mode
	AllowDomains      []string
	AllowCIDRs        []netip.Prefix
	AllowPorts        map[int]struct{}
	DenyPrivateRanges bool
}

// DefaultPolicy returns a permissive policy for backward compatibility.
func DefaultPolicy() Policy {
	return Policy{Mode: ModeFull}
}

// Resolver resolves hostnames to IP addresses.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type stdResolver struct{}

func (stdResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, network, host)
}

func normalizePort(port string) (int, error) {
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("invalid destination port %q", port)
	}
	return p, nil
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	return host
}

func isPrivateOrReserved(addr netip.Addr) bool {
	return addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified()
}

func domainAllowed(host string, allow []string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, raw := range allow {
		rule := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(raw, ".")))
		if rule == "" {
			continue
		}
		if strings.HasPrefix(rule, "*.") {
			suffix := strings.TrimPrefix(rule, "*.")
			if suffix == "" {
				continue
			}
			if host == suffix || strings.HasSuffix(host, "."+suffix) {
				return true
			}
			continue
		}
		if host == rule {
			return true
		}
	}
	return false
}

func cidrAllowed(addr netip.Addr, allow []netip.Prefix) bool {
	for _, p := range allow {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func portAllowed(port int, allow map[int]struct{}) bool {
	if len(allow) == 0 {
		return true
	}
	_, ok := allow[port]
	return ok
}

func splitAddress(address string) (host string, port int, err error) {
	h, p, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, fmt.Errorf("invalid destination %q: %w", address, err)
	}
	host = normalizeHost(h)
	port, err = normalizePort(p)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

// EvaluateAddress validates destination host:port against policy and returns
// the allowed resolved IP to dial.
func (p Policy) EvaluateAddress(ctx context.Context, address string, resolver Resolver) (netip.Addr, int, string, error) {
	if resolver == nil {
		resolver = stdResolver{}
	}
	host, port, err := splitAddress(address)
	if err != nil {
		return netip.Addr{}, 0, "", err
	}

	if p.Mode == "" {
		p.Mode = ModeFull
	}
	if p.Mode == ModeOff {
		return netip.Addr{}, 0, "", fmt.Errorf("network disabled by policy")
	}
	if !portAllowed(port, p.AllowPorts) {
		return netip.Addr{}, 0, "", fmt.Errorf("destination port %d is not allowed", port)
	}

	// Fast path for unrestricted mode.
	if p.Mode == ModeFull && !p.DenyPrivateRanges {
		return netip.Addr{}, port, host, nil
	}

	// Handle IP-literal destination.
	if ip, err := netip.ParseAddr(host); err == nil {
		if p.DenyPrivateRanges && !cidrAllowed(ip, p.AllowCIDRs) && isPrivateOrReserved(ip) {
			return netip.Addr{}, 0, "", fmt.Errorf("destination ip %s is blocked by private-range policy", ip.String())
		}
		if p.Mode == ModeAllowlist && !cidrAllowed(ip, p.AllowCIDRs) {
			return netip.Addr{}, 0, "", fmt.Errorf("destination ip %s is not in allowed cidrs", ip.String())
		}
		return ip, port, host, nil
	}

	allowedByDomain := domainAllowed(host, p.AllowDomains)

	ips, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return netip.Addr{}, 0, "", fmt.Errorf("dns lookup failed for %s: %w", host, err)
	}
	if len(ips) == 0 {
		return netip.Addr{}, 0, "", fmt.Errorf("dns lookup returned no addresses for %s", host)
	}

	var chosen netip.Addr
	for _, ip := range ips {
		if !ip.IsValid() {
			continue
		}
		if p.DenyPrivateRanges && !cidrAllowed(ip, p.AllowCIDRs) && isPrivateOrReserved(ip) {
			return netip.Addr{}, 0, "", fmt.Errorf("dns resolved to disallowed ip %s", ip.String())
		}

		if p.Mode == ModeAllowlist {
			allowedByCIDR := cidrAllowed(ip, p.AllowCIDRs)
			if !allowedByDomain && !allowedByCIDR {
				return netip.Addr{}, 0, "", fmt.Errorf("dns resolved to ip %s outside allowlist", ip.String())
			}
		}
		if !chosen.IsValid() {
			chosen = ip
		}
	}

	// If no suitable address chosen, return explicit denial.
	if !chosen.IsValid() {
		return netip.Addr{}, 0, "", fmt.Errorf("no allowed addresses found for %s", host)
	}
	return chosen, port, host, nil
}
