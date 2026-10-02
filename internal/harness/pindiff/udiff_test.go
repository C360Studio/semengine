package main

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func lines(from, to int, replace map[int]string) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		if r, ok := replace[i]; ok {
			b.WriteString(r + "\n")
			continue
		}
		fmt.Fprintf(&b, "%d\n", i)
	}
	return b.String()
}

// The unified diff a reviewer reads, held to `diff -u`'s format by examples written out by hand
// (and checked against `diff -u` once when they were written): changes six unchanged lines apart
// share a hunk and seven apart do not; an empty range is named by the line before it; a last line
// without a newline is marked.
func TestUnifiedDiff(t *testing.T) {
	for _, tc := range []struct{ name, a, b, want string }{
		{"six lines apart, one hunk", lines(1, 12, nil), lines(1, 12, map[int]string{2: "two", 9: "nine"}),
			"@@ -1,12 +1,12 @@\n 1\n-2\n+two\n 3\n 4\n 5\n 6\n 7\n 8\n-9\n+nine\n 10\n 11\n 12\n"},
		{"seven lines apart, two hunks", lines(1, 13, nil), lines(1, 13, map[int]string{2: "two", 10: "ten"}),
			"@@ -1,5 +1,5 @@\n 1\n-2\n+two\n 3\n 4\n 5\n" +
				"@@ -7,7 +7,7 @@\n 7\n 8\n 9\n-10\n+ten\n 11\n 12\n 13\n"},
		{"empty old range", "", "x\ny\n", "@@ -0,0 +1,2 @@\n+x\n+y\n"},
		{"empty new range", "x\ny\n", "", "@@ -1,2 +0,0 @@\n-x\n-y\n"},
		{"no final newline", "a\nb", "A\nb", "@@ -1,2 +1,2 @@\n-a\n+A\n b\n\\ No newline at end of file\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, approximate := unifiedDiff("old", "new", []byte(tc.a), []byte(tc.b))
			want := "--- old\n+++ new\n" + tc.want
			if got != want || approximate {
				t.Fatalf("got (approximate %v):\n%s\nwant:\n%s", approximate, got, want)
			}
		})
	}
}

// fuzzText turns fuzz bytes into a text of short lines drawn from five values, so the two sides
// share lines and the diffs have context, gaps and several hunks; trim drops the final newline.
func fuzzText(data []byte, trim bool) string {
	var b strings.Builder
	for _, c := range data {
		b.WriteString(string(rune('a'+c%5)) + "\n")
	}
	s := b.String()
	if trim {
		s = strings.TrimSuffix(s, "\n")
	}
	return s
}

// FuzzUnifiedDiffApplies checks the law the printed diff exists for: applied to the pin text it
// gives the tree text. The applier below is the test's own reference model of a unified diff; it
// checks every header's start and count and every context and removed line against the old text.
// The assertion applies a hunk whenever the two texts differ, which every seed but the last does.
func FuzzUnifiedDiffApplies(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 0, 1, 2, 3, 4, 0, 1}, []byte{0, 4, 2, 3, 4, 0, 1, 2, 2, 4, 0, 1}, false, false) // one merged hunk
	f.Add([]byte{0, 1, 2, 3, 4, 0, 1, 2, 3, 4, 0, 1, 2}, []byte{0, 4, 2, 3, 4, 0, 1, 2, 3, 0, 0, 1, 2}, false, false)
	f.Add([]byte{0, 1, 2, 3, 4, 0, 1, 2}, []byte{0, 4, 2, 3, 0, 0, 1, 2}, false, false) // two changes three lines apart
	f.Add([]byte{}, []byte{1, 2}, false, false)                                         // empty old range
	f.Add([]byte{1, 2}, []byte{}, false, false)                                         // empty new range
	f.Add([]byte{0, 1}, []byte{3, 1}, true, true)                                       // no final newline on either side
	f.Add([]byte{0, 1}, []byte{0, 1}, false, true)                                      // only the final newline differs
	f.Add([]byte{0, 1, 2}, []byte{0, 1, 2}, false, false)                               // equal: no hunk at all
	f.Fuzz(func(t *testing.T, a, b []byte, trimA, trimB bool) {
		oldText, newText := fuzzText(a, trimA), fuzzText(b, trimB)
		diff, _ := unifiedDiff("old", "new", []byte(oldText), []byte(newText))
		got, err := applyUnified(oldText, diff)
		if err != nil {
			t.Fatalf("apply: %v\nold %q\nnew %q\ndiff:\n%s", err, oldText, newText, diff)
		}
		if got != newText {
			t.Fatalf("applied diff gives %q, want %q\ndiff:\n%s", got, newText, diff)
		}
	})
}

// applyUnified applies a unified diff to old. It is written from the format, not from unifiedDiff.
func applyUnified(old, diff string) (string, error) {
	src := splitKeep(old)
	body := strings.SplitAfter(diff, "\n")
	if len(body) < 2 || !strings.HasPrefix(body[0], "--- ") || !strings.HasPrefix(body[1], "+++ ") {
		return "", fmt.Errorf("no file header")
	}
	var out []string
	pos := 0 // lines of old consumed
	for i := 2; i < len(body) && body[i] != ""; {
		var oStart, oc, ns, nc int
		if !parseHunkHeader(body[i], &oStart, &oc, &ns, &nc) {
			return "", fmt.Errorf("bad hunk header %q", body[i])
		}
		i++
		startOld := oStart - 1
		if oc == 0 {
			startOld = oStart
		}
		if startOld < pos || startOld > len(src) {
			return "", fmt.Errorf("hunk at old line %d overlaps or is out of range", oStart)
		}
		out = append(out, src[pos:startOld]...)
		pos = startOld
		wantNew := ns - 1
		if nc == 0 {
			wantNew = ns
		}
		if len(out) != wantNew {
			return "", fmt.Errorf("hunk new start %d, but %d lines precede it", ns, len(out))
		}
		seenOld, seenNew := 0, 0
		for i < len(body) && body[i] != "" && !strings.HasPrefix(body[i], "@@") {
			line := body[i]
			i++
			if strings.HasPrefix(line, "\\ No newline at end of file") {
				continue
			}
			text := line[1:]
			if i < len(body) && strings.HasPrefix(body[i], "\\ No newline at end of file") {
				text = strings.TrimSuffix(text, "\n")
			}
			switch line[0] {
			case ' ', '-':
				if pos >= len(src) || src[pos] != text {
					return "", fmt.Errorf("line %d of old is not %q", pos+1, text)
				}
				pos++
				seenOld++
				if line[0] == ' ' {
					out = append(out, text)
					seenNew++
				}
			case '+':
				out = append(out, text)
				seenNew++
			default:
				return "", fmt.Errorf("bad hunk line %q", line)
			}
		}
		if seenOld != oc || seenNew != nc {
			return "", fmt.Errorf("hunk counts -%d +%d, body has -%d +%d", oc, nc, seenOld, seenNew)
		}
	}
	out = append(out, src[pos:]...)
	return strings.Join(out, ""), nil
}

func parseHunkHeader(h string, oStart, oc, ns, nc *int) bool {
	var oldR, newR string
	if _, err := fmt.Sscanf(h, "@@ -%s +%s @@\n", &oldR, &newR); err != nil {
		return false
	}
	return parseRange(oldR, oStart, oc) && parseRange(newR, ns, nc)
}

func parseRange(r string, start, count *int) bool {
	s, c, found := strings.Cut(r, ",")
	var err1, err2 error
	*start, err1 = strconv.Atoi(s)
	*count = 1
	if found {
		*count, err2 = strconv.Atoi(c)
	}
	return err1 == nil && err2 == nil
}

func splitKeep(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.SplitAfter(s, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
