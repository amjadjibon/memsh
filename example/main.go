package main

import (
	"bytes"
	"context"
	"fmt"
	"log"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/shell"
)

func main() {
	ctx := context.Background()

	// --- Example 1: basic commands with captured output ---
	fmt.Println("=== Example 1: basic file operations ===")

	var out bytes.Buffer
	sh, err := shell.New(
		shell.WithStdIO(nil, &out, &out),
	)
	if err != nil {
		log.Fatal(err)
	}

	script := `
mkdir -p /home/user/docs
echo "Hello, memsh!" > /home/user/docs/hello.txt
cat /home/user/docs/hello.txt
ls /home/user/docs
`
	if err := sh.Run(ctx, script); err != nil {
		log.Fatal(err)
	}
	fmt.Print(out.String())

	// --- Example 2: pre-seeded filesystem ---
	fmt.Println("=== Example 2: pre-seeded filesystem ===")

	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, "/data/config.txt", []byte("debug=true\nport=8080\n"), 0644); err != nil {
		log.Fatal(err)
	}

	var out2 bytes.Buffer
	sh2, err := shell.New(
		shell.WithFS(fs),
		shell.WithStdIO(nil, &out2, &out2),
	)
	if err != nil {
		log.Fatal(err)
	}

	if err := sh2.Run(ctx, `cat /data/config.txt`); err != nil {
		log.Fatal(err)
	}
	fmt.Print(out2.String())

	// --- Example 3: environment variables ---
	fmt.Println("=== Example 3: environment variables ===")

	var out3 bytes.Buffer
	sh3, err := shell.New(
		shell.WithEnv(map[string]string{
			"APP_NAME": "memsh",
			"VERSION":  "0.1.0",
		}),
		shell.WithStdIO(nil, &out3, &out3),
	)
	if err != nil {
		log.Fatal(err)
	}

	if err := sh3.Run(ctx, `APP_NAME=memsh VERSION=0.1.0 && echo "$APP_NAME v$VERSION"`); err != nil {
		log.Fatal(err)
	}
	fmt.Print(out3.String())

	// --- Example 4: pipes and redirects ---
	fmt.Println("=== Example 4: pipes and redirects ===")

	var out4 bytes.Buffer
	sh4, err := shell.New(
		shell.WithStdIO(nil, &out4, &out4),
	)
	if err != nil {
		log.Fatal(err)
	}

	script4 := `
mkdir /logs
echo "error: disk full" > /logs/app.log
echo "info: started" >> /logs/app.log
echo "error: timeout" >> /logs/app.log
cat /logs/app.log
`
	if err := sh4.Run(ctx, script4); err != nil {
		log.Fatal(err)
	}
	fmt.Print(out4.String())
}
