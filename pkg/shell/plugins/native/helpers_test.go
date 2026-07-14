package native

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
)

func TestCopyWithContextCopiesAndStopsAtEOF(t *testing.T) {
	var out bytes.Buffer
	n, err := copyWithContext(context.Background(), &out, strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("copyWithContext returned error: %v", err)
	}
	if n != 5 {
		t.Fatalf("n = %d, want 5", n)
	}
	if out.String() != "hello" {
		t.Fatalf("output = %q, want hello", out.String())
	}
}

func TestCopyWithContextReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	n, err := copyWithContext(ctx, io.Discard, strings.NewReader("hello"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if n != 0 {
		t.Fatalf("n = %d, want 0", n)
	}
}

func TestHeadAndTailLines(t *testing.T) {
	input := "one\ntwo\nthree\n"

	var head bytes.Buffer
	if err := headLines(strings.NewReader(input), &head, 2); err != nil {
		t.Fatalf("headLines returned error: %v", err)
	}
	if head.String() != "one\ntwo\n" {
		t.Fatalf("head output = %q", head.String())
	}

	var tail bytes.Buffer
	if err := tailLines(strings.NewReader(input), &tail, 2); err != nil {
		t.Fatalf("tailLines returned error: %v", err)
	}
	if tail.String() != "two\nthree\n" {
		t.Fatalf("tail output = %q", tail.String())
	}
}

func TestParseRangeList(t *testing.T) {
	tests := []struct {
		name string
		spec string
		max  int
		want []int
	}{
		{name: "single values", spec: "1,3", max: 5, want: []int{0, 2}},
		{name: "closed range", spec: "2-4", max: 5, want: []int{1, 2, 3}},
		{name: "open start", spec: "-2", max: 5, want: []int{0, 1}},
		{name: "open end", spec: "4-", max: 5, want: []int{3, 4}},
		{name: "duplicates sorted", spec: "3,1,3", max: 5, want: []int{0, 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRangeList(tt.spec, tt.max)
			if err != nil {
				t.Fatalf("parseRangeList returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseRangeList = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseRangeListRejectsInvalidSpecs(t *testing.T) {
	for _, spec := range []string{"0", "a", "2-b", "0-2"} {
		t.Run(spec, func(t *testing.T) {
			if _, err := parseRangeList(spec, 5); err == nil {
				t.Fatal("parseRangeList returned nil error")
			}
		})
	}
}

func TestExpandTrSetAndRuneIndex(t *testing.T) {
	set := expandTrSet("a-c[:digit:]")
	got := string(set)
	want := "abc0123456789"
	if got != want {
		t.Fatalf("expandTrSet = %q, want %q", got, want)
	}
	if runeIndex(set, '2') != 5 {
		t.Fatalf("runeIndex('2') = %d, want 5", runeIndex(set, '2'))
	}
	if runeIndex(set, 'z') != -1 {
		t.Fatalf("runeIndex('z') = %d, want -1", runeIndex(set, 'z'))
	}
}

func TestParseDurationAcceptsSecondsAndGoDurations(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{in: "1.5", want: 1500 * time.Millisecond},
		{in: "2s", want: 2 * time.Second},
	}
	for _, tt := range tests {
		got, err := parseDuration(tt.in)
		if err != nil {
			t.Fatalf("parseDuration(%q) returned error: %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("parseDuration(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestExpandEscapeSequences(t *testing.T) {
	got := expandEscapeSequences(`line\n\t\\\x41\101\z`)
	want := "line\n\t\\AA\\z"
	if got != want {
		t.Fatalf("expandEscapeSequences = %q, want %q", got, want)
	}
}

func TestExpandPrintfFormat(t *testing.T) {
	got := expandPrintfFormat("%s:%d:%f:%x:%o:%%:%c:%q\\n", []string{"name", "12", "1.5", "15", "8", "Z", "ignored"})
	want := "name:12:1.5:f:10:%:Z:%q\n"
	if got != want {
		t.Fatalf("expandPrintfFormat = %q, want %q", got, want)
	}
}

func TestLsofSizeHelpers(t *testing.T) {
	formatTests := map[int64]string{
		12:      "12B",
		2 << 10: "2.0K",
		3 << 20: "3.0M",
		4 << 30: "4.0G",
	}
	for size, want := range formatTests {
		if got := formatSize(size); got != want {
			t.Fatalf("formatSize(%d) = %q, want %q", size, got, want)
		}
	}

	parseTests := map[string]int64{
		"7":  7,
		"2K": 2 << 10,
		"3m": 3 << 20,
		"4G": 4 << 30,
	}
	for input, want := range parseTests {
		got, err := parseSize(input)
		if err != nil {
			t.Fatalf("parseSize(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("parseSize(%q) = %d, want %d", input, got, want)
		}
	}

	for _, input := range []string{"", "-1", "bogus"} {
		if _, err := parseSize(input); err == nil {
			t.Fatalf("parseSize(%q) returned nil error", input)
		}
	}
}

func TestLastSegment(t *testing.T) {
	tests := map[string]string{
		"/tmp/file.txt": "file.txt",
		"/tmp/dir/":     "dir",
		"relative":      "relative",
	}
	for input, want := range tests {
		if got := lastSegment(input); got != want {
			t.Fatalf("lastSegment(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMktempGenerateCreatesFileDirectoryAndDryRun(t *testing.T) {
	fs := afero.NewMemMapFs()
	sc := plugins.ShellContext{FS: fs}

	filePath, err := mktempGenerate(sc, "/tmp/file.XXXX", ".txt", false, false)
	if err != nil {
		t.Fatalf("mktempGenerate file returned error: %v", err)
	}
	if !strings.HasPrefix(filePath, "/tmp/file.") || !strings.HasSuffix(filePath, ".txt") {
		t.Fatalf("file path = %q, want /tmp/file.*.txt", filePath)
	}
	info, err := fs.Stat(filePath)
	if err != nil {
		t.Fatalf("created file stat failed: %v", err)
	}
	if info.IsDir() {
		t.Fatal("created path is directory, want file")
	}

	dirPath, err := mktempGenerate(sc, "/var/run.XXX", "", true, false)
	if err != nil {
		t.Fatalf("mktempGenerate dir returned error: %v", err)
	}
	info, err = fs.Stat(dirPath)
	if err != nil {
		t.Fatalf("created dir stat failed: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("created path is file, want directory")
	}

	dryPath, err := mktempGenerate(sc, "/tmp/dry.XXX", "", false, true)
	if err != nil {
		t.Fatalf("mktempGenerate dry-run returned error: %v", err)
	}
	if _, err := fs.Stat(dryPath); err == nil {
		t.Fatalf("dry-run created path %q", dryPath)
	} else if !errors.Is(err, afero.ErrFileNotFound) {
		t.Fatalf("dry-run stat returned error: %v", err)
	}
}

func TestMktempGenerateRejectsInvalidTemplate(t *testing.T) {
	sc := plugins.ShellContext{FS: afero.NewMemMapFs()}
	if _, err := mktempGenerate(sc, "/tmp/no-xs", "", false, false); err == nil {
		t.Fatal("mktempGenerate returned nil error for missing X run")
	}
	if _, err := mktempGenerate(sc, "/tmp/XX", "", false, false); err == nil {
		t.Fatal("mktempGenerate returned nil error for short X run")
	}
}

func TestMktempPathHelpers(t *testing.T) {
	if got := lastXRun("/tmp/file.XXXX"); got != len("/tmp/file.") {
		t.Fatalf("lastXRun = %d, want %d", got, len("/tmp/file."))
	}
	if got := lastXRun("/tmp/file"); got != -1 {
		t.Fatalf("lastXRun without Xs = %d, want -1", got)
	}

	if got := dirOf("/tmp/file"); got != "/tmp" {
		t.Fatalf("dirOf(/tmp/file) = %q, want /tmp", got)
	}
	if got := dirOf("/file"); got != "/" {
		t.Fatalf("dirOf(/file) = %q, want /", got)
	}
	if got := dirOf("file"); got != "" {
		t.Fatalf("dirOf(file) = %q, want empty", got)
	}

	if got := randString(8); len(got) != 8 {
		t.Fatalf("randString length = %d, want 8", len(got))
	}
}
