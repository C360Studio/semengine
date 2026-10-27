package graphindex

import (
	"log/slog"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/vocabulary"
)

// incomingIndexMarker is the fixed, content-free value stored at every INCOMING_INDEX
// composite key; empty for the reason predicateIndexMarker is (ADR-065).
var incomingIndexMarker = []byte{}

// incomingIndexKey builds the INCOMING_INDEX composite key for one directed edge:
// targetID.sourceID.hex(predicate). The predicate is hex-encoded because a predicate may
// hold bytes a KV key may not; hex keeps it reversible, so a reader recovers it from the
// key alone.
//
// Unconditional Put, no CAS (ADR-065): each (target, source, predicate) key is written by
// its source entity's lane alone, so no two writers contend on it. If this bucket ever
// needs CAS again, that invariant no longer holds.
func incomingIndexKey(targetID, sourceID, predicate string) string {
	return targetID + "." + sourceID + "." + encodePredicateToken(predicate)
}

// incomingIndexSourceFilter lists every edge sourceID owns, across all targets:
// target6.source6.hex(predicate). The owner is the source, tokens seven to twelve,
// although the key begins with the target.
func incomingIndexSourceFilter(sourceID string) string {
	return wildcardPositions(6) + "." + sourceID + ".*"
}

// validateIncomingKeyInputs reports whether targetID, sourceID and predicate make a valid
// INCOMING_INDEX key, logging the refusal at Debug.
func validateIncomingKeyInputs(targetID, sourceID, predicate string, logger *slog.Logger) bool {
	if !message.IsValidEntityID(targetID) {
		logger.Debug("incoming index: invalid target entity ID, skipping", slog.String("target_id", targetID))
		return false
	}
	if !message.IsValidEntityID(sourceID) {
		logger.Debug("incoming index: invalid source entity ID, skipping", slog.String("source_id", sourceID))
		return false
	}
	if _, err := vocabulary.ParsePredicate(predicate); err != nil {
		logger.Debug("incoming index: invalid predicate, skipping",
			slog.String("target_id", targetID), slog.String("source_id", sourceID), slog.Any("error", err))
		return false
	}
	return true
}
