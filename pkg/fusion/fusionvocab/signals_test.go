package fusionvocab

import (
	"testing"

	"github.com/c360studio/semengine/vocabulary"
)

// A hand-built ancestor table standing in for semvocab's cco.Parents: each class
// maps to its ancestors up to the root, so the expected specificity is the length
// written here, never something the code under test computes.
const (
	rootClass     = "http://example.org/onto/Entity"
	artifactClass = "http://example.org/onto/Artifact"
	sensorClass   = "http://example.org/onto/Sensor"
)

var testAncestors = map[string][]string{
	rootClass:     nil,
	artifactClass: {"http://example.org/onto/MaterialEntity", rootClass},
	sensorClass:   {artifactClass, "http://example.org/onto/MaterialEntity", rootClass},
}

// TestClassSpecificityCountsParents: the score is the number of IRIs the parents
// function returns for the class; a root or unknown class scores 0, an empty class
// scores 0 without asking, and a subclass outranks its ancestor.
func TestClassSpecificityCountsParents(t *testing.T) {
	var asked []string
	s := New(func(iri string) []string {
		asked = append(asked, iri)
		return testAncestors[iri]
	})
	cases := []struct {
		iri  string
		want float64
	}{
		{rootClass, 0},
		{artifactClass, 2},
		{sensorClass, 3},
		{"http://example.org/onto/NotAClass", 0},
	}
	for _, c := range cases {
		if got := s.ClassSpecificity(c.iri); got != c.want {
			t.Errorf("ClassSpecificity(%q) = %v, want %v", c.iri, got, c.want)
		}
	}
	asked = nil
	if got := s.ClassSpecificity(""); got != 0 || len(asked) != 0 {
		t.Errorf("ClassSpecificity(\"\") = %v after asking %v, want 0 without asking", got, asked)
	}
	if s.ClassSpecificity(sensorClass) <= s.ClassSpecificity(artifactClass) {
		t.Error("a subclass must be more specific than its ancestor")
	}
}

// TestClassSpecificityNilParentsIsZero: with no ontology every class scores 0,
// so ranking falls back to the signals without one.
func TestClassSpecificityNilParentsIsZero(t *testing.T) {
	s := New(nil)
	for _, iri := range []string{"", rootClass, sensorClass} {
		if got := s.ClassSpecificity(iri); got != 0 {
			t.Errorf("New(nil).ClassSpecificity(%q) = %v, want 0", iri, got)
		}
	}
}

// TestPredicateSalience: the registered predicate's signed Weight, 0 when
// unweighted or unregistered, a negative weight included.
func TestPredicateSalience(t *testing.T) {
	t.Cleanup(vocabulary.SnapshotRegistry())

	vocabulary.Register("test.identity.serial", vocabulary.WithWeight(2.5))
	vocabulary.Register("test.artifact.generated", vocabulary.WithWeight(-1.5))
	vocabulary.Register("test.meta.updated") // no weight → 0

	s := New(nil)
	for predicate, want := range map[string]float64{
		"test.identity.serial":    2.5,
		"test.artifact.generated": -1.5,
		"test.meta.updated":       0,
		"test.unregistered.field": 0,
	} {
		if got := s.PredicateSalience(predicate); got != want {
			t.Errorf("PredicateSalience(%q) = %v, want %v", predicate, got, want)
		}
	}
}
