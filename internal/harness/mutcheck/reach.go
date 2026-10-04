package main

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// reachState is whether the test reached the wrong change (mutation-check › "The changed region
// and reach").
type reachState string

const (
	reached       reachState = "reached"
	notReached    reachState = "not reached"
	notMeasurable reachState = "not measurable"
)

// hunk is one hunk of the line diff of the target against the mutant, with no context lines.
type hunk struct {
	header             string // the @@ line up to its second @@
	oldStart, oldCount int    // in the target's line numbers
	newStart, newCount int    // in the mutant's line numbers
}

// block is one block of a coverage profile.
type block struct {
	startLine, startCol, endLine, endCol, statements, count int
}

// regionResult is one hunk's region and what the reach run's profile shows of it.
type regionResult struct {
	hunk   hunk
	region string // the region in the target's line numbers, in words
	state  reachState
	blocks []block // the blocks that decided it
	note   string
}

var hunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

// parseHunks reads the hunks of a unified diff taken with no context lines. A count the diff
// leaves out is 1.
func parseHunks(diff []byte) ([]hunk, error) {
	var hunks []hunk
	for _, line := range strings.Split(string(diff), "\n") {
		if !strings.HasPrefix(line, "@@") {
			continue
		}
		m := hunkHeader.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("a hunk header that cannot be read: %q", line)
		}
		number := func(s string) int {
			if s == "" {
				return 1
			}
			n, _ := strconv.Atoi(s)
			return n
		}
		hunks = append(hunks, hunk{header: m[0], oldStart: number(m[1]), oldCount: number(m[2]), newStart: number(m[3]), newCount: number(m[4])})
	}
	return hunks, nil
}

// reach judges each hunk's region against the reach run's profile of the unchanged target, and
// the wrong change as a whole: reached when any region is reached, not reached when every region
// is measurable and none is reached, not measurable otherwise. target is the target's source and
// base its file name; regions are in the target's line numbers.
func reach(hunks []hunk, target, profile []byte, base string) (reachState, []regionResult, error) {
	blocks, err := profileBlocks(profile, base)
	if err != nil {
		return "", nil, err
	}
	bodies, err := functionBodies(target)
	if err != nil {
		return "", nil, err
	}
	var results []regionResult
	anyReached, allNotReached := false, len(hunks) > 0
	for _, h := range hunks {
		var r regionResult
		if h.oldCount > 0 {
			r = removedRegion(h, blocks)
		} else {
			r = insertedRegion(h, blocks, bodies)
		}
		anyReached = anyReached || r.state == reached
		allNotReached = allNotReached && r.state == notReached
		results = append(results, r)
	}
	switch {
	case anyReached:
		return reached, results, nil
	case allNotReached:
		return notReached, results, nil
	}
	return notMeasurable, results, nil
}

// regionText is a hunk's region in words, in the target's line numbers.
func regionText(h hunk) string {
	switch last := h.oldStart + h.oldCount - 1; {
	case h.oldCount == 0:
		return fmt.Sprintf("an insertion after target line %d", h.oldStart)
	case last > h.oldStart:
		return fmt.Sprintf("target lines %d-%d", h.oldStart, last)
	}
	return fmt.Sprintf("target line %d", h.oldStart)
}

// removedRegion judges a hunk that removes or replaces target lines: reached when an executed
// block overlaps them.
func removedRegion(h hunk, blocks []block) regionResult {
	first, last := h.oldStart, h.oldStart+h.oldCount-1
	r := regionResult{hunk: h, region: regionText(h)}
	var overlapping []block
	for _, b := range blocks {
		if b.startLine <= last && b.endLine >= first {
			overlapping = append(overlapping, b)
		}
	}
	return judge(r, overlapping, "no block of the profile overlaps it")
}

// insertedRegion judges a hunk that only inserts lines after target line k. Outside every function
// body it is not measurable; inside one, it is reached when an executed block of that body
// contains the position or begins on line k+1, the statement that would run right after it.
func insertedRegion(h hunk, blocks []block, bodies [][2]int) regionResult {
	k := h.oldStart
	r := regionResult{hunk: h, region: regionText(h)}
	body, inside := [2]int{}, false
	for _, b := range bodies {
		if b[0] <= k && k < b[1] && (!inside || b[0] > body[0]) {
			body, inside = b, true
		}
	}
	if !inside {
		r.state, r.note = notMeasurable, "the position is outside every function body"
		return r
	}
	var touching []block
	for _, b := range blocks {
		ofBody := b.startLine >= body[0] && b.endLine <= body[1]
		if ofBody && ((b.startLine <= k && b.endLine >= k+1) || b.startLine == k+1) {
			touching = append(touching, b)
		}
	}
	return judge(r, touching, "no block of its function body contains the position or begins on the line after it")
}

// judge sets a region's state from the blocks that overlap or touch it, keeping those that
// decided it: the executed ones when any ran, all of them otherwise.
func judge(r regionResult, candidates []block, none string) regionResult {
	if len(candidates) == 0 {
		r.state, r.note = notMeasurable, none
		return r
	}
	for _, b := range candidates {
		if b.count > 0 {
			r.blocks = append(r.blocks, b)
		}
	}
	if len(r.blocks) > 0 {
		r.state = reached
		return r
	}
	r.state, r.blocks = notReached, candidates
	return r
}

var profileLine = regexp.MustCompile(`^(.+):([0-9]+)\.([0-9]+),([0-9]+)\.([0-9]+) ([0-9]+) ([0-9]+)$`)

// profileBlocks reads the blocks of the file named base from a coverage profile. The reach run
// covers the target's package only, so the file name identifies the target. A block listed more
// than once has its counts added.
func profileBlocks(profile []byte, base string) ([]block, error) {
	var blocks []block
	index := map[[4]int]int{}
	scanner := bufio.NewScanner(bytes.NewReader(profile))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "mode: ") {
			continue
		}
		m := profileLine.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("a profile line that cannot be read: %q", line)
		}
		if path.Base(m[1]) != base {
			continue
		}
		var n [6]int
		for i := range n {
			n[i], _ = strconv.Atoi(m[i+2])
		}
		key := [4]int{n[0], n[1], n[2], n[3]}
		if i, ok := index[key]; ok {
			blocks[i].count += n[5]
			continue
		}
		index[key] = len(blocks)
		blocks = append(blocks, block{startLine: n[0], startCol: n[1], endLine: n[2], endCol: n[3], statements: n[4], count: n[5]})
	}
	return blocks, scanner.Err()
}

// functionBodies lists the line of the opening and of the closing brace of every function body in
// the target, function literals included, as Go's parser reads it.
func functionBodies(target []byte) ([][2]int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "target.go", target, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("the target could not be parsed: %w", err)
	}
	var bodies [][2]int
	ast.Inspect(file, func(n ast.Node) bool {
		var body *ast.BlockStmt
		switch f := n.(type) {
		case *ast.FuncDecl:
			body = f.Body
		case *ast.FuncLit:
			body = f.Body
		}
		if body != nil {
			bodies = append(bodies, [2]int{fset.Position(body.Lbrace).Line, fset.Position(body.Rbrace).Line})
		}
		return true
	})
	return bodies, nil
}
