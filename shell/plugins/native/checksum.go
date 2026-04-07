package native

import (
	"bufio"
	"context"
	"crypto/md5"  //nolint:gosec
	"crypto/sha1" //nolint:gosec
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/amjadjibon/memsh/shell/plugins"
)

type hashAlgo int

const (
	algoMD5 hashAlgo = iota
	algoSHA1
	algoSHA224
	algoSHA256
	algoSHA384
	algoSHA512
)

func (a hashAlgo) new() hash.Hash {
	switch a {
	case algoMD5:
		return md5.New() //nolint:gosec
	case algoSHA1:
		return sha1.New() //nolint:gosec
	case algoSHA224:
		return sha256.New224()
	case algoSHA384:
		return sha512.New384()
	case algoSHA512:
		return sha512.New()
	default:
		return sha256.New()
	}
}

// ChecksumPlugin computes or verifies file checksums.
//
//	sha256sum file...
//	sha256sum -c sums.txt
//	echo "data" | sha256sum
type ChecksumPlugin struct{ algo hashAlgo }

func MD5Sum() ChecksumPlugin    { return ChecksumPlugin{algoMD5} }
func SHA1Sum() ChecksumPlugin   { return ChecksumPlugin{algoSHA1} }
func SHA224Sum() ChecksumPlugin { return ChecksumPlugin{algoSHA224} }
func SHA256Sum() ChecksumPlugin { return ChecksumPlugin{algoSHA256} }
func SHA384Sum() ChecksumPlugin { return ChecksumPlugin{algoSHA384} }
func SHA512Sum() ChecksumPlugin { return ChecksumPlugin{algoSHA512} }

func (c ChecksumPlugin) Name() string {
	switch c.algo {
	case algoMD5:
		return "md5sum"
	case algoSHA1:
		return "sha1sum"
	case algoSHA224:
		return "sha224sum"
	case algoSHA384:
		return "sha384sum"
	case algoSHA512:
		return "sha512sum"
	default:
		return "sha256sum"
	}
}

func (c ChecksumPlugin) Description() string {
	return "compute and check " + strings.TrimSuffix(c.Name(), "sum") + " checksums"
}

func (c ChecksumPlugin) Usage() string {
	return c.Name() + " [-c] [file...]"
}

func (c ChecksumPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	checkMode := false
	binary := false // -b flag accepted but ignored (no binary/text distinction in virtual FS)
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
		case "--check":
			checkMode = true
			continue
		case "--binary":
			binary = true
			continue
		case "--text":
			binary = false
			continue
		case "--quiet", "--status", "--warn", "--strict":
			continue
		}
		unknown := ""
		for _, ch := range a[1:] {
			switch ch {
			case 'c':
				checkMode = true
			case 'b':
				binary = true
			case 't':
				binary = false
			case 'q', 'w':
				// quiet/warn — ignore
			default:
				unknown += string(ch)
			}
		}
		if unknown != "" {
			return fmt.Errorf("%s: invalid option -- '%s'", c.Name(), unknown)
		}
	}
	_ = binary

	if checkMode {
		return c.verify(hc, sc, files)
	}
	return c.compute(hc, sc, files)
}

// compute hashes files (or stdin) and prints "<hash>  <name>" lines.
func (c ChecksumPlugin) compute(hc interp.HandlerContext, sc plugins.ShellContext, files []string) error {
	if len(files) == 0 {
		sum, err := c.hashReader(hc.Stdin)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Name(), err)
		}
		fmt.Fprintf(hc.Stdout, "%s  -\n", sum)
		return nil
	}

	exitErr := false
	for _, f := range files {
		var sum string
		var err error
		if f == "-" {
			sum, err = c.hashReader(hc.Stdin)
		} else {
			sum, err = c.hashFile(sc, f)
		}
		if err != nil {
			fmt.Fprintf(hc.Stderr, "%s: %s: %v\n", c.Name(), f, err)
			exitErr = true
			continue
		}
		fmt.Fprintf(hc.Stdout, "%s  %s\n", sum, f)
	}
	if exitErr {
		return interp.ExitStatus(1)
	}
	return nil
}

// verify reads a checksum file and re-hashes each listed file.
func (c ChecksumPlugin) verify(hc interp.HandlerContext, sc plugins.ShellContext, files []string) error {
	var src io.Reader
	if len(files) == 0 || files[0] == "-" {
		src = hc.Stdin
	} else {
		abs := sc.ResolvePath(files[0])
		f, err := sc.FS.Open(abs)
		if err != nil {
			return fmt.Errorf("%s: %s: %w", c.Name(), files[0], err)
		}
		defer f.Close()
		src = f
	}

	failed := 0
	total := 0
	scanner := bufio.NewScanner(src)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// format: "<hash>  <file>" or "<hash> *<file>"
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			// try single-space (binary indicator)
			parts = strings.SplitN(line, " *", 2)
			if len(parts) != 2 {
				fmt.Fprintf(hc.Stderr, "%s: improperly formatted checksum line\n", c.Name())
				continue
			}
		}
		wantSum := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		total++

		gotSum, err := c.hashFile(sc, name)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "%s: %s: FAILED open or read\n", c.Name(), name)
			failed++
			continue
		}
		if gotSum != wantSum {
			fmt.Fprintf(hc.Stdout, "%s: FAILED\n", name)
			failed++
		} else {
			fmt.Fprintf(hc.Stdout, "%s: OK\n", name)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%s: %w", c.Name(), err)
	}
	if failed > 0 {
		fmt.Fprintf(hc.Stderr, "%s: WARNING: %d of %d computed checksums did NOT match\n",
			c.Name(), failed, total)
		return interp.ExitStatus(1)
	}
	return nil
}

func (c ChecksumPlugin) hashFile(sc plugins.ShellContext, name string) (string, error) {
	abs := sc.ResolvePath(name)
	f, err := sc.FS.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return c.hashReader(f)
}

func (c ChecksumPlugin) hashReader(r io.Reader) (string, error) {
	h := c.algo.new()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ensure ChecksumPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = ChecksumPlugin{}
