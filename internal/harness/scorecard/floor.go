package main

import "fmt"

// Tier 1's floor, held by every model: of the 500 questions, searchGraph's merged list has the
// answer in its first ten at least floorRecallAt10 of the time, with an MRR of at least floorMRR,
// and in its first ten no less often than globalSearch's words or the semantic verb's vectors.
// Measured from the setup-04c-02-hybrid-search change's
// notes/probes/scorecard-snowflake-arctic-embed-s-6f69a4e2.md: Snowflake/snowflake-arctic-embed-s through semembed
// merged R@10 61.6% and MRR 0.410, against words 56.0% and vectors 52.0%.
const (
	floorRecallAt10 = 0.60
	floorMRR        = 0.40
)

// verdict lists why the merged row falls short of a floor of recallFloor and mrrFloor, or nothing
// when it holds. R@10 is compared in answers, so the floor's share of the questions is a count.
func verdict(r rows, recallFloor, mrrFloor float64) []string {
	var short []string
	m := r.merged
	if float64(m.top10) < recallFloor*float64(m.questions) {
		short = append(short, fmt.Sprintf("merged R@10 %d of %d, below %.0f%%", m.top10, m.questions, 100*recallFloor))
	}
	if m.mrr() < mrrFloor {
		short = append(short, fmt.Sprintf("merged MRR %.4f, below %.3f", m.mrr(), mrrFloor))
	}
	for _, single := range []struct {
		name string
		row  *tally
	}{{"words'", r.words}, {"vectors'", r.vectors}} {
		if m.top10 < single.row.top10 {
			short = append(short, fmt.Sprintf("merged R@10 %d, below %s %d", m.top10, single.name, single.row.top10))
		}
	}
	return short
}
