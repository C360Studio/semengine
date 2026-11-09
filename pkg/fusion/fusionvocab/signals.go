// Package fusionvocab is the production implementation of fusion.RankSignals
// (ADR-062 increment 5, gh#396): it wires the lens engine's ranking signals onto
// the vocabulary registry (predicate salience) and an ontology's ancestor
// function (class specificity).
//
// It is a SUBPACKAGE of pkg/fusion deliberately: pkg/fusion stays free of the
// vocabulary packages, so the engine stays deterministic and unit-testable
// without them. It imports no ontology: the BFO/CCO hierarchy lives in semvocab,
// which depends on SemEngine, so the caller passes semvocab's ancestor function
// (for example cco.Parents) to New (design D6).
package fusionvocab

import (
	"github.com/c360studio/semengine/pkg/fusion"
	"github.com/c360studio/semengine/vocabulary"
)

// Signals implements fusion.RankSignals: class specificity from the ancestor
// function given to New, predicate salience from the vocabulary registry. It
// holds only that function, so it is safe to share and pass by value as long as
// the function is.
type Signals struct {
	parents func(iri string) []string
}

// Compile-time assertion that Signals satisfies the engine's ranking surface.
var _ fusion.RankSignals = Signals{}

// New returns the ranking signals over parents, a function that returns a class
// IRI's ancestors up to the root (semvocab's cco.Parents resolves both CCO and
// BFO IRIs). A nil parents means no ontology: every class then scores 0, and
// ranking is as without the ontology signal.
func New(parents func(iri string) []string) Signals {
	return Signals{parents: parents}
}

// ClassSpecificity scores an ontology class IRI by the number of ancestors the
// parents function returns, so a more specific class ranks higher. An empty IRI,
// or a nil parents function, scores 0; an unknown IRI scores what parents
// returns for it.
func (s Signals) ClassSpecificity(classIRI string) float64 {
	if classIRI == "" || s.parents == nil {
		return 0
	}
	return float64(len(s.parents(classIRI)))
}

// PredicateSalience returns a predicate's stored salience weight from the
// vocabulary registry (0 when unregistered or unweighted). Signed: a negative
// weight down-ranks (gh#441) — a faithful pass-through of PredicateMetadata.Weight.
func (Signals) PredicateSalience(predicate string) float64 {
	if meta := vocabulary.GetPredicateMetadata(predicate); meta != nil {
		return meta.Weight
	}
	return 0
}
