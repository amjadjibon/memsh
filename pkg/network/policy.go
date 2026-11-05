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

// DefaultPolicy allows any public destination but denies loopback, private,
// link-local and other reserved ranges, so an embedded shell cannot be used
// to reach services on the host or its network (SSRF). Use WithNetworkPolicy
// with AllowCIDRs, or DenyPrivateRanges: false, to reach internal hosts.
func DefaultPolicy() Policy {
	return Policy{Mode: ModeFull, DenyPrivateRanges: true}
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

// reservedPrefixes are special-purpose ranges not covered by the netip
// predicates below. Several route to internal infrastructure in practice:
// CGNAT space (also used by cloud metadata services and tailnets), NAT64
// prefixes that embed an IPv4 address, and 6to4.
var reservedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range []string{
		"0.0.0.0/8",       // "this network"
		"100.64.0.0/10",   // shared address space (CGNAT)
		"192.0.0.0/24",    // IETF protocol assignments
		"192.0.2.0/24",    // TEST-NET-1
		"198.18.0.0/15",   // benchmarking
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"240.0.0.0/4",     // reserved, includes 255.255.255.255
		"64:ff9b::/96",    // NAT64 well-known prefix
		"64:ff9b:1::/48",  // local-use NAT64
		"100::/64",        // discard-only
		"2001:db8::/32",   // documentation
		"2002::/16",       // 6to4 (embeds an IPv4 address)
		"fec0::/10",       // deprecated site-local
	} {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}()

func isPrivateOrReserved(addr netip.Addr) bool {
	addr = addr.Unmap()
	if addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return true
	}
	for _, p := range reservedPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
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
