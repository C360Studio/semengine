package main

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

// parseHunks reads the hunks of a unified diff taken with no context lines.
func parseHunks(diff []byte) ([]hunk, error) {
	return nil, nil
}

// reach judges each hunk's region against the reach run's profile of the unchanged target, and
// the wrong change as a whole. target is the target's source; base is its file name.
func reach(hunks []hunk, target, profile []byte, base string) (reachState, []regionResult, error) {
	return "", nil, nil
}
