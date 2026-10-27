package graphindex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
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

// reconcileNameIndex replaces entityID's name memberships with writes.
func (c *Component) reconcileNameIndex(ctx context.Context, entityID string, writes []nameIndexWrite) error {
	if err := semtypes.ValidateEntityID(entityID); err != nil {
		return err
	}
	desired := make(map[string][]byte, len(writes))
	for _, write := range writes {
		if write.name == "" || !validateNameKeyInputs(entityID, write.predicate, c.logger) {
			return errs.WrapInvalid(errs.ErrInvalidData, "Component", "reconcileNameIndex",
				"name membership contains an invalid name, entity ID, or predicate")
		}
		value, err := json.Marshal(nameCompositeValue{Name: write.name, Priority: write.priority})
		if err != nil {
			return errs.Wrap(err, "Component", "reconcileNameIndex", "marshal value")
		}
		desired[nameCompositeKey(nameIndexKey(write.name), entityID, write.predicate)] = value
	}
	// Retained keys are overwritten: the original-case spelling and the priority live in
	// the value and may change while the folded key stays the same.
	return c.reconcileOwnedRows(ctx, "name", c.nameBucket, nameIndexEntityFilter(entityID), desired, true)
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
