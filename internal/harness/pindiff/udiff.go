package main

import (
	"fmt"
	"strings"
)

// diffContext is the number of unchanged lines printed around a change, as in `diff -u`.
const diffContext = 3

// maxLCSCells caps the table of the line matching below. Past it the changed middle of the two
// files is printed as one removal and one addition: still a correct unified diff, not a minimal one.
const maxLCSCells = 4 << 20

type diffLine struct {
	kind byte // ' ', '-' or '+'
	text string
}

// unifiedDiff renders the difference from a to b in unified format with the given labels. Lines
// keep their newline, so a last line without one differs from the same text with one and is
// marked as `diff -u` marks it. approximate reports that the changed middle was too large to match
// line by line and is printed as one removal and one addition.
func unifiedDiff(fromLabel, toLabel string, a, b []byte) (text string, approximate bool) {
	ops, approximate := editScript(splitLines(string(a)), splitLines(string(b)))
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", fromLabel, toLabel)

	// aSeen[k] and bSeen[k] count the lines of a and b before ops[k].
	aSeen, bSeen := make([]int, len(ops)+1), make([]int, len(ops)+1)
	for k, op := range ops {
		aSeen[k+1], bSeen[k+1] = aSeen[k], bSeen[k]
		if op.kind != '+' {
			aSeen[k+1]++
		}
		if op.kind != '-' {
			bSeen[k+1]++
		}
	}
	for i := 0; i < len(ops); {
		for i < len(ops) && ops[i].kind == ' ' {
			i++
		}
		if i == len(ops) {
			break
		}
		start, end := max(i-diffContext, 0), i
		for {
			for end < len(ops) && ops[end].kind != ' ' {
				end++
			}
			next := end
			for next < len(ops) && ops[next].kind == ' ' {
				next++
			}
			if next < len(ops) && next-end <= 2*diffContext {
				end = next
				continue
			}
			end = min(end+diffContext, next)
			break
		}
		fmt.Fprintf(&out, "@@ -%s +%s @@\n", hunkRange(aSeen[start], aSeen[end]), hunkRange(bSeen[start], bSeen[end]))
		for _, op := range ops[start:end] {
			out.WriteByte(op.kind)
			out.WriteString(strings.TrimSuffix(op.text, "\n"))
			out.WriteByte('\n')
			if !strings.HasSuffix(op.text, "\n") {
				out.WriteString("\\ No newline at end of file\n")
			}
		}
		i = end
	}
	return out.String(), approximate
}

// hunkRange prints a hunk's line range as `diff -u` does: the count is left out when it is one,
// and an empty range names the line before it.
func hunkRange(before, after int) string {
	switch n := after - before; n {
	case 0:
		return fmt.Sprintf("%d,0", before)
	case 1:
		return fmt.Sprint(before + 1)
	default:
		return fmt.Sprintf("%d,%d", before+1, n)
	}
}

func splitLines(s string) []string {
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// editScript matches the lines of a and b: common prefix and suffix first, then a longest common
// subsequence of the middle.
func editScript(a, b []string) ([]diffLine, bool) {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	var ops []diffLine
	for _, l := range a[:p] {
		ops = append(ops, diffLine{' ', l})
	}
	middle, approximate := matchMiddle(a[p:len(a)-s], b[p:len(b)-s])
	ops = append(ops, middle...)
	for _, l := range a[len(a)-s:] {
		ops = append(ops, diffLine{' ', l})
	}
	return ops, approximate
}

func matchMiddle(a, b []string) ([]diffLine, bool) {
	var ops []diffLine
	n, m := len(a), len(b)
	if (n+1)*(m+1) > maxLCSCells {
		for _, l := range a {
			ops = append(ops, diffLine{'-', l})
		}
		for _, l := range b {
			ops = append(ops, diffLine{'+', l})
		}
		return ops, true
	}
	// lcs[i*(m+1)+j] is the length of a longest common subsequence of a[i:] and b[j:].
	lcs := make([]int32, (n+1)*(m+1))
	at := func(i, j int) int32 { return lcs[i*(m+1)+j] }
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i*(m+1)+j] = at(i+1, j+1) + 1
			} else {
				lcs[i*(m+1)+j] = max(at(i+1, j), at(i, j+1))
			}
		}
	}
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			ops = append(ops, diffLine{' ', a[i]})
			i, j = i+1, j+1
		case j == m || i < n && at(i+1, j) >= at(i, j+1):
			ops = append(ops, diffLine{'-', a[i]})
			i++
		default:
			ops = append(ops, diffLine{'+', b[j]})
			j++
		}
	}
	return ops, false
}
