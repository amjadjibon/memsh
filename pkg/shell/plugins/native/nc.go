package native

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// NcPlugin implements `nc` (netcat) — arbitrary TCP/UDP connections and listeners.
//
// Client mode (default):
//
//	nc [-u] [-w secs] [-v] [-n] HOST PORT
//
// Listen mode:
//
//	nc -l [-u] [-w secs] [-v] [-k] [HOST] PORT
//
// Port-scan mode:
//
//	nc -z [-v] HOST PORT [PORT2 ...]   (reports open/closed without I/O)
type NcPlugin struct{}

func (NcPlugin) Name() string        { return "nc" }
func (NcPlugin) Description() string { return "arbitrary TCP and UDP connections and listens" }
func (NcPlugin) Usage() string {
	return "nc [-l] [-u] [-z] [-w secs] [-v] [-k] [-n] [HOST] PORT"
}

func (NcPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	listen := false
	udp := false
	scan := false    // -z: port scan only
	verbose := false // -v
	keepOpen := false // -k: keep listening after disconnect
	noDNS := false   // -n: numeric only
	timeoutSecs := 0 // -w: connect/idle timeout (0 = none)
	var positional []string

	i := 1
	for i < len(args) {
		switch args[i] {
		case "--":
			i++
			goto doneFlags
		case "-l", "--listen":
			listen = true
			i++
		case "-u", "--udp":
			udp = true
			i++
		case "-z":
			scan = true
			i++
		case "-v", "--verbose":
			verbose = true
			i++
		case "-k", "--keep-open":
			keepOpen = true
			i++
		case "-n", "--nodns":
			noDNS = true
			i++
		case "-w", "--timeout", "--wait":
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "nc: -w requires an argument")
				return interp.ExitStatus(1)
			}
			secs, err := strconv.Atoi(args[i+1])
			if err != nil || secs < 0 {
				fmt.Fprintf(hc.Stderr, "nc: invalid timeout %q\n", args[i+1])
				return interp.ExitStatus(1)
			}
			timeoutSecs = secs
			i += 2
		case "-p", "--port":
			// Some netcat variants use -p for port in listen mode.
			if i+1 >= len(args) {
				fmt.Fprintln(hc.Stderr, "nc: -p requires an argument")
				return interp.ExitStatus(1)
			}
			positional = append(positional, args[i+1])
			i += 2
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(hc.Stderr, "nc: invalid option -- %q\n", args[i])
				return interp.ExitStatus(1)
			}
			positional = append(positional, args[i])
			i++
		}
	}
doneFlags:
	positional = append(positional, args[i:]...)

	proto := "tcp"
	if udp {
		proto = "udp"
	}

	// Build a dialer that respects the shell's network policy. Fail closed
	// when no policy-enforced dialer is available.
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if sc.NetworkDialContext == nil {
			return nil, fmt.Errorf("network dialer not configured")
		}
		return sc.NetworkDialContext(ctx, network, addr)
	}

	// ── listen mode ──────────────────────────────────────────────────────────
	if listen {
		if !sc.AllowHostListen {
			fmt.Fprintln(hc.Stderr, "nc: listen mode disabled (host port binding not allowed)")
			return interp.ExitStatus(1)
		}
		return ncListen(ctx, hc, proto, positional, timeoutSecs, verbose, keepOpen, noDNS)
	}

	// ── port-scan mode ───────────────────────────────────────────────────────
	if scan {
		return ncScan(ctx, hc, dial, proto, positional, timeoutSecs, verbose)
	}

	// ── client mode ──────────────────────────────────────────────────────────
	if len(positional) < 2 {
		fmt.Fprintln(hc.Stderr, "nc: usage: nc [options] HOST PORT")
		return interp.ExitStatus(1)
	}

	host := positional[0]
	port := positional[1]

	if noDNS {
		if net.ParseIP(host) == nil {
			fmt.Fprintf(hc.Stderr, "nc: -n: non-numeric hostname %q\n", host)
			return interp.ExitStatus(1)
		}
	}

	addr := net.JoinHostPort(host, port)

	dialCtx := ctx
	if timeoutSecs > 0 {
		var cancel context.CancelFunc
		dialCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
		defer cancel()
	}

	if verbose {
		fmt.Fprintf(hc.Stderr, "nc: connecting to %s (%s)\n", addr, proto)
	}

	conn, err := dial(dialCtx, proto, addr)
	if err != nil {
		fmt.Fprintf(hc.Stderr, "nc: %v\n", err)
		return interp.ExitStatus(1)
	}
	defer conn.Close()

	if verbose {
		fmt.Fprintf(hc.Stderr, "nc: connected to %s\n", conn.RemoteAddr())
	}

	relay(ctx, hc, conn, timeoutSecs)
	return nil
}

// ncListen binds a port and accepts one connection (or loops with -k).
func ncListen(
	ctx context.Context,
	hc interp.HandlerContext,
	proto string,
	positional []string,
	timeoutSecs int,
	verbose, keepOpen, noDNS bool,
) error {
	var host, port string
	switch len(positional) {
	case 1:
		port = positional[0]
	case 2:
		host = positional[0]
		port = positional[1]
	default:
		fmt.Fprintln(hc.Stderr, "nc: usage: nc -l [HOST] PORT")
		return interp.ExitStatus(1)
	}

	addr := net.JoinHostPort(host, port)
	ln, err := net.Listen(proto, addr)
	if err != nil {
		fmt.Fprintf(hc.Stderr, "nc: listen %s: %v\n", addr, err)
		return interp.ExitStatus(1)
	}
	defer ln.Close()

	// Close listener when context is cancelled so Accept unblocks.
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	if verbose {
		fmt.Fprintf(hc.Stderr, "nc: listening on %s\n", ln.Addr())
	}

	for {
		if timeoutSecs > 0 {
			ln.(*net.TCPListener).SetDeadline(time.Now().Add(time.Duration(timeoutSecs) * time.Second)) //nolint:errcheck
		}

		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Fprintf(hc.Stderr, "nc: accept: %v\n", err)
			return interp.ExitStatus(1)
		}

		if verbose {
			fmt.Fprintf(hc.Stderr, "nc: connection from %s\n", conn.RemoteAddr())
		}

		relay(ctx, hc, conn, timeoutSecs)
		conn.Close()

		if !keepOpen {
			break
		}
	}
	return nil
}

// ncScan probes each HOST:PORT and reports open/closed.
func ncScan(
	ctx context.Context,
	hc interp.HandlerContext,
	dial func(context.Context, string, string) (net.Conn, error),
	proto string,
	positional []string,
	timeoutSecs int,
	verbose bool,
) error {
	if len(positional) < 2 {
		fmt.Fprintln(hc.Stderr, "nc: -z requires HOST PORT [PORT2 ...]")
		return interp.ExitStatus(1)
	}

	host := positional[0]
	ports := positional[1:]
	timeout := 3 * time.Second
	if timeoutSecs > 0 {
		timeout = time.Duration(timeoutSecs) * time.Second
	}

	anyOpen := false
	for _, port := range ports {
		// Support port ranges e.g. "20-25".
		lo, hi, err := parsePortRange(port)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "nc: invalid port %q: %v\n", port, err)
			return interp.ExitStatus(1)
		}
		for p := lo; p <= hi; p++ {
			addr := net.JoinHostPort(host, strconv.Itoa(p))
			dCtx, cancel := context.WithTimeout(ctx, timeout)
			conn, err := dial(dCtx, proto, addr)
			cancel()
			if err == nil {
				conn.Close()
				anyOpen = true
				fmt.Fprintf(hc.Stdout, "nc: %s port %d (%s) open\n", host, p, proto)
			} else if verbose {
				fmt.Fprintf(hc.Stdout, "nc: %s port %d (%s) closed\n", host, p, proto)
			}
		}
	}

	if !anyOpen {
		return interp.ExitStatus(1)
	}
	return nil
}

// relay copies data between the connection and the handler's stdin/stdout
// concurrently until either side closes or the context is done.
func relay(ctx context.Context, hc interp.HandlerContext, conn net.Conn, timeoutSecs int) {
	if timeoutSecs > 0 {
		conn.SetDeadline(time.Now().Add(time.Duration(timeoutSecs) * time.Second)) //nolint:errcheck
	}

	done := make(chan struct{}, 2)

	// stdin → conn
	go func() {
		io.Copy(conn, hc.Stdin) //nolint:errcheck
		if tc, ok := conn.(interface{ CloseWrite() error }); ok {
			tc.CloseWrite() //nolint:errcheck
		}
		done <- struct{}{}
	}()

	// conn → stdout
	go func() {
		io.Copy(hc.Stdout, conn) //nolint:errcheck
		done <- struct{}{}
	}()

	// Wait for both goroutines or context cancel.
	for range 2 {
		select {
		case <-done:
		case <-ctx.Done():
			conn.Close()
			return
		}
	}
}

// parsePortRange parses "80" or "20-25" into (lo, hi).
func parsePortRange(s string) (lo, hi int, err error) {
	parts := strings.SplitN(s, "-", 2)
	lo, err = strconv.Atoi(parts[0])
	if err != nil || lo < 1 || lo > 65535 {
		return 0, 0, fmt.Errorf("invalid port %q", s)
	}
	if len(parts) == 1 {
		return lo, lo, nil
	}
	hi, err = strconv.Atoi(parts[1])
	if err != nil || hi < lo || hi > 65535 {
		return 0, 0, fmt.Errorf("invalid port range %q", s)
	}
	return lo, hi, nil
}

var _ plugins.PluginInfo = NcPlugin{}
