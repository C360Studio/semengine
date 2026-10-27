package graphindex

import "strings"

// predicateIndexMarker is the fixed, content-free value stored at every PREDICATE_INDEX
// composite key. Empty by construction, not a timestamp: the lanes that write the index
// have no cross-entity completion order, so a timestamp would invite a reader to treat it
// as "most recent", which nothing guarantees (ADR-065).
var predicateIndexMarker = []byte{}

// predicateIndexKey builds the PREDICATE_INDEX composite key for one (predicate, entity)
// membership: predicate3.entity6.
func predicateIndexKey(predicate, entityID string) string {
	return predicate + "." + entityID
}

// predicateIndexEntityFilter lists every predicate membership one entity owns under the
// predicate3.entity6 layout: the owner is the last six tokens.
func predicateIndexEntityFilter(entityID string) string {
	return wildcardPositions(3) + "." + entityID
}

// wildcardPositions returns exactly count single-token NATS wildcards, so a fixed-arity
// filter states its token count.
func wildcardPositions(count int) string {
	return strings.TrimSuffix(strings.Repeat("*.", count), ".")
}
