// Package graph provides the canonical graph mutation request/reply contract.
// Request/reply messages are identified by their typed port and operation
// subject, so they do not use the polymorphic payload registry.
package graph

import "github.com/c360studio/semengine/message"

// CreateEntityRequest atomically creates one entity with its initial facts.
type CreateEntityRequest struct {
	Entity          *EntityState     `json:"entity"`
	Triples         []message.Triple `json:"triples"`
	IndexingProfile string           `json:"indexing_profile,omitempty"`
	TraceID         string           `json:"trace_id,omitempty"`
	RequestID       string           `json:"request_id,omitempty"`
}

// DeleteEntityRequest conditionally deletes one entity.
type DeleteEntityRequest struct {
	EntityID         string `json:"entity_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	TraceID          string `json:"trace_id,omitempty"`
	RequestID        string `json:"request_id,omitempty"`
}

// ReconcilePredicatesRequest makes the statements of the named predicates from
// Source equal Desired on one existing entity at ExpectedRevision; the same
// predicates' statements from other sources stay. Desired may be empty, which
// clears Source's statements of them. Source is required, and every desired
// statement's Source must equal it (graph-entity-writes, "One rule per write
// mode"); graph-ingest refuses the request as invalid_request otherwise.
type ReconcilePredicatesRequest struct {
	EntityID         string           `json:"entity_id"`
	ExpectedRevision uint64           `json:"expected_revision"`
	Source           string           `json:"source"`
	Predicates       []string         `json:"predicates"`
	Desired          []message.Triple `json:"desired"`
	TraceID          string           `json:"trace_id,omitempty"`
	RequestID        string           `json:"request_id,omitempty"`
}

// AppendTriplesRequest appends canonical tuples independently per subject.
type AppendTriplesRequest struct {
	Triples   []message.Triple `json:"triples"`
	TraceID   string           `json:"trace_id,omitempty"`
	RequestID string           `json:"request_id,omitempty"`
}
