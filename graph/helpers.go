package graph

import (
	"github.com/c360studio/semengine/message"
)

// GetPropertyValue returns the value of one property statement (a statement that
// is not a relationship) of the given predicate, picked by EntityState.GetTriple's
// rule, and true; or nil and false when the entity holds none.
func GetPropertyValue(entity *EntityState, predicate string) (any, bool) {
	if entity == nil {
		return nil, false
	}
	t := pickLatest(entity.Triples, func(t *message.Triple) bool {
		return t.Predicate == predicate && !t.IsRelationship()
	})
	if t == nil {
		return nil, false
	}
	return t.Object, true
}

// StaleSet names one (subject, predicate, source) set of an arrival that
// ReplaceBySource did not apply because it was older than the stored set.
type StaleSet struct {
	Subject   string
	Predicate string
	Source    string
}

// ReplaceResult is what ReplaceBySource leaves: the statements to store, and
// each arriving set it did not apply.
type ReplaceResult struct {
	Triples []message.Triple
	Stale   []StaleSet
}

type replaceKey struct{ subject, predicate, source string }

// ReplaceBySource is the replace write mode (graph-entity-writes, "One rule per
// write mode" and "Timestamp orders a replace"). The statements of one
// (subject, predicate, source) are one set: each set the arrival carries
// replaces the stored set of the same key whole, and the statements of the
// same predicate from every other source stay. A set whose latest Timestamp is
// older than the latest Timestamp of the stored set of its key is not applied
// and is named in Stale; equal timestamps apply. Confidence and Context never
// decide. Stored statements of keys the arrival does not carry are kept in
// their order, ahead of the arrival's applied sets.
func ReplaceBySource(stored, arrival []message.Triple) ReplaceResult {
	if len(arrival) == 0 {
		return ReplaceResult{Triples: stored}
	}
	keyOf := func(t message.Triple) replaceKey { return replaceKey{t.Subject, t.Predicate, t.Source} }

	storedLatest := map[replaceKey]message.Triple{}
	for _, t := range stored {
		k := keyOf(t)
		if cur, ok := storedLatest[k]; !ok || t.Timestamp.After(cur.Timestamp) {
			storedLatest[k] = t
		}
	}
	arrivalLatest := map[replaceKey]message.Triple{}
	var order []replaceKey
	for _, t := range arrival {
		k := keyOf(t)
		cur, ok := arrivalLatest[k]
		if !ok {
			order = append(order, k)
		}
		if !ok || t.Timestamp.After(cur.Timestamp) {
			arrivalLatest[k] = t
		}
	}

	applied := map[replaceKey]bool{}
	var stale []StaleSet
	for _, k := range order {
		if prev, ok := storedLatest[k]; ok && arrivalLatest[k].Timestamp.Before(prev.Timestamp) {
			stale = append(stale, StaleSet{Subject: k.subject, Predicate: k.predicate, Source: k.source})
			continue
		}
		applied[k] = true
	}

	merged := make([]message.Triple, 0, len(stored)+len(arrival))
	for _, t := range stored {
		if !applied[keyOf(t)] {
			merged = append(merged, t)
		}
	}
	for _, t := range arrival {
		if applied[keyOf(t)] {
			merged = append(merged, t)
		}
	}
	return ReplaceResult{Triples: merged, Stale: stale}
}
