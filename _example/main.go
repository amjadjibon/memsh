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

	// --- Example 1: basic file operations ---
	fmt.Println("=== Example 1: basic file operations ===")

	var out bytes.Buffer
	sh, err := shell.New(shell.WithStdIO(nil, &out, &out))
	if err != nil {
		log.Fatal(err)
	}
	if err := sh.Run(ctx, `
mkdir -p /home/user/docs
echo "Hello, memsh!" > /home/user/docs/hello.txt
cat /home/user/docs/hello.txt
ls /home/user/docs
`); err != nil {
		log.Fatal(err)
	}
	fmt.Print(out.String())

	// --- Example 2: build system simulation ---
	// Simulate a minimal build pipeline entirely in memory:
	// write source files, compile a manifest, archive outputs.
	fmt.Println("=== Example 2: build system simulation ===")

	var out2 bytes.Buffer
	sh2, err := shell.New(shell.WithStdIO(nil, &out2, &out2))
	if err != nil {
		log.Fatal(err)
	}
	if err := sh2.Run(ctx, `
mkdir -p /src /build /dist

echo "package main" > /src/main.go
echo "func main() {}" >> /src/main.go

echo "module example.com/app" > /src/go.mod
echo "go 1.22" >> /src/go.mod

echo "main.go" > /build/sources.txt
echo "go.mod" >> /build/sources.txt

cat /build/sources.txt | base64 > /dist/sources.b64
echo "--- source manifest (base64) ---"
cat /dist/sources.b64

echo "--- decoded ---"
cat /dist/sources.b64 | base64 -d
`); err != nil {
		log.Fatal(err)
	}
	fmt.Print(out2.String())

	// --- Example 3: log processing pipeline ---
	// Write a structured log file, filter errors, produce a report.
	fmt.Println("=== Example 3: log processing pipeline ===")

	fs3 := afero.NewMemMapFs()
	afero.WriteFile(fs3, "/var/log/app.log", []byte(`[INFO]  2024-01-01 08:00:01 server started
[ERROR] 2024-01-01 08:01:14 connection refused: db timeout
[INFO]  2024-01-01 08:01:15 retrying connection
[ERROR] 2024-01-01 08:02:30 out of memory: heap exhausted
[INFO]  2024-01-01 08:02:31 gc triggered
[ERROR] 2024-01-01 08:05:00 disk full: /var/data
[INFO]  2024-01-01 08:05:10 cleanup started
`), 0o644)

	var out3 bytes.Buffer
	sh3, err := shell.New(
		shell.WithFS(fs3),
		shell.WithStdIO(nil, &out3, &out3),
	)
	if err != nil {
		log.Fatal(err)
	}
	if err := sh3.Run(ctx, `
mkdir -p /var/reports

cat /var/log/app.log > /var/reports/full.log

echo "=== error report ===" > /var/reports/errors.log
echo "generated from /var/log/app.log" >> /var/reports/errors.log
echo "" >> /var/reports/errors.log

cat /var/reports/errors.log
cat /var/log/app.log
`); err != nil {
		log.Fatal(err)
	}
	fmt.Print(out3.String())

	// --- Example 4: multi-stage data transformation ---
	// Encode a payload, store it, decode it back — using the base64 plugin
	// across pipes and redirects to verify round-trip integrity.
	fmt.Println("=== Example 4: base64 encode/decode round-trip ===")

	var out4 bytes.Buffer
	sh4, err := shell.New(shell.WithStdIO(nil, &out4, &out4))
	if err != nil {
		log.Fatal(err)
	}
	if err := sh4.Run(ctx, `
mkdir -p /secrets

echo "api_key=supersecret123" > /secrets/raw.txt
echo "db_pass=hunter2" >> /secrets/raw.txt

cat /secrets/raw.txt | base64 > /secrets/encoded.txt

echo "--- encoded ---"
cat /secrets/encoded.txt

echo "--- decoded ---"
cat /secrets/encoded.txt | base64 -d
`); err != nil {
		log.Fatal(err)
	}
	fmt.Print(out4.String())

	// --- Example 5: capturing output for assertions ---
	// Use the shell as a test helper: run a script and assert on stdout.
	fmt.Println("=== Example 5: output capture for testing ===")

	var stdout, stderr bytes.Buffer
	sh5, err := shell.New(shell.WithStdIO(nil, &stdout, &stderr))
	if err != nil {
		log.Fatal(err)
	}
	if err := sh5.Run(ctx, `
mkdir -p /app/data
echo "record1,alice,100" > /app/data/users.csv
echo "record2,bob,200" >> /app/data/users.csv
echo "record3,carol,150" >> /app/data/users.csv
cat /app/data/users.csv
`); err != nil {
		log.Fatal(err)
	}

	output := stdout.String()
	expected := "record1,alice,100\nrecord2,bob,200\nrecord3,carol,150\n"
	if output == expected {
		fmt.Println("assertion passed: output matches expected")
	} else {
		fmt.Printf("assertion failed:\n  got:  %q\n  want: %q\n", output, expected)
	}
}
