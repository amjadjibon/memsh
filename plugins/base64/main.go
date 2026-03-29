package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	decode := false
	var inputs []string

	for _, arg := range os.Args[1:] {
		if arg == "-d" || arg == "--decode" {
			decode = true
		} else {
			inputs = append(inputs, arg)
		}
	}

	var data []byte
	var err error
	if len(inputs) > 0 {
		// Positional args are treated as inline data.
		data = []byte(strings.Join(inputs, " "))
	} else {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "base64:", err)
			os.Exit(1)
		}
	}

	if decode {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			fmt.Fprintln(os.Stderr, "base64:", err)
			os.Exit(1)
		}
		os.Stdout.Write(decoded)
	} else {
		fmt.Println(base64.StdEncoding.EncodeToString(data))
	}
}
