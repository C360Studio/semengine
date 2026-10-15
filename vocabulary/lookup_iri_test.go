package vocabulary

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

func TestLookupByIRI(t *testing.T) {
	defer SnapshotRegistry()()
	ClearRegistry()

	// Empty and unregistered IRIs return nil.
	assert.Nil(t, LookupByIRI(""))
	assert.Nil(t, LookupByIRI("http://example.org/unknown"))

	// Single predicate registration.
	Register("geo.location.latitude",
		WithDescription("Latitude in decimal degrees"),
		WithDataType(DataTypeFloat),
		WithIRI("http://example.org/lat"))
	assert.Equal(t, []string{"geo.location.latitude"}, LookupByIRI("http://example.org/lat"))

	// One-to-many IRI mapping returns lexicographically sorted canonical predicates.
	Register("air.track.latitude",
		WithDescription("Track latitude"),
		WithDataType(DataTypeFloat),
		WithIRI("http://example.org/lat"))
	got := LookupByIRI("http://example.org/lat")
	assert.Equal(t, []string{"air.track.latitude", "geo.location.latitude"}, got)

	// Mutating the returned slice must not mutate registry state.
	got[0] = "mutated.predicate.name"
	assert.Equal(t, []string{"air.track.latitude", "geo.location.latitude"}, LookupByIRI("http://example.org/lat"))

	// Amending a predicate with a new StandardIRI removes the old mapping and adds the new one.
	Register("ops.alert.status", WithIRI("http://example.org/v1/status"))
	assert.Equal(t, []string{"ops.alert.status"}, LookupByIRI("http://example.org/v1/status"))

	Register("ops.alert.status", WithIRI("http://example.org/v2/status"))
	assert.Nil(t, LookupByIRI("http://example.org/v1/status"))
	assert.Equal(t, []string{"ops.alert.status"}, LookupByIRI("http://example.org/v2/status"))

	// Amending without WithIRI preserves the existing StandardIRI and reverse index entry.
	Register("ops.alert.status", WithDescription("Amended description without IRI"))
	assert.Equal(t, []string{"ops.alert.status"}, LookupByIRI("http://example.org/v2/status"))

	// SnapshotRegistry and ClearRegistry preserve and clear reverse index state.
	restore := SnapshotRegistry()
	ClearRegistry()
	assert.Nil(t, LookupByIRI("http://example.org/lat"))
	assert.Nil(t, LookupByIRI("http://example.org/v2/status"))

	restore()
	assert.Equal(t, []string{"air.track.latitude", "geo.location.latitude"}, LookupByIRI("http://example.org/lat"))
	assert.Equal(t, []string{"ops.alert.status"}, LookupByIRI("http://example.org/v2/status"))
}

func TestPropLookupByIRIRoundTrip(t *testing.T) {
	defer SnapshotRegistry()()

	segmentGen := rapid.StringMatching(`[a-z][a-z0-9]{2,5}`)
	predicateGen := rapid.Custom(func(t *rapid.T) string {
		return fmt.Sprintf("%s.%s.%s",
			segmentGen.Draw(t, "domain"),
			segmentGen.Draw(t, "category"),
			segmentGen.Draw(t, "property"))
	})
	iriGen := rapid.Custom(func(t *rapid.T) string {
		return "https://example.org/ont/" + segmentGen.Draw(t, "term")
	})

	rapid.Check(t, func(t *rapid.T) {
		ClearRegistry()

		iris := rapid.SliceOfN(iriGen, 1, 4).Draw(t, "iris")
		preds := rapid.SliceOfNDistinct(predicateGen, 1, 8, func(s string) string { return s }).Draw(t, "preds")

		// Reference model: predicate -> current StandardIRI.
		model := make(map[string]string, len(preds))
		for _, pred := range preds {
			iri := rapid.SampledFrom(iris).Draw(t, "initial_iri_"+pred)
			Register(pred, WithIRI(iri))
			model[pred] = iri
		}

		// Optionally amend a subset of predicates with either a new IRI or a description-only update.
		for _, pred := range preds {
			if rapid.Bool().Draw(t, "amend_"+pred) {
				if rapid.Bool().Draw(t, "amend_iri_"+pred) {
					newIRI := rapid.SampledFrom(iris).Draw(t, "new_iri_"+pred)
					Register(pred, WithIRI(newIRI))
					model[pred] = newIRI
				} else {
					Register(pred, WithDescription("amended"))
				}
			}
		}

		for _, iri := range iris {
			var want []string
			for pred, currentIRI := range model {
				if currentIRI == iri {
					want = append(want, pred)
				}
			}
			sort.Strings(want)
			assert.Equal(t, want, LookupByIRI(iri))
		}
		assert.Nil(t, LookupByIRI("https://example.org/unregistered/term"))
	})
}
