package graphindex

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/vocabulary"
)

// nameIndexKey is the NAME_INDEX key prefix for a name: the hex sha256 of the case-folded,
// trimmed name. A name may hold bytes a KV key may not, so the index keys on a hash; the
// original-case name rides in the stored value. Folding gives case-insensitive recall.
func nameIndexKey(name string) string {
	sum := sha256.Sum256([]byte(normalizeName(name)))
	return hex.EncodeToString(sum[:])
}

// normalizeName case-folds and trims a name for case-insensitive matching.
func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// nameCompositeKey builds the NAME_INDEX key for one (name, entity, predicate) membership:
// hash(name).entity6.hex(predicate). Unconditional Put, no CAS (ADR-065): the key is
// written by its entity's lane alone.
func nameCompositeKey(nameHash, entityID, predicate string) string {
	return nameHash + "." + entityID + "." + encodePredicateToken(predicate)
}

// nameIndexEntityFilter lists every name membership entityID owns:
// hash(name).entity6.hex(predicate).
func nameIndexEntityFilter(entityID string) string {
	return "*." + entityID + ".*"
}

// nameCompositeValue is the JSON value stored at each NAME_INDEX key: the original-case
// name and the label predicate's priority, which the hashed key cannot carry.
type nameCompositeValue struct {
	Name     string `json:"name"`     // original-case name as stored on the entity
	Priority int    `json:"priority"` // label-predicate salience (lower = higher)
}

type nameIndexWrite struct {
	name      string
	predicate string
	priority  int
}

// validateNameKeyInputs reports whether entityID and predicate make a valid NAME_INDEX
// key, logging the refusal at Debug: a key the reader would reject or mis-split is never
// stored.
func validateNameKeyInputs(entityID, predicate string, logger *slog.Logger) bool {
	if !message.IsValidEntityID(entityID) {
		logger.Debug("name index: invalid entity ID, skipping", slog.String("entity_id", entityID))
		return false
	}
	if _, err := vocabulary.ParsePredicate(predicate); err != nil {
		logger.Debug("name index: invalid predicate, skipping",
			slog.String("entity_id", entityID), slog.Any("error", err))
		return false
	}
	return true
}

// aliasIndexKey builds the ALIAS_INDEX key for one (alias, entity, predicate) membership:
// hash(alias).entity6.hex(predicate), NAME's layout (ADR-114 §1). The hash is nameIndexKey's,
// so an alias is trimmed and case-folded as a name is; the value is an empty marker.
func aliasIndexKey(alias, entityID, predicate string) string {
	return nameCompositeKey(nameIndexKey(alias), entityID, predicate)
}

// aliasIndexEntityFilter lists every alias membership entityID owns (ADR-114 §2).
func aliasIndexEntityFilter(entityID string) string {
	return nameIndexEntityFilter(entityID)
}

type aliasIndexWrite struct {
	alias     string
	predicate string
}
