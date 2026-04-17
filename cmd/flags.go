package cmd

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/amjadjibon/memsh/pkg/network"
	"github.com/spf13/pflag"
)

type networkFlagConfig struct {
	Mode         string
	AllowDomains []string
	AllowCIDRs   []string
	AllowPorts   []string
	MaxRequests  int
	MaxBytesSent int64
	MaxBytesRecv int64
	MaxRuntime   time.Duration
}

func addNetworkFlags(fs *pflag.FlagSet, cfg *networkFlagConfig) {
	fs.StringVar(&cfg.Mode, "net-mode", string(network.ModeFull), "Network policy mode: off|allowlist|full")
	fs.StringSliceVar(&cfg.AllowDomains, "net-allow-domain", nil, "Allow outbound domain (repeatable). Supports wildcard prefix like *.example.com")
	fs.StringSliceVar(&cfg.AllowCIDRs, "net-allow-cidr", nil, "Allow outbound CIDR (repeatable), e.g. 203.0.113.0/24")
	fs.StringSliceVar(&cfg.AllowPorts, "net-allow-port", nil, "Allow outbound port (repeatable), e.g. 443")
	fs.IntVar(&cfg.MaxRequests, "net-max-requests", 0, "Maximum outbound network requests per shell/session (0 = unlimited)")
	fs.Int64Var(&cfg.MaxBytesSent, "net-max-bytes-sent", 0, "Maximum outbound network bytes sent per shell/session (0 = unlimited)")
	fs.Int64Var(&cfg.MaxBytesRecv, "net-max-bytes-recv", 0, "Maximum outbound network bytes received per shell/session (0 = unlimited)")
	fs.DurationVar(&cfg.MaxRuntime, "net-max-runtime", 0, "Maximum cumulative network runtime per shell/session (0 = unlimited)")
}

func parseNetworkPolicy(cfg networkFlagConfig) (network.Policy, error) {
	mode := network.Mode(strings.TrimSpace(strings.ToLower(cfg.Mode)))
	switch mode {
	case "", network.ModeFull:
		mode = network.ModeFull
	case network.ModeOff, network.ModeAllowlist:
	default:
		return network.Policy{}, fmt.Errorf("invalid --net-mode %q (expected off|allowlist|full)", cfg.Mode)
	}

	allowCIDRs := make([]netip.Prefix, 0, len(cfg.AllowCIDRs))
	for _, c := range cfg.AllowCIDRs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		pfx, err := netip.ParsePrefix(c)
		if err != nil {
			return network.Policy{}, fmt.Errorf("invalid --net-allow-cidr %q: %w", c, err)
		}
		allowCIDRs = append(allowCIDRs, pfx)
	}

	allowPorts := make(map[int]struct{}, len(cfg.AllowPorts))
	for _, p := range cfg.AllowPorts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return network.Policy{}, fmt.Errorf("invalid --net-allow-port %q: expected 1-65535", p)
		}
		allowPorts[n] = struct{}{}
	}

	allowDomains := make([]string, 0, len(cfg.AllowDomains))
	for _, d := range cfg.AllowDomains {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		allowDomains = append(allowDomains, d)
	}

	policy := network.Policy{
		Mode:              mode,
		AllowDomains:      allowDomains,
		AllowCIDRs:        allowCIDRs,
		AllowPorts:        allowPorts,
		DenyPrivateRanges: true,
	}
	return policy, nil
}

func parseNetworkLimits(cfg networkFlagConfig) (network.Limits, error) {
	if cfg.MaxRequests < 0 {
		return network.Limits{}, fmt.Errorf("invalid --net-max-requests: must be >= 0")
	}
	if cfg.MaxBytesSent < 0 {
		return network.Limits{}, fmt.Errorf("invalid --net-max-bytes-sent: must be >= 0")
	}
	if cfg.MaxBytesRecv < 0 {
		return network.Limits{}, fmt.Errorf("invalid --net-max-bytes-recv: must be >= 0")
	}
	if cfg.MaxRuntime < 0 {
		return network.Limits{}, fmt.Errorf("invalid --net-max-runtime: must be >= 0")
	}
	return network.Limits{
		MaxRequests:      cfg.MaxRequests,
		MaxBytesSent:     cfg.MaxBytesSent,
		MaxBytesReceived: cfg.MaxBytesRecv,
		MaxRuntime:       cfg.MaxRuntime,
	}, nil
}
