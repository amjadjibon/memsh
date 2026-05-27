package native

import (
	"context"
	"fmt"
	"net"
	"os"
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

	// Prefer virtual env var, then real OS hostname.
	host := sc.Env("HOSTNAME")
	if host == "" {
		var err error
		host, err = os.Hostname()
		if err != nil {
			host = "localhost"
		}
	}

	switch {
	case fqdn:
		addrs, err := net.LookupHost(host)
		if err == nil && len(addrs) > 0 {
			if names, err := net.LookupAddr(addrs[0]); err == nil && len(names) > 0 {
				host = strings.TrimSuffix(names[0], ".")
			}
		}
	case short:
		if idx := strings.IndexByte(host, '.'); idx != -1 {
			host = host[:idx]
		}
	}

	fmt.Fprintln(hc.Stdout, host)
	return nil
}

var _ plugins.PluginInfo = HostnamePlugin{}
