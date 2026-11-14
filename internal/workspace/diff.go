package workspace

import (
	"fmt"
	"strings"
)

// maxDiffCells bounds the work of the line diff (lines of a times lines of
// b); bigger files are reported as changed without a diff.
const maxDiffCells = 4_000_000

// Diff returns a unified diff from a to b with three lines of context, or ""
// when they are equal. name labels both sides.
func Diff(name string, a, b []byte) string {
	if string(a) == string(b) {
		return ""
	}
	x, y := splitLines(string(a)), splitLines(string(b))
	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", name, name)
	if len(x)*len(y) > maxDiffCells {
		fmt.Fprintf(&out, "@@ too large to show: %d lines before, %d after @@\n", len(x), len(y))
		return out.String()
	}
	ops := lineOps(x, y)
	for _, h := range hunks(ops, 3) {
		writeHunk(&out, h)
	}
	return out.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type op struct {
	kind byte // ' ', '-', '+'
	line string
	// ai and bi are the 1-based line numbers in a and b.
	ai, bi int
}

// lineOps computes an edit script with a longest common subsequence table.
func lineOps(x, y []string) []op {
	n, m := len(x), len(y)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var ops []op
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && x[i] == y[j]:
			ops = append(ops, op{' ', x[i], i + 1, j + 1})
			i++
			j++
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			// Deletions come before additions, as diff(1) shows them.
			ops = append(ops, op{'-', x[i], i + 1, j + 1})
			i++
		default:
			ops = append(ops, op{'+', y[j], i + 1, j + 1})
			j++
		}
	}
	return ops
}

// hunks groups changes with ctx lines of context around them.
func hunks(ops []op, ctx int) [][]op {
	var out [][]op
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		start := max(i-ctx, 0)
		end := i
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			// Stop when the next change is more than 2*ctx lines away.
			next := end
			for next < len(ops) && ops[next].kind == ' ' {
				next++
			}
			if next == len(ops) || next-end > 2*ctx {
				end = min(end+ctx, len(ops))
				break
			}
			end = next
		}
		out = append(out, ops[start:end])
		i = end
	}
	return out
}

func writeHunk(out *strings.Builder, h []op) {
	var aStart, bStart, aLen, bLen int
	for _, o := range h {
		if o.kind != '+' {
			if aLen == 0 {
				aStart = o.ai
			}
			aLen++
		}
		if o.kind != '-' {
			if bLen == 0 {
				bStart = o.bi
			}
			bLen++
		}
	}
	if aLen == 0 {
		aStart = h[0].ai - 1
	}
	if bLen == 0 {
		bStart = h[0].bi - 1
	}
	fmt.Fprintf(out, "@@ -%d,%d +%d,%d @@\n", aStart, aLen, bStart, bLen)
	for _, o := range h {
		out.WriteByte(o.kind)
		out.WriteString(o.line)
		if !strings.HasSuffix(o.line, "\n") {
			out.WriteString("\n\\ No newline at end of file\n")
		}
	}
}
