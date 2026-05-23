// Package native contains the built-in native Go plugins shipped with memsh.
package native

import (
	"context"
	"fmt"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"github.com/mvm-sh/mvm/interp"
	"github.com/mvm-sh/mvm/lang/golang"
	"github.com/mvm-sh/mvm/stdlib"
	"github.com/spf13/afero"
	shinterp "mvdan.cc/sh/v3/interp"
)

// goMu serialises go invocations because os.Pipe-based stdout capture is not
// goroutine-safe.
var goMu sync.Mutex

// GoPlugin emulates the Go toolchain for source files stored in the virtual
// filesystem, backed by the MVM interpreter.
//
//	go run <file.go>                      run a Go source file
//	go test [./path|./...] [-v] [-run RE] run Test* functions
//	go fmt  <file.go> [...]               format Go source in place
//	go version                            print MVM version
type GoPlugin struct{}

func (GoPlugin) Name() string        { return "go" }
func (GoPlugin) Description() string { return "Go tool — run, test, and format Go source in the virtual filesystem (via MVM)" }
func (GoPlugin) Usage() string {
	return strings.TrimSpace(`
go run <file.go>                        run a Go source file
go test [./path|./...] [-v] [-run RE]   run Test* functions
go fmt  <file.go> [...]                 gofmt source files in place
go version                              print MVM version`)
}

func (GoPlugin) Run(ctx context.Context, args []string) error {
	hc := shinterp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	// No subcommand: read Go source from stdin.
	if len(args) < 2 {
		data, err := io.ReadAll(hc.Stdin)
		if err != nil {
			return fmt.Errorf("go: read stdin: %w", err)
		}
		return runGoSource(hc.Stdout, hc.Stderr, string(data), "<stdin>")
	}

	switch args[1] {
	case "run":
		return goRun(hc.Stdout, hc.Stderr, sc, args[2:])
	case "test":
		return goTest(hc.Stdout, hc.Stderr, sc, args[2:])
	case "fmt":
		return goFmt(hc.Stdout, sc, args[2:])
	case "version":
		fmt.Fprintln(hc.Stdout, "go version mvm0.3.0 (github.com/mvm-sh/mvm)")
		return nil
	case "help", "-h", "--help":
		fmt.Fprintln(hc.Stdout, GoPlugin{}.Usage())
		return nil
	default:
		fmt.Fprintf(hc.Stderr, "go %s: unknown subcommand\nRun 'go help' for usage.\n", args[1])
		return shinterp.ExitStatus(2)
	}
}

// ── go run ────────────────────────────────────────────────────────────────────

func goRun(stdout, stderr io.Writer, sc plugins.ShellContext, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("go run: no .go file specified")
	}
	filePath := sc.ResolvePath(args[0])
	data, err := afero.ReadFile(sc.FS, filePath)
	if err != nil {
		return fmt.Errorf("go run: %s: %w", args[0], err)
	}
	return runGoSource(stdout, stderr, string(data), args[0])
}

// ── go test ───────────────────────────────────────────────────────────────────

var testFuncRe = regexp.MustCompile(`(?m)^func (Test\w+)\(`)

func goTest(stdout, stderr io.Writer, sc plugins.ShellContext, args []string) error {
	verbose := false
	runFilter := ""
	path := "."

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-v":
			verbose = true
		case "-run":
			if i+1 < len(args) {
				i++
				runFilter = args[i]
			}
		default:
			path = args[i]
		}
	}

	recursive := strings.HasSuffix(path, "...")
	root := sc.ResolvePath(strings.TrimSuffix(strings.TrimSuffix(path, "..."), "/"))

	testFiles, err := collectTestFiles(sc.FS, root, recursive)
	if err != nil {
		return fmt.Errorf("go test: %w", err)
	}
	if len(testFiles) == 0 {
		fmt.Fprintln(stdout, "?   \t[no test files]")
		return nil
	}

	allOK := true
	for _, tf := range testFiles {
		ok, err := runTestFile(stdout, stderr, sc.FS, tf, verbose, runFilter)
		if err != nil {
			fmt.Fprintf(stderr, "FAIL\t%s\n%v\n", tf, err)
			allOK = false
		} else if !ok {
			allOK = false
		}
	}
	if !allOK {
		return shinterp.ExitStatus(1)
	}
	return nil
}

func collectTestFiles(fs afero.Fs, root string, recursive bool) ([]string, error) {
	var files []string
	if recursive {
		afero.Walk(fs, root, func(p string, info os.FileInfo, err error) error { //nolint:errcheck
			if err == nil && !info.IsDir() && strings.HasSuffix(p, "_test.go") {
				files = append(files, p)
			}
			return nil
		})
		return files, nil
	}
	entries, err := afero.ReadDir(fs, root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), "_test.go") {
			files = append(files, filepath.Join(root, e.Name()))
		}
	}
	return files, nil
}

func runTestFile(stdout, stderr io.Writer, fs afero.Fs, path string, verbose bool, runFilter string) (bool, error) {
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return false, err
	}
	src := string(data)

	matches := testFuncRe.FindAllStringSubmatch(src, -1)
	if len(matches) == 0 {
		fmt.Fprintf(stdout, "?   \t%s [no test functions]\n", path)
		return true, nil
	}

	var names []string
	for _, m := range matches {
		name := m[1]
		if runFilter != "" {
			if ok, _ := regexp.MatchString(runFilter, name); !ok {
				continue
			}
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return true, nil
	}

	runner := buildTestRunner(src, names, verbose)
	if err := runGoSource(stdout, stderr, runner, path); err != nil {
		fmt.Fprintf(stdout, "FAIL\t%s\n", path)
		return false, err
	}
	return true, nil
}

// buildTestRunner generates a self-contained main that runs the discovered
// Test* functions using a minimal T shim (MVM does not fully support
// testing.RunTests). The shim replaces *testing.T in function signatures so
// the test source compiles without the stdlib testing package.
func buildTestRunner(src string, names []string, verbose bool) string {
	stripped := regexp.MustCompile(`(?m)^package \w+\s*\n`).ReplaceAllString(src, "")

	// Rewrite *testing.T → *T so test functions accept our shim.
	stripped = strings.ReplaceAll(stripped, "*testing.T", "*T")
	stripped = strings.ReplaceAll(stripped, " testing.T", " T")

	// Drop import "testing" — the shim provides everything the tests use.
	stripped = regexp.MustCompile(`(?m)^\s*"testing"\n`).ReplaceAllString(stripped, "")
	stripped = regexp.MustCompile(`(?m)^import "testing"\n`).ReplaceAllString(stripped, "")

	var b strings.Builder
	b.WriteString("package main\n\nimport \"fmt\"\n\n")

	// Minimal T shim.
	b.WriteString(`type T struct{ name string; failed bool }
func (t *T) Name() string { return t.name }
func (t *T) Helper()      {}
func (t *T) Log(a ...interface{})                   { fmt.Println(a...) }
func (t *T) Logf(f string, a ...interface{})        { fmt.Printf(f+"\n", a...) }
func (t *T) Error(a ...interface{})                 { t.failed = true; fmt.Println("    FAIL:", fmt.Sprint(a...)) }
func (t *T) Errorf(f string, a ...interface{})      { t.failed = true; fmt.Printf("    FAIL: "+f+"\n", a...) }
func (t *T) Fatal(a ...interface{})                 { t.failed = true; fmt.Println("    FAIL:", fmt.Sprint(a...)); panic("_fatal_") }
func (t *T) Fatalf(f string, a ...interface{})      { t.failed = true; fmt.Printf("    FAIL: "+f+"\n", a...); panic("_fatal_") }
func (t *T) Skip(a ...interface{})                  { panic("_skip_") }
func (t *T) Skipf(f string, a ...interface{})       { panic("_skip_") }
func (t *T) SkipNow()                               { panic("_skip_") }
func (t *T) FailNow()                               { t.failed = true; panic("_fatal_") }
func (t *T) Run(name string, f func(*T)) bool {
	sub := &T{name: t.name + "/" + name}
	_runTest(sub, f)
	if sub.failed { t.failed = true }
	return !sub.failed
}

func _runTest(t *T, f func(*T)) {
	defer func() {
		r := recover()
		if r == "_fatal_" || r == nil { return }
		if r == "_skip_" { fmt.Printf("--- SKIP: %s\n", t.name); return }
		t.failed = true
		fmt.Printf("    panic: %v\n", r)
	}()
	f(t)
}

`)

	b.WriteString(stripped)
	b.WriteString("\nfunc main() {\n\tallOK := true\n")
	for _, name := range names {
		if verbose {
			fmt.Fprintf(&b, "\tfmt.Printf(\"=== RUN   %s\\n\")\n", name)
		}
		fmt.Fprintf(&b, "\t{ t := &T{name: %q}; _runTest(t, %s)\n", name, name)
		fmt.Fprintf(&b, "\t  if t.failed { fmt.Printf(\"--- FAIL: %s\\n\"); allOK = false } else { fmt.Printf(\"--- PASS: %s\\n\") } }\n", name, name)
	}
	b.WriteString("\tif allOK { fmt.Println(\"ok\") } else { fmt.Println(\"FAIL\") }\n}\n")
	return b.String()
}

// ── go fmt ────────────────────────────────────────────────────────────────────

func goFmt(stdout io.Writer, sc plugins.ShellContext, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("go fmt: no file specified")
	}
	for _, arg := range args {
		filePath := sc.ResolvePath(arg)
		data, err := afero.ReadFile(sc.FS, filePath)
		if err != nil {
			return fmt.Errorf("go fmt: %s: %w", arg, err)
		}
		formatted, err := format.Source(data)
		if err != nil {
			return fmt.Errorf("go fmt: %s: %w", arg, err)
		}
		if err := afero.WriteFile(sc.FS, filePath, formatted, 0o644); err != nil {
			return fmt.Errorf("go fmt: %s: %w", arg, err)
		}
		fmt.Fprintln(stdout, arg)
	}
	return nil
}

// ── shared MVM execution ──────────────────────────────────────────────────────

// runGoSource compiles and executes src via MVM, redirecting os.Stdout/Stderr
// to the provided writers. Serialised with goMu because os.Pipe capture is not
// goroutine-safe.
func runGoSource(stdout, stderr io.Writer, src, label string) error {
	goMu.Lock()
	defer goMu.Unlock()

	outR, outW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("go: pipe: %w", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		outR.Close()
		outW.Close()
		return fmt.Errorf("go: pipe: %w", err)
	}

	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW

	done := make(chan struct{})
	go func() {
		defer close(done)
		io.Copy(stdout, outR) //nolint:errcheck
		io.Copy(stderr, errR) //nolint:errcheck
	}()

	runErr := evalGoSource(src, label)

	os.Stdout, os.Stderr = origOut, origErr
	outW.Close()
	errW.Close()
	<-done
	outR.Close()
	errR.Close()

	if runErr != nil {
		var exitErr *interp.ExitError
		if e, ok := runErr.(*interp.ExitError); ok {
			exitErr = e
			return shinterp.ExitStatus(exitErr.Code)
		}
		return fmt.Errorf("go: %w", runErr)
	}
	return nil
}

func evalGoSource(src, label string) (err error) {
	i := interp.NewInterpreter(golang.GoSpec)
	i.ImportPackageValues(stdlib.Values)
	i.AutoImportPackages()
	_, err = i.Eval(label, src)
	return err
}

// ensure GoPlugin satisfies plugins.PluginInfo at compile time.
var _ plugins.PluginInfo = GoPlugin{}
