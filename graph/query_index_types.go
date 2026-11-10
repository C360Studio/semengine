package graph

// --- Index Query Data Types ---

// OutgoingEntry represents a single outgoing relationship in the graph.
type OutgoingEntry struct {
	ToEntityID string `json:"to_entity_id"`
	Predicate  string `json:"predicate"`
}

// IncomingEntry represents a single incoming relationship in the graph.
type IncomingEntry struct {
	FromEntityID string `json:"from_entity_id"`
	Predicate    string `json:"predicate"`
}

// RelationshipEdge is one row of the graph-query relationships verb's reply, in
// either direction.
type RelationshipEdge struct {
	Predicate    string `json:"predicate"`
	FromEntityID string `json:"from_entity_id"`
	ToEntityID   string `json:"to_entity_id"`
}

// --- Index Query Response Payloads ---

// OutgoingRelationshipsData contains outgoing relationships for an entity.
type OutgoingRelationshipsData struct {
	Relationships []OutgoingEntry `json:"relationships"`
}

// IncomingRelationshipsData contains incoming relationships for an entity.
type IncomingRelationshipsData struct {
	Relationships []IncomingEntry `json:"relationships"`
}

// AliasData contains the canonical entity ID for an alias lookup.
type AliasData struct {
	CanonicalID *string `json:"canonical_id"` // nil if not found
}

// PredicateRequest is the request for graph.index.query.predicate, and for
// graph.query.predicate, which forwards it unchanged: the entities carrying
// Predicate, with Value set only those whose current state holds it with that
// value (an empty Value is a value), at most Limit of them when Limit is above
// zero.
type PredicateRequest struct {
	Predicate string  `json:"predicate"`
	Value     *string `json:"value,omitempty"`
	Limit     int     `json:"limit,omitempty"`
	MinRevisionField
}

// PredicateData contains entities that have a specific predicate.
type PredicateData struct {
	Entities []string `json:"entities"`
}
