package native

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/spf13/afero"
	"mvdan.cc/sh/v3/interp"
)

// WcPlugin counts lines, words, and bytes.
//
//	wc [-l] [-w] [-c] [file]   read from virtual-FS file or stdin
type WcPlugin struct{}

func (WcPlugin) Name() string        { return "wc" }
func (WcPlugin) Description() string { return "count lines, words, and bytes" }
func (WcPlugin) Usage() string       { return "wc [-l] [-w] [-c] [file]" }

func (WcPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	showLines, showWords, showBytes := false, false, false
	var files []string

	endOfFlags := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if endOfFlags || a == "" || a[0] != '-' {
			files = append(files, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		switch a {
		case "--lines":
			showLines = true
			continue
		case "--words":
			showWords = true
			continue
		case "--bytes":
			showBytes = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'l':
				showLines = true
			case 'w':
				showWords = true
			case 'c':
				showBytes = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			return fmt.Errorf("wc: invalid option -- '%s'", unknown)
		}
	}

	if !showLines && !showWords && !showBytes {
		showLines, showWords, showBytes = true, true, true
	}

	totalLines, totalWords, totalBytes := 0, 0, 0
	printCounts := func(data []byte, name string) {
		lines, words, bytesCount := wcCount(data)
		totalLines += lines
		totalWords += words
		totalBytes += bytesCount

		var parts []string
		if showLines {
			parts = append(parts, fmt.Sprintf("%d", lines))
		}
		if showWords {
			parts = append(parts, fmt.Sprintf("%d", words))
		}
		if showBytes {
			parts = append(parts, fmt.Sprintf("%d", bytesCount))
		}
		if name != "" {
			parts = append(parts, name)
		}
		fmt.Fprintln(hc.Stdout, strings.Join(parts, " "))
	}

	if len(files) == 0 {
		data, err := io.ReadAll(hc.Stdin)
		if err != nil {
			return err
		}
		printCounts(data, "")
		return nil
	}

	for _, file := range files {
		data, err := afero.ReadFile(sc.FS, sc.ResolvePath(file))
		if err != nil {
			return fmt.Errorf("wc: %s: %w", file, err)
		}
		printCounts(data, file)
	}

	if len(files) > 1 {
		var parts []string
		if showLines {
			parts = append(parts, fmt.Sprintf("%d", totalLines))
		}
		if showWords {
			parts = append(parts, fmt.Sprintf("%d", totalWords))
		}
		if showBytes {
			parts = append(parts, fmt.Sprintf("%d", totalBytes))
		}
		parts = append(parts, "total")
		fmt.Fprintln(hc.Stdout, strings.Join(parts, " "))
	}
	return nil
}

func wcCount(data []byte) (lines, words, bytes_ int) {
	bytes_ = len(data)
	content := string(data)
	lines = strings.Count(content, "\n")
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		lines++
	}
	words = len(strings.Fields(content))
	return
}

// ensure WcPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = WcPlugin{}
