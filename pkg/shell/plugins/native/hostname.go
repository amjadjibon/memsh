package native

import (
	"context"
	"fmt"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// HostnamePlugin implements the `hostname` command.
// Prints the hostname from the HOSTNAME env var, falling back to the real OS
// hostname. With -f it attempts a full DNS FQDN lookup; with -s it trims the
// domain portion.
type HostnamePlugin struct{}

func (HostnamePlugin) Name() string        { return "hostname" }
func (HostnamePlugin) Description() string { return "print the system hostname" }
func (HostnamePlugin) Usage() string       { return "hostname [-f|--fqdn] [-s|--short]" }

func (HostnamePlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	fqdn := false
	short := false

	for _, a := range args[1:] {
		switch a {
		case "-f", "--fqdn", "-A", "--all-fqdns":
			fqdn = true
		case "-s", "--short":
			short = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(hc.Stderr, "hostname: invalid option -- %q\n", a)
				return interp.ExitStatus(1)
			}
		}
	}

	// Use the virtual HOSTNAME, else a fixed name. The host's real name is
	// never read, and -f does no DNS lookups (they bypassed the network
	// policy and would reveal the host's identity).
	host := sc.Env("HOSTNAME")
	if host == "" {
		host = virtualHostname
	}

	switch {
	case fqdn:
		// The virtual host has no DNS identity; its FQDN is its name.
	case short:
		if idx := strings.IndexByte(host, '.'); idx != -1 {
			host = host[:idx]
		}
	}

	fmt.Fprintln(hc.Stdout, host)
	return nil
}

var _ plugins.PluginInfo = HostnamePlugin{}
