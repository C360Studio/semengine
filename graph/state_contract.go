package graph

import (
	"errors"
	"fmt"
	"slices"

	"github.com/c360studio/semengine/pkg/errs"
)

// ErrorCodeGraphStateResetRequired is the cross-component code for
// authoritative ENTITY_STATES that cannot be interpreted by the running graph
// contract.
const ErrorCodeGraphStateResetRequired = "graph_state_reset_required"

// IsStateContractError reports whether err carries the shared authoritative
// graph-reset contract, either as the in-process typed cause or as the stable
// classified code reconstructed across request/reply.
func IsStateContractError(err error) bool {
	if err == nil {
		return false
	}
	var stateErr *StateContractError
	if errors.As(err, &stateErr) {
		return true
	}
	var classified *errs.ClassifiedError
	return errors.As(err, &classified) && classified.Code == ErrorCodeGraphStateResetRequired
}

// detailEntityIDs is the Detail key of a graph_state_reset_required refusal
// that lists the damaged entities it refuses for.
const detailEntityIDs = "entity_ids"

// stateRefusal is the one shape of a refused authoritative entity: a single
// fatal graph_state_reset_required classification over a *StateContractError
// with reason and entityID, which is the key when the refusing site has one.
// A refusal with a key names it in Detail["entity_ids"], so a caller reads it
// as data; one with no key carries no detail.
func stateRefusal(entityID string, reason StateResetReason, err error) error {
	cause := &StateContractError{Reason: reason, EntityID: entityID, Err: err}
	if entityID == "" {
		return errs.ClassifiedCode(errs.ErrorFatal, ErrorCodeGraphStateResetRequired, cause)
	}
	return errs.ClassifiedCodeDetail(errs.ErrorFatal, ErrorCodeGraphStateResetRequired,
		map[string]any{detailEntityIDs: []string{entityID}}, cause)
}

// StateRefusalEntityIDs returns the damaged entities a graph_state_reset_required
// refusal names in its detail, sorted, each once. It reads a refusal built in
// this process and one RequestClassified rebuilt from a reply, where JSON has
// turned the list into []any. It returns nil for any other error and for a
// refusal that names no entity. The list is the caller's own copy.
func StateRefusalEntityIDs(err error) []string {
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) || classified.Code != ErrorCodeGraphStateResetRequired {
		return nil
	}
	switch ids := classified.Detail[detailEntityIDs].(type) {
	case []string:
		return slices.Clone(ids)
	case []any:
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			s, ok := id.(string)
			if !ok {
				return nil
			}
			out = append(out, s)
		}
		return out
	}
	return nil
}

// StateResetReason is a bounded machine-readable reset cause.
type StateResetReason string

const (
	// GraphStateReasonUnreadableEntity means authoritative JSON cannot decode.
	GraphStateReasonUnreadableEntity StateResetReason = "unreadable_entity_state"
	// GraphStateReasonNoncanonicalPredicate means stored state violates the predicate grammar.
	GraphStateReasonNoncanonicalPredicate StateResetReason = "noncanonical_predicate"
	// GraphStateReasonNoncanonicalEntityID means the stored entity ID, a triple
	// subject, or an explicitly marked entity reference violates the canonical
	// entity identity contract.
	GraphStateReasonNoncanonicalEntityID StateResetReason = "noncanonical_entity_id"
	// GraphStateReasonMissingStatementMetadata means a stored statement has no Source or no Timestamp, which every
	// stored statement carries (ADR-110 Decision 5).
	GraphStateReasonMissingStatementMetadata StateResetReason = "missing_statement_metadata"
)

// StateContractError means the authoritative graph cannot be safely read
// or projected. Every reader scopes the refusal to the damaged entity and
// recovers as soon as operators repair the stored bytes (write a valid value
// or delete the key) — no restart required. The authoritative read surface
// (graph-ingest's query lanes) refuses reads of that entity; projection
// owners (derived views built from watched or replayed entity state) record
// it with follower.ReportDamaged, remove what they built from it, keep
// serving the other entities, and count the entity in their replies.
//
// EntityID names the damaged entity when the classification site knows it
// (snapshot sweep entry key, queried entity ID, RMW target, mutation read
// seam). The struct itself stays in process: across request/reply the
// refusal's message carries its text, and the classified error's
// Detail["entity_ids"] carries the entities it names, which
// StateRefusalEntityIDs reads on either side. Err is excluded from JSON on
// purpose (error values do not marshal usefully).
type StateContractError struct {
	Reason   StateResetReason `json:"reason"`
	EntityID string           `json:"entity_id,omitempty"`
	Err      error            `json:"-"`
}

// Error implements error.
func (e *StateContractError) Error() string {
	if e == nil {
		return ErrorCodeGraphStateResetRequired
	}
	msg := fmt.Sprintf("%s: %s", ErrorCodeGraphStateResetRequired, e.Reason)
	if e.EntityID != "" {
		msg += fmt.Sprintf(": entity %s", e.EntityID)
	}
	if e.Err != nil {
		msg += fmt.Sprintf(": %v", e.Err)
	}
	return msg
}

// Unwrap exposes the structural or JSON cause.
func (e *StateContractError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
