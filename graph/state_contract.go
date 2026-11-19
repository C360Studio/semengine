package graph

import (
	"errors"
	"fmt"

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

// stateRefusal is the one shape of a refused authoritative entity: a single
// fatal graph_state_reset_required classification over a *StateContractError
// with reason and entityID, which is the key when the refusing site has one.
func stateRefusal(entityID string, reason StateResetReason, err error) error {
	return errs.ClassifiedCode(errs.ErrorFatal, ErrorCodeGraphStateResetRequired,
		&StateContractError{Reason: reason, EntityID: entityID, Err: err})
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
// EntityID names the poisoned entity when the classification site knows it
// (snapshot sweep entry key, queried entity ID, RMW target, mutation read
// seam). It is additive and in-process only: the wire carries just the error
// class and code, so consumers reconstructing the classification from headers
// never see it. Err is excluded from JSON on purpose (error values do not
// marshal usefully); the rendered Error() string is the transport for detail.
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
