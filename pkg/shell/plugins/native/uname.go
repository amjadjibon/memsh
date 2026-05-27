package native

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

// UnamePlugin implements `uname`.
// Reports system information — virtual values where overridden by env vars,
// falling back to real runtime values.
//
//	uname [-a] [-s] [-n] [-r] [-v] [-m] [-p] [-o]
type UnamePlugin struct{}

func (UnamePlugin) Name() string        { return "uname" }
func (UnamePlugin) Description() string { return "print system information" }
func (UnamePlugin) Usage() string       { return "uname [-a] [-s] [-n] [-r] [-v] [-m] [-p] [-o]" }

func (UnamePlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	showKernel := false
	showNode := false
	showRelease := false
	showVersion := false
	showMachine := false
	showProcessor := false
	showOS := false
	all := false

	for _, a := range args[1:] {
		if a == "--" {
			break
		}
		if !strings.HasPrefix(a, "-") {
			fmt.Fprintf(hc.Stderr, "uname: extra operand %q\n", a)
			return interp.ExitStatus(1)
		}
		for _, c := range a[1:] {
			switch c {
			case 'a':
				all = true
			case 's':
				showKernel = true
			case 'n':
				showNode = true
			case 'r':
				showRelease = true
			case 'v':
				showVersion = true
			case 'm':
				showMachine = true
			case 'p':
				showProcessor = true
			case 'o':
				showOS = true
			default:
				fmt.Fprintf(hc.Stderr, "uname: invalid option -- '%c'\n", c)
				return interp.ExitStatus(1)
			}
		}
	}

	if all {
		showKernel = true
		showNode = true
		showRelease = true
		showVersion = true
		showMachine = true
		showProcessor = true
		showOS = true
	}

	// Default: kernel name only.
	if !showKernel && !showNode && !showRelease && !showVersion &&
		!showMachine && !showProcessor && !showOS {
		showKernel = true
	}

	// Resolve values: prefer env vars, fall back to runtime.
	kernel := sc.Env("UNAME_KERNEL")
	if kernel == "" {
		switch runtime.GOOS {
		case "darwin":
			kernel = "Darwin"
		case "linux":
			kernel = "Linux"
		case "windows":
			kernel = "Windows_NT"
		default:
			kernel = runtime.GOOS
		}
	}

	node := sc.Env("UNAME_NODE")
	if node == "" {
		if h, err := os.Hostname(); err == nil {
			node = h
		} else {
			node = "localhost"
		}
	}

	release := sc.Env("UNAME_RELEASE")
	if release == "" {
		release = "1.0.0"
	}

	version := sc.Env("UNAME_VERSION")
	if version == "" {
		version = "#1 " + runtime.Version()
	}

	machine := sc.Env("UNAME_MACHINE")
	if machine == "" {
		switch runtime.GOARCH {
		case "amd64":
			machine = "x86_64"
		case "arm64":
			machine = "arm64"
		case "386":
			machine = "i686"
		default:
			machine = runtime.GOARCH
		}
	}

	processor := sc.Env("UNAME_PROCESSOR")
	if processor == "" {
		processor = machine
	}

	osName := sc.Env("UNAME_OS")
	if osName == "" {
		switch runtime.GOOS {
		case "darwin":
			osName = "Darwin"
		case "linux":
			osName = "GNU/Linux"
		default:
			osName = runtime.GOOS
		}
	}

	var parts []string
	if showKernel {
		parts = append(parts, kernel)
	}
	if showNode {
		parts = append(parts, node)
	}
	if showRelease {
		parts = append(parts, release)
	}
	if showVersion {
		parts = append(parts, version)
	}
	if showMachine {
		parts = append(parts, machine)
	}
	if showProcessor {
		parts = append(parts, processor)
	}
	if showOS {
		parts = append(parts, osName)
	}

	fmt.Fprintln(hc.Stdout, strings.Join(parts, " "))
	return nil
}

var _ plugins.PluginInfo = UnamePlugin{}
