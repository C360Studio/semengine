package main

import (
	"math"
	"strings"
	"testing"
)

// The tally's arithmetic against ranks worked by hand: answers at 1, 3, 10, 11, 100, 101 and
// absent, seven questions. R@1 counts the 1; R@10 the 1, 3 and 10; the 101 is past the depth
// and the absent one is 0, so neither adds to the reciprocal sum 1 + 1/3 + 1/10 + 1/11 + 1/100.
func TestTallyCountsHandWorkedRanks(t *testing.T) {
	s := &tally{name: "hand"}
	for _, rank := range []int{1, 3, 10, 11, 100, 101, 0} {
		s.add(rank)
	}
	if s.questions != 7 || s.top1 != 1 || s.top10 != 3 {
		t.Errorf("questions %d, top1 %d, top10 %d; want 7, 1, 3", s.questions, s.top1, s.top10)
	}
	const sum = 1 + 1.0/3 + 1.0/10 + 1.0/11 + 1.0/100 // 1.534242...
	if got, want := s.mrr(), sum/7; math.Abs(got-want) > 1e-12 {
		t.Errorf("MRR %.15f, want %.15f", got, want)
	}
	// 1 of 7 is 14.3%, 3 of 7 is 42.9%, 1.534242/7 is 0.219.
	if got := s.String(); !strings.Contains(got, "R@1  14.3%  R@10  42.9%  MRR 0.219  over 7 questions") {
		t.Errorf("String() = %q", got)
	}
}

// A tally with no questions reads 0 everywhere rather than dividing by zero.
func TestEmptyTallyReadsZero(t *testing.T) {
	s := &tally{name: "empty"}
	if s.mrr() != 0 || s.share(s.top10) != 0 {
		t.Errorf("empty tally: MRR %v, R@10 share %v; want 0, 0", s.mrr(), s.share(s.top10))
	}
}

func TestMatchRank(t *testing.T) {
	ids := []string{"a", "b", "c"}
	for id, want := range map[string]int{"a": 1, "c": 3, "z": 0} {
		if got := matchRank(ids, id); got != want {
			t.Errorf("matchRank(%q) = %d, want %d", id, got, want)
		}
	}
}

// rowsOf builds a tally of n questions with top10 answers at rank 1 and the rest absent, whose MRR
// is therefore top10/n.
func rowsOf(name string, n, top10 int) *tally {
	s := &tally{name: name}
	for i := range n {
		if i < top10 {
			s.add(1)
		} else {
			s.add(0)
		}
	}
	return s
}

// The verdict holds at the floor exactly and fails one answer under it, for R@10, for MRR, and for
// a merged R@10 below either single row.
func TestVerdict(t *testing.T) {
	const floorR10, floorMRR = 0.60, 0.40
	cases := []struct {
		name                   string
		merged, words, vectors *tally
		mrrFloor               float64
		want                   string // "" when the floor holds
	}{
		{"at the floor", rowsOf("m", 500, 300), rowsOf("w", 500, 280), rowsOf("v", 500, 260), floorMRR, ""},
		{"one answer under R@10", rowsOf("m", 500, 299), rowsOf("w", 500, 280), rowsOf("v", 500, 260), floorMRR, "R@10 299 of 500, below 60%"},
		// 300 of 500 at rank 1 is MRR 0.600; a floor of 0.601 is above it.
		{"under MRR", rowsOf("m", 500, 300), rowsOf("w", 500, 280), rowsOf("v", 500, 260), 0.601, "MRR 0.6000, below 0.601"},
		{"below words", rowsOf("m", 500, 310), rowsOf("w", 500, 311), rowsOf("v", 500, 260), floorMRR, "below words' 311"},
		{"below vectors", rowsOf("m", 500, 310), rowsOf("w", 500, 300), rowsOf("v", 500, 311), floorMRR, "below vectors' 311"},
		{"level with both", rowsOf("m", 500, 310), rowsOf("w", 500, 310), rowsOf("v", 500, 310), floorMRR, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := verdict(rows{words: tc.words, vectors: tc.vectors, merged: tc.merged}, floorR10, tc.mrrFloor)
			if tc.want == "" {
				if len(got) != 0 {
					t.Errorf("verdict = %q, want it to hold", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Errorf("verdict = %q, want one reason holding %q", got, tc.want)
			}
		})
	}
}
