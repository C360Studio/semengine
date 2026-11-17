package main

import (
	"fmt"
	"slices"

	"github.com/c360studio/semengine/graph"
)

// depth is the most matches each verb is asked for; a rank past it counts as not found. The full
// hundred, not ten: a wrong order inside the first hundred moves the answer out of the first ten,
// which R@10 sees.
const depth = graph.MaxSimilarityLimit

// tally counts where the answers ranked: R@1, R@10 and MRR, counting a rank past depth as not
// found.
type tally struct {
	name                   string
	questions, top1, top10 int
	reciprocal             float64
}

// add counts one question whose answer is at rank, 1-based, or 0 when it was not returned.
func (s *tally) add(rank int) {
	s.questions++
	if rank <= 0 || rank > depth {
		return
	}
	s.reciprocal += 1 / float64(rank)
	if rank == 1 {
		s.top1++
	}
	if rank <= 10 {
		s.top10++
	}
}

func (s *tally) mrr() float64 {
	if s.questions == 0 {
		return 0
	}
	return s.reciprocal / float64(s.questions)
}

func (s *tally) share(n int) float64 {
	if s.questions == 0 {
		return 0
	}
	return 100 * float64(n) / float64(s.questions)
}

func (s *tally) String() string {
	return fmt.Sprintf("%-26s R@1 %5.1f%%  R@10 %5.1f%%  MRR %.3f  over %d questions",
		s.name, s.share(s.top1), s.share(s.top10), s.mrr(), s.questions)
}

// matchRank is id's 1-based place among ids, or 0 when it is not there.
func matchRank(ids []string, id string) int {
	return slices.Index(ids, id) + 1
}
