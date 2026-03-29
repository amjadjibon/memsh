package main

import (
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
)

func main() {
	args := os.Args[1:] // argv[0] is the command name

	// Flag parsing
	flagLines := false
	flagWords := false
	flagBytes := false
	files := []string{}

	for _, a := range args {
		switch a {
		case "-l":
			flagLines = true
		case "-w":
			flagWords = true
		case "-c":
			flagBytes = true
		case "-lw":
			flagLines = true
			flagWords = true
		case "-lc":
			flagLines = true
			flagBytes = true
		case "-wc":
			flagWords = true
			flagBytes = true
		case "-lwc", "-lcw", "-wlc", "-wcl", "-clw", "-cwl":
			flagLines = true
			flagWords = true
			flagBytes = true
		default:
			if strings.HasPrefix(a, "-") {
				os.Stderr.WriteString("wc: unknown flag: " + a + "\n")
				os.Exit(1)
			}
			files = append(files, a)
		}
	}

	// Default: show all three if no flags given
	if !flagLines && !flagWords && !flagBytes {
		flagLines = true
		flagWords = true
		flagBytes = true
	}

	if len(files) == 0 {
		// Read from stdin
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Stderr.WriteString("wc: read error: " + err.Error() + "\n")
			os.Exit(1)
		}
		lines, words, bytes_ := count(data)
		printResult(flagLines, flagWords, flagBytes, lines, words, bytes_, "")
	} else {
		// Read from each file
		totalLines, totalWords, totalBytes := 0, 0, 0

		for _, path := range files {
			f, err := os.Open(path)
			if err != nil {
				os.Stderr.WriteString("wc: " + path + ": " + err.Error() + "\n")
				os.Exit(1)
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				os.Stderr.WriteString("wc: " + path + ": read error: " + err.Error() + "\n")
				os.Exit(1)
			}

			lines, words, bytes_ := count(data)
			printResult(flagLines, flagWords, flagBytes, lines, words, bytes_, path)

			totalLines += lines
			totalWords += words
			totalBytes += bytes_
		}

		if len(files) > 1 {
			printResult(flagLines, flagWords, flagBytes, totalLines, totalWords, totalBytes, "total")
		}
	}
}

// count returns line count, word count, and byte count for the given data.
func count(data []byte) (lines, words, bytes_ int) {
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

// printResult formats and writes one result line to stdout.
func printResult(flagLines, flagWords, flagBytes bool, lines, words, bytes_ int, label string) {
	parts := []string{}

	if flagLines {
		parts = append(parts, pad(strconv.Itoa(lines)))
	}
	if flagWords {
		parts = append(parts, pad(strconv.Itoa(words)))
	}
	if flagBytes {
		parts = append(parts, pad(strconv.Itoa(bytes_)))
	}
	if label != "" {
		parts = append(parts, label)
	}

	os.Stdout.WriteString(strings.Join(parts, " ") + "\n")
}

// pad right-aligns a number string to 8 chars, matching GNU wc behaviour.
func pad(s string) string {
	for len(s) < 8 {
		s = " " + s
	}
	return s
}
