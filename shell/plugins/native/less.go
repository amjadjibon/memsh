package native

import (
	"context"
	"fmt"
	"io"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/shell/plugins"
)

// PagerSentinel is written at the very start of `less` stdout when running in
// pager mode (web terminal / HTTP server).  The server strips it and sets
// pager:true in the JSON response so the browser can render the overlay.
const PagerSentinel = "\x00PAGER\x00"

// pagerModeKey is the unexported context key used by WithPagerMode.
type pagerModeKey struct{}

// WithPagerMode returns a context that tells the less/more plugin to emit
// PagerSentinel output instead of writing raw content.  Call this from the
// HTTP server handler before creating the shell.
func WithPagerMode(ctx context.Context) context.Context {
	return context.WithValue(ctx, pagerModeKey{}, true)
}

func isPagerMode(ctx context.Context) bool {
	v, _ := ctx.Value(pagerModeKey{}).(bool)
	return v
}

// LessPlugin implements the `less` pager.  It reads file arguments (or stdin)
// and writes PagerSentinel + the full content to stdout so the web terminal
// can render a scrollable pager overlay.
//
// Most flags that control display behaviour (line numbers, wrapping, colour)
// are client-side concerns and are silently accepted but ignored here.
// Flags that affect content selection (+N start line, -N/-n line numbers) are
// passed through in the sentinel metadata line so the client can act on them.
//
// Sentinel format:
//
//	\x00PAGER\x00<JSON metadata>\n<file content>
//
// Metadata JSON fields:
//
//	start   int   – 0-based line to start at (from +N argument, default 0)
//	numbers bool  – whether to show line numbers (-N flag, default true; -n disables)
type LessPlugin struct{ name string }

// Less returns a LessPlugin registered as "less".
func Less() LessPlugin { return LessPlugin{name: "less"} }

// More returns a LessPlugin registered as "more".
func More() LessPlugin { return LessPlugin{name: "more"} }

func (p LessPlugin) Name() string        { return p.name }
func (p LessPlugin) Description() string { return "scrollable pager" }
func (p LessPlugin) Usage() string       { return p.name + " [-Nn] [+N] [file...]" }

func (p LessPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	startLine := 0
	showNumbers := true // -N shows, -n hides; default show
	var files []string

	endFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endFlags || a == "" {
			files = append(files, a)
			continue
		}
		if a == "--" {
			endFlags = true
			continue
		}
		// +N  →  start at line N (1-based from user, 0-based internally)
		if a[0] == '+' {
			n := 0
			if _, err := fmt.Sscanf(a[1:], "%d", &n); err == nil && n > 0 {
				startLine = n - 1
			}
			continue
		}
		if a[0] == '-' {
			// -  means read stdin explicitly
			if a == "-" {
				files = append(files, "-")
				continue
			}
			for _, c := range a[1:] {
				switch c {
				case 'N':
					showNumbers = true
				case 'n':
					showNumbers = false
				// all other flags silently ignored
				}
			}
			continue
		}
		files = append(files, a)
	}

	// collect content
	var content strings.Builder
	if len(files) == 0 {
		data, err := io.ReadAll(hc.Stdin)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "%s: %v\n", p.name, err)
			return interp.ExitStatus(1)
		}
		content.Write(data)
	} else {
		for _, f := range files {
			if f == "-" {
				data, err := io.ReadAll(hc.Stdin)
				if err != nil {
					fmt.Fprintf(hc.Stderr, "%s: stdin: %v\n", p.name, err)
					continue
				}
				content.Write(data)
				continue
			}
			path := sc.ResolvePath(f)
			fh, err := sc.FS.Open(path)
			if err != nil {
				fmt.Fprintf(hc.Stderr, "%s: %s: %v\n", p.name, f, err)
				continue
			}
			data, rerr := io.ReadAll(fh)
			fh.Close()
			if rerr != nil {
				fmt.Fprintf(hc.Stderr, "%s: %s: %v\n", p.name, f, rerr)
				continue
			}
			content.Write(data)
		}
	}

	if isPagerMode(ctx) {
		// Web terminal / HTTP server: emit sentinel so the client can render a pager overlay.
		fmt.Fprintf(hc.Stdout, "%s{\"start\":%d,\"numbers\":%v}\n%s",
			PagerSentinel, startLine, showNumbers, content.String())
	} else {
		// REPL / CLI / pipe: output content directly (acts like cat).
		fmt.Fprint(hc.Stdout, content.String())
	}
	return nil
}

var _ plugins.PluginInfo = LessPlugin{}
