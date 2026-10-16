package vocabulary

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// TestParsePredicateDoesNotAllocate gives every run, the warm-up included, a valid predicate
// the process has not parsed before, so no cache can answer it (vocabulary-packs spec, "A
// predicate never seen before"). The predicates are built before the measured function runs.
// Not parallel: testing.AllocsPerRun counts every allocation in the process.
func TestParsePredicateDoesNotAllocate(t *testing.T) {
	const runs = 100
	fresh := make([]string, runs+1) // AllocsPerRun calls the function once more as a warm-up
	for i := range fresh {
		fresh[i] = fmt.Sprintf("alloc.fresh.p%d", i)
	}

	next := 0
	allocs := testing.AllocsPerRun(runs, func() {
		predicate := fresh[next]
		next++
		if _, err := ParsePredicate(predicate); err != nil {
			t.Fatalf("ParsePredicate(%q): %v", predicate, err)
		}
	})
	if next != len(fresh) {
		t.Fatalf("parsed %d predicates, want %d: a run reused one", next, len(fresh))
	}
	if allocs != 0 {
		t.Fatalf("ParsePredicate of a predicate not parsed before allocated %.1f objects per run, want 0", allocs)
	}
}

// predicateGrammarSeeds holds one input per refusal reason, a valid predicate, an input with
// several faults whose reported reason the parse order decides, and the inputs either side of
// the segment and total byte limits. want is the reason the spec's order gives, "" for accept.
var predicateGrammarSeeds = []struct {
	predicate string
	want      PredicateValidationReason
}{
	{"agent.run.entity-id", ""},
	{"", PredicateReasonEmpty},
	{strings.Repeat("a", 64) + "." + strings.Repeat("a", 64) + "." + strings.Repeat("a", 65), PredicateReasonLength},
	{"a.b", PredicateReasonArity},
	{"a..b", PredicateReasonSegmentEmpty},
	{"a.b." + strings.Repeat("a", 65), PredicateReasonSegmentLength},
	{"a.B.c", PredicateReasonSegmentStart},
	{"a.b_c.d", PredicateReasonSegmentCharacter},
	{"a.b-.c", PredicateReasonSegmentHyphen},
	// Four segments, the second empty: arity is reported, not the empty segment.
	{"a..b.c", PredicateReasonArity},
	{"a.b." + strings.Repeat("a", 64), ""},
	{strings.Repeat("a", 64) + "." + strings.Repeat("a", 64) + "." + strings.Repeat("a", 64), ""},
}

// predicateSegmentGrammar is ADR-074's segment rule, written by the test, not taken from the
// parser.
var predicateSegmentGrammar = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// grammarAccepts reports whether predicate is in ADR-074's grammar, and its parts if it is.
func grammarAccepts(predicate string) (PredicateParts, bool) {
	if len(predicate) > MaxPredicateBytes {
		return PredicateParts{}, false
	}
	segments := strings.Split(predicate, ".")
	if len(segments) != 3 {
		return PredicateParts{}, false
	}
	for _, segment := range segments {
		if len(segment) > MaxPredicateSegmentBytes || !predicateSegmentGrammar.MatchString(segment) {
			return PredicateParts{}, false
		}
	}
	return PredicateParts{Domain: segments[0], Category: segments[1], Property: segments[2]}, true
}

// parsePredicateBySplit is ParsePredicate as it was before this change, without its memo. It is
// the oracle for which reason and segment a refusal reports: the order of reasons for an input
// with several faults is the one this code defined. It shares validatePredicateSegment, which
// the change leaves as it is; grammarAccepts checks segments independently.
func parsePredicateBySplit(predicate string) (PredicateParts, error) {
	if predicate == "" {
		return PredicateParts{}, newPredicateValidationError(predicate, PredicateReasonEmpty, -1)
	}
	if len(predicate) > MaxPredicateBytes {
		return PredicateParts{}, newPredicateValidationError(predicate, PredicateReasonLength, -1)
	}
	segments := strings.Split(predicate, ".")
	if len(segments) != 3 {
		return PredicateParts{}, newPredicateValidationError(predicate, PredicateReasonArity, -1)
	}
	for i, segment := range segments {
		if err := validatePredicateSegment(predicate, segment, i); err != nil {
			return PredicateParts{}, err
		}
	}
	return PredicateParts{Domain: segments[0], Category: segments[1], Property: segments[2]}, nil
}

// TestPredicateGrammarSeedsCoverEveryReason shows the fuzz target's seeds reach every reason, so
// its reason comparison runs on each one under plain go test.
func TestPredicateGrammarSeedsCoverEveryReason(t *testing.T) {
	reached := map[PredicateValidationReason]bool{}
	for _, seed := range predicateGrammarSeeds {
		_, err := parsePredicateBySplit(seed.predicate)
		var got PredicateValidationReason
		var validationErr *PredicateValidationError
		if errors.As(err, &validationErr) {
			got = validationErr.Reason
		}
		if got != seed.want {
			t.Fatalf("seed %q: reason %q, want %q", seed.predicate, got, seed.want)
		}
		reached[got] = true
	}
	for _, reason := range []PredicateValidationReason{
		"", PredicateReasonEmpty, PredicateReasonLength, PredicateReasonArity,
		PredicateReasonSegmentEmpty, PredicateReasonSegmentLength, PredicateReasonSegmentStart,
		PredicateReasonSegmentCharacter, PredicateReasonSegmentHyphen,
	} {
		if !reached[reason] {
			t.Errorf("no seed reaches reason %q", reason)
		}
	}
}

// FuzzParsePredicateAgreesWithGrammar checks two rules on every input (vocabulary-packs spec,
// "Agreement with the grammar"): ParsePredicate accepts exactly the grammar, with the same
// parts; and a refusal is a *PredicateValidationError equal to the pre-change parse's.
func FuzzParsePredicateAgreesWithGrammar(f *testing.F) {
	for _, seed := range predicateGrammarSeeds {
		f.Add(seed.predicate)
	}

	f.Fuzz(func(t *testing.T, predicate string) {
		got, err := ParsePredicate(predicate)
		want, inGrammar := grammarAccepts(predicate)
		if inGrammar != (err == nil) {
			t.Fatalf("ParsePredicate(%q) error = %v, grammar accepts = %t", predicate, err, inGrammar)
		}
		if inGrammar {
			if got != want {
				t.Fatalf("ParsePredicate(%q) = %#v, grammar gives %#v", predicate, got, want)
			}
			return
		}

		var gotErr *PredicateValidationError
		if !errors.As(err, &gotErr) {
			t.Fatalf("ParsePredicate(%q) error = %T %v, want *PredicateValidationError", predicate, err, err)
		}
		_, baseErr := parsePredicateBySplit(predicate)
		var wantErr *PredicateValidationError
		if !errors.As(baseErr, &wantErr) {
			t.Fatalf("parsePredicateBySplit(%q) error = %T %v, want *PredicateValidationError", predicate, baseErr, baseErr)
		}
		if *gotErr != *wantErr {
			t.Fatalf("ParsePredicate(%q) refused with %+v, the pre-change parse with %+v", predicate, *gotErr, *wantErr)
		}
	})
}

// BenchmarkParsePredicate parses one valid predicate repeatedly. Its numbers are recorded as
// information only (design D7); they make no speed claim.
func BenchmarkParsePredicate(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ParsePredicate("agent.run.entity-id"); err != nil {
			b.Fatal(err)
		}
	}
}
