package native

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

// CurlPlugin performs HTTP requests, mimicking curl behaviour.
//
//	curl [options] <url>
//	curl -X POST -d '{"k":"v"}' -H 'Content-Type: application/json' https://example.com/api
//	curl -o /out.html https://example.com
//	curl -L -s https://example.com | jq .
type CurlPlugin struct{}

func (CurlPlugin) Name() string        { return "curl" }
func (CurlPlugin) Description() string { return "transfer data from or to a server" }
func (CurlPlugin) Usage() string {
	return "curl [-X method] [-d data] [-H header] [-o file] [-u user:pass] [-L] [-i] [-I] [-s] [-v] [-k] [-A agent] [-m secs] <url...>"
}

func (CurlPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	// --- option state ---
	method := ""
	var headers []string
	data := ""
	outputFile := ""
	saveAsBasename := false // -O
	followRedirects := true
	includeHeaders := false // -i
	headOnly := false       // -I
	silent := false
	verbose := false
	insecure := false
	userAgent := "memsh-curl/1.0"
	basicAuth := ""
	timeoutSecs := 30
	var urls []string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			urls = append(urls, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}

		// long flags
		switch {
		case a == "--request" || a == "-X":
			if i+1 >= len(args) {
				return fmt.Errorf("curl: option '%s' requires an argument", a)
			}
			i++
			method = strings.ToUpper(args[i])
			continue
		case strings.HasPrefix(a, "--request="):
			method = strings.ToUpper(strings.TrimPrefix(a, "--request="))
			continue
		case a == "--data" || a == "--data-ascii" || a == "--data-raw" || a == "-d":
			if i+1 >= len(args) {
				return fmt.Errorf("curl: option '%s' requires an argument", a)
			}
			i++
			data = args[i]
			continue
		case strings.HasPrefix(a, "--data="):
			data = strings.TrimPrefix(a, "--data=")
			continue
		case a == "--header" || a == "-H":
			if i+1 >= len(args) {
				return fmt.Errorf("curl: option '%s' requires an argument", a)
			}
			i++
			headers = append(headers, args[i])
			continue
		case strings.HasPrefix(a, "--header="):
			headers = append(headers, strings.TrimPrefix(a, "--header="))
			continue
		case a == "--output" || a == "-o":
			if i+1 >= len(args) {
				return fmt.Errorf("curl: option '%s' requires an argument", a)
			}
			i++
			outputFile = args[i]
			continue
		case strings.HasPrefix(a, "--output="):
			outputFile = strings.TrimPrefix(a, "--output=")
			continue
		case a == "--user" || a == "-u":
			if i+1 >= len(args) {
				return fmt.Errorf("curl: option '%s' requires an argument", a)
			}
			i++
			basicAuth = args[i]
			continue
		case strings.HasPrefix(a, "--user="):
			basicAuth = strings.TrimPrefix(a, "--user=")
			continue
		case a == "--user-agent" || a == "-A":
			if i+1 >= len(args) {
				return fmt.Errorf("curl: option '%s' requires an argument", a)
			}
			i++
			userAgent = args[i]
			continue
		case strings.HasPrefix(a, "--user-agent="):
			userAgent = strings.TrimPrefix(a, "--user-agent=")
			continue
		case a == "--max-time" || a == "-m":
			if i+1 >= len(args) {
				return fmt.Errorf("curl: option '%s' requires an argument", a)
			}
			i++
			fmt.Sscanf(args[i], "%d", &timeoutSecs)
			continue
		case strings.HasPrefix(a, "--max-time="):
			fmt.Sscanf(strings.TrimPrefix(a, "--max-time="), "%d", &timeoutSecs)
			continue
		case a == "--location":
			followRedirects = true
			continue
		case a == "--no-location":
			followRedirects = false
			continue
		case a == "--silent":
			silent = true
			continue
		case a == "--verbose":
			verbose = true
			continue
		case a == "--insecure":
			insecure = true
			continue
		case a == "--include":
			includeHeaders = true
			continue
		case a == "--head":
			headOnly = true
			continue
		case a == "--remote-name":
			saveAsBasename = true
			continue
		}

		// short flags (combined: -sLi, etc.)
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'L':
				followRedirects = true
			case 's':
				silent = true
			case 'S':
				// show errors even in silent mode — no-op for us
			case 'v':
				verbose = true
			case 'k':
				insecure = true
			case 'i':
				includeHeaders = true
			case 'I':
				headOnly = true
			case 'O':
				saveAsBasename = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("curl: invalid option -- '%s'", unknown)
		}
	}

	if len(urls) == 0 {
		return fmt.Errorf("curl: no URL specified\nUsage: %s", CurlPlugin{}.Usage())
	}

	// default method
	if method == "" {
		if headOnly {
			method = "HEAD"
		} else if data != "" {
			method = "POST"
		} else {
			method = "GET"
		}
	}

	// build HTTP client
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}, //nolint:gosec
	}
	if sc.NetworkDialContext != nil {
		transport.DialContext = sc.NetworkDialContext
	}
	client := &http.Client{
		Timeout:   time.Duration(timeoutSecs) * time.Second,
		Transport: transport,
	}
	if !followRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	for _, rawURL := range urls {
		if err := curlFetch(ctx, hc, sc, client, method, rawURL, headers, data,
			basicAuth, userAgent, outputFile, saveAsBasename,
			includeHeaders, headOnly, silent, verbose); err != nil {
			if !silent {
				fmt.Fprintf(hc.Stderr, "curl: %v\n", err)
			}
			return interp.ExitStatus(1)
		}
	}
	return nil
}

func curlFetch(
	ctx context.Context,
	hc interp.HandlerContext,
	sc plugins.ShellContext,
	client *http.Client,
	method, rawURL string,
	headers []string,
	data, basicAuth, userAgent, outputFile string,
	saveAsBasename, includeHeaders, headOnly, silent, verbose bool,
) error {
	// validate URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL '%s': %w", rawURL, err)
	}
	if u.Scheme == "" {
		rawURL = "https://" + rawURL
		u, err = url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("invalid URL '%s': %w", rawURL, err)
		}
	}

	var bodyReader io.Reader
	if data != "" {
		bodyReader = strings.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return err
	}

	// default headers
	req.Header.Set("User-Agent", userAgent)
	if data != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	// caller headers
	for _, h := range headers {
		k, v, ok := strings.Cut(h, ":")
		if !ok {
			return fmt.Errorf("malformed header (missing ':'): %s", h)
		}
		req.Header.Set(strings.TrimSpace(k), strings.TrimSpace(v))
	}

	// basic auth
	if basicAuth != "" {
		user, pass, _ := strings.Cut(basicAuth, ":")
		req.SetBasicAuth(user, pass)
	}

	if verbose {
		fmt.Fprintf(hc.Stderr, "> %s %s HTTP/1.1\n", method, u.RequestURI())
		fmt.Fprintf(hc.Stderr, "> Host: %s\n", u.Host)
		for k, vs := range req.Header {
			for _, v := range vs {
				fmt.Fprintf(hc.Stderr, "> %s: %s\n", k, v)
			}
		}
		fmt.Fprintln(hc.Stderr, ">")
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if verbose {
		fmt.Fprintf(hc.Stderr, "< HTTP/%d.%d %s\n", resp.ProtoMajor, resp.ProtoMinor, resp.Status)
		for k, vs := range resp.Header {
			for _, v := range vs {
				fmt.Fprintf(hc.Stderr, "< %s: %s\n", k, v)
			}
		}
		fmt.Fprintln(hc.Stderr, "<")
	}

	// determine output destination
	var dest io.Writer
	outPath := ""
	if outputFile != "" {
		outPath = sc.ResolvePath(outputFile)
	} else if saveAsBasename {
		base := path.Base(u.Path)
		if base == "" || base == "/" || base == "." {
			base = "index.html"
		}
		outPath = sc.ResolvePath(base)
	}

	if outPath != "" {
		f, createErr := sc.FS.Create(outPath)
		if createErr != nil {
			return fmt.Errorf("cannot create output file '%s': %w", outPath, createErr)
		}
		defer f.Close()
		dest = f
		if !silent {
			fmt.Fprintf(hc.Stderr, "  %% Total received → %s\n", outPath)
		}
	} else {
		dest = hc.Stdout
	}

	// HEAD: only print response headers
	if headOnly {
		fmt.Fprintf(dest, "HTTP/%d.%d %s\n", resp.ProtoMajor, resp.ProtoMinor, resp.Status)
		for k, vs := range resp.Header {
			for _, v := range vs {
				fmt.Fprintf(dest, "%s: %s\n", k, v)
			}
		}
		return nil
	}

	// print response headers inline (-i)
	if includeHeaders {
		fmt.Fprintf(dest, "HTTP/%d.%d %s\r\n", resp.ProtoMajor, resp.ProtoMinor, resp.Status)
		for k, vs := range resp.Header {
			for _, v := range vs {
				fmt.Fprintf(dest, "%s: %s\r\n", k, v)
			}
		}
		fmt.Fprint(dest, "\r\n")
	}

	if _, err := io.Copy(dest, resp.Body); err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}
	return nil
}

// ensure CurlPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = CurlPlugin{}
