package native

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type DiffPlugin struct{}

func (DiffPlugin) Name() string        { return "diff" }
func (DiffPlugin) Description() string { return "compare two files line by line" }
func (DiffPlugin) Usage() string       { return "diff [-u] <file1> <file2>" }

func (DiffPlugin) Run(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	sc := plugins.ShellCtx(ctx)

	unified := false
	ctxLines := 3
	ignoreCase := false
	ignoreSpace := false
	ignoreAllSpace := false
	quiet := false
	color := false
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
		case "--unified":
			unified = true
			continue
		case "--ignore-case":
			ignoreCase = true
			continue
		case "--ignore-space-change":
			ignoreSpace = true
			continue
		case "--ignore-all-space":
			ignoreAllSpace = true
			continue
		case "--quiet", "--brief":
			quiet = true
			continue
		case "--color", "--color=always", "--color=auto":
			color = true
			continue
		case "--color=never":
			color = false
			continue
		}
		if a == "-U" && i+1 < len(args) {
			fmt.Sscanf(args[i+1], "%d", &ctxLines)
			i++
			unified = true
			continue
		}
		if len(a) > 2 && strings.HasPrefix(a, "-U") {
			fmt.Sscanf(a[2:], "%d", &ctxLines)
			unified = true
			continue
		}
		unknown := ""
		for _, c := range a[1:] {
			switch c {
			case 'u':
				unified = true
			case 'i':
				ignoreCase = true
			case 'b':
				ignoreSpace = true
			case 'w':
				ignoreAllSpace = true
			case 'q':
				quiet = true
			default:
				unknown += string(c)
			}
		}
		if unknown != "" {
			fmt.Fprintf(hc.Stderr, "diff: invalid option -- '%s'\n", unknown)
			return interp.ExitStatus(2)
		}
	}

	if len(files) < 2 {
		fmt.Fprintln(hc.Stderr, "diff: missing operand")
		return interp.ExitStatus(2)
	}

	readFileLines := func(path string) ([]string, error) {
		f, err := sc.FS.Open(sc.ResolvePath(path))
		if err != nil {
			return nil, fmt.Errorf("diff: %s: %w", path, err)
		}
		defer f.Close()

		var lines []string
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		return lines, scanner.Err()
	}

	normalise := func(line string) string {
		if ignoreAllSpace {
			var b strings.Builder
			for _, c := range line {
				if c != ' ' && c != '\t' {
					b.WriteRune(c)
				}
			}
			line = b.String()
		} else if ignoreSpace {
			var b strings.Builder
			prevSpace := false
			for _, c := range line {
				if c == ' ' || c == '\t' {
					if !prevSpace {
						b.WriteByte(' ')
						prevSpace = true
					}
					continue
				}
				prevSpace = false
				b.WriteRune(c)
			}
			line = strings.TrimRight(b.String(), " ")
		}
		if ignoreCase {
			line = strings.ToLower(line)
		}
		return line
	}

	rawA, err := readFileLines(files[0])
	if err != nil {
		return err
	}
	rawB, err := readFileLines(files[1])
	if err != nil {
		return err
	}

	normA := make([]string, len(rawA))
	normB := make([]string, len(rawB))
	for i, line := range rawA {
		normA[i] = normalise(line)
	}
	for i, line := range rawB {
		normB[i] = normalise(line)
	}

	ops := diffLCS(normA, normB)
	changed := false
	for _, op := range ops {
		if op.kind != diffKeep {
			changed = true
			break
		}
	}
	if !changed {
		return nil
	}
	if quiet {
		fmt.Fprintf(hc.Stdout, "Files %s and %s differ\n", files[0], files[1])
		return interp.ExitStatus(1)
	}

	ansiRed := "\033[31m"
	ansiGreen := "\033[32m"
	ansiCyan := "\033[36m"
	ansiBold := "\033[1m"
	ansiReset := "\033[0m"
	col := func(code, text string) string {
		if !color {
			return text
		}
		return code + text + ansiReset
	}

	if unified {
		fmt.Fprintln(hc.Stdout, col(ansiBold, "--- "+files[0]))
		fmt.Fprintln(hc.Stdout, col(ansiBold, "+++ "+files[1]))
		for i := 0; i < len(ops); {
			if ops[i].kind == diffKeep {
				i++
				continue
			}

			start := i - ctxLines
			if start < 0 {
				start = 0
			}
			end := i
			for end < len(ops) {
				if ops[end].kind != diffKeep {
					lookahead := end + 1
					for lookahead < len(ops) && lookahead <= end+ctxLines {
						if ops[lookahead].kind != diffKeep {
							end = lookahead
							break
						}
						lookahead++
					}
					if lookahead > end+ctxLines || lookahead >= len(ops) {
						end++
						break
					}
				}
				end++
			}
			hunkEnd := end + ctxLines - 1
			if hunkEnd >= len(ops) {
				hunkEnd = len(ops) - 1
			}

			hunkOps := ops[start : hunkEnd+1]
			aStart, bStart := -1, -1
			aCount, bCount := 0, 0
			for _, op := range hunkOps {
				switch op.kind {
				case diffKeep:
					if aStart == -1 {
						aStart = op.aIdx + 1
						bStart = op.bIdx + 1
					}
					aCount++
					bCount++
				case diffDelete:
					if aStart == -1 {
						aStart = op.aIdx + 1
					}
					aCount++
				case diffInsert:
					if bStart == -1 {
						bStart = op.bIdx + 1
					}
					bCount++
				}
			}
			if aStart == -1 {
				aStart = 1
			}
			if bStart == -1 {
				bStart = 1
			}

			aRange := fmt.Sprintf("%d", aStart)
			if aCount != 1 {
				aRange = fmt.Sprintf("%d,%d", aStart, aCount)
			}
			bRange := fmt.Sprintf("%d", bStart)
			if bCount != 1 {
				bRange = fmt.Sprintf("%d,%d", bStart, bCount)
			}

			fmt.Fprintln(hc.Stdout, col(ansiCyan, fmt.Sprintf("@@ -%s +%s @@", aRange, bRange)))
			for _, op := range hunkOps {
				switch op.kind {
				case diffKeep:
					fmt.Fprintln(hc.Stdout, " "+rawA[op.aIdx])
				case diffDelete:
					fmt.Fprintln(hc.Stdout, col(ansiRed, "-"+rawA[op.aIdx]))
				case diffInsert:
					fmt.Fprintln(hc.Stdout, col(ansiGreen, "+"+rawB[op.bIdx]))
				}
			}
			i = hunkEnd + 1
		}
		return interp.ExitStatus(1)
	}

	for i := 0; i < len(ops); {
		if ops[i].kind == diffKeep {
			i++
			continue
		}
		j := i
		for j < len(ops) && ops[j].kind != diffKeep {
			j++
		}
		chunk := ops[i:j]

		aStart, aEnd := -1, -1
		bStart, bEnd := -1, -1
		for _, op := range chunk {
			switch op.kind {
			case diffDelete:
				if aStart == -1 {
					aStart = op.aIdx + 1
				}
				aEnd = op.aIdx + 1
			case diffInsert:
				if bStart == -1 {
					bStart = op.bIdx + 1
				}
				bEnd = op.bIdx + 1
			}
		}

		hasDelete := aStart != -1
		hasInsert := bStart != -1
		rangeA := ""
		rangeB := ""
		if hasDelete {
			rangeA = fmt.Sprintf("%d", aStart)
			if aStart != aEnd {
				rangeA = fmt.Sprintf("%d,%d", aStart, aEnd)
			}
		}
		if hasInsert {
			rangeB = fmt.Sprintf("%d", bStart)
			if bStart != bEnd {
				rangeB = fmt.Sprintf("%d,%d", bStart, bEnd)
			}
		}

		header := ""
		switch {
		case hasDelete && hasInsert:
			header = rangeA + "c" + rangeB
		case hasDelete:
			lineB := 0
			for k := i - 1; k >= 0; k-- {
				if ops[k].kind == diffKeep {
					lineB = ops[k].bIdx + 1
					break
				}
			}
			header = rangeA + "d" + fmt.Sprintf("%d", lineB)
		case hasInsert:
			lineA := 0
			for k := i - 1; k >= 0; k-- {
				if ops[k].kind == diffKeep {
					lineA = ops[k].aIdx + 1
					break
				}
			}
			header = fmt.Sprintf("%d", lineA) + "a" + rangeB
		}
		fmt.Fprintln(hc.Stdout, col(ansiCyan, header))
		if hasDelete {
			for _, op := range chunk {
				if op.kind == diffDelete {
					fmt.Fprintln(hc.Stdout, col(ansiRed, "< "+rawA[op.aIdx]))
				}
			}
		}
		if hasDelete && hasInsert {
			fmt.Fprintln(hc.Stdout, "---")
		}
		if hasInsert {
			for _, op := range chunk {
				if op.kind == diffInsert {
					fmt.Fprintln(hc.Stdout, col(ansiGreen, "> "+rawB[op.bIdx]))
				}
			}
		}
		i = j
	}

	return interp.ExitStatus(1)
}

type diffOpKind int

const (
	diffKeep diffOpKind = iota
	diffDelete
	diffInsert
)

type diffOp struct {
	kind diffOpKind
	aIdx int
	bIdx int
}

func diffLCS(a, b []string) []diffOp {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	ops := make([]diffOp, 0, m+n)
	for i, j := m, n; i > 0 || j > 0; {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			i--
			j--
			ops = append(ops, diffOp{kind: diffKeep, aIdx: i, bIdx: j})
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			j--
			ops = append(ops, diffOp{kind: diffInsert, aIdx: i, bIdx: j})
		default:
			i--
			ops = append(ops, diffOp{kind: diffDelete, aIdx: i, bIdx: j})
		}
	}
	for l, r := 0, len(ops)-1; l < r; l, r = l+1, r-1 {
		ops[l], ops[r] = ops[r], ops[l]
	}
	return ops
}

var _ plugins.PluginInfo = DiffPlugin{}
