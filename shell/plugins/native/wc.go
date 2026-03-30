package native

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/spf13/afero"
	"github.com/amjadjibon/memsh/shell/plugins"
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

	for _, a := range args[1:] {
		switch a {
		case "-l":
			showLines = true
		case "-w":
			showWords = true
		case "-c":
			showBytes = true
		default:
			files = append(files, a)
		}
	}

	if !showLines && !showWords && !showBytes {
		showLines, showWords, showBytes = true, true, true
	}

	var input []byte
	if len(files) > 0 && sc.FS != nil {
		data, err := afero.ReadFile(sc.FS, sc.ResolvePath(files[0]))
		if err != nil {
			return fmt.Errorf("wc: %s: %w", files[0], err)
		}
		input = data
	} else {
		data, err := io.ReadAll(hc.Stdin)
		if err != nil {
			return err
		}
		input = data
	}

	lines, words, bytes_ := wcCount(input)
	var parts []string
	if showLines {
		parts = append(parts, fmt.Sprintf("%d", lines))
	}
	if showWords {
		parts = append(parts, fmt.Sprintf("%d", words))
	}
	if showBytes {
		parts = append(parts, fmt.Sprintf("%d", bytes_))
	}
	fmt.Fprintln(hc.Stdout, strings.Join(parts, " "))
	return nil
}

func wcCount(data []byte) (lines, words, bytes_ int) {
	bytes_ = len(data)
	inWord := false
	for _, b := range data {
		if b == '\n' {
			lines++
		}
		if unicode.IsSpace(rune(b)) {
			inWord = false
		} else {
			if !inWord {
				words++
			}
			inWord = true
		}
	}
	return
}

// ensure WcPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = WcPlugin{}
