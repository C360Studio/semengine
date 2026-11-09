package graph

// --- Graph Summary Query Types ---
//
// GraphSummary is the composite "what's in this graph" overview a
// caller (LLM agent or external dashboard) gets without knowing any
// entity IDs up-front. Solves the discovery chicken-and-egg problem:
// query_entity / query_relationships all need known IDs to start;
// graphSummary tells the caller which IDs and predicates are worth
// knowing.
//
// Composed server-side from existing primitives (entity prefix scan
// for type distribution, predicate-list index for predicate counts).
// Source distribution (Triple.Source aggregation) is deliberately
// deferred to V2 — computing it requires a write-time index or a
// full triple walk, neither of which fits a V1 budget; see the
// project_graph_tools_gateway_first_plan memory.

// SummaryRequest is the summary verb's request, decoded strictly: an unknown key is refused. The zero
// value asks for the first page of the whole graph with its predicates.
type SummaryRequest struct {
	// Prefix, when not empty, narrows the entity types to one group: a dot-delimited entity-ID prefix
	// with no trailing dot. It needs ExcludePredicates, and is refused without it: predicateList takes
	// no entity prefix, so the answer would set one group's types beside the whole graph's predicates,
	// at the cost of scanning the whole predicate index.
	Prefix string `json:"prefix,omitempty"`

	// ExcludePredicates leaves the predicate counts out, so the answer does not depend on graph-index,
	// which otherwise refuses for the whole request when it cannot answer. Predicates is then null and
	// IndexedRevision 0.
	ExcludePredicates bool `json:"exclude_predicates,omitempty"`

	// ExamplesPerType caps how many sample entity IDs to retain per type. 0 means 2; a negative count
	// is refused. Example IDs are load-bearing for the discovery use case: models invent IDs when no
	// examples are provided.
	ExamplesPerType int `json:"examples_per_type,omitempty"`

	// Cursor is a NextCursor from an earlier reply; empty reads the first page.
	Cursor string `json:"cursor,omitempty"`

	// MinRevision reaches predicateList. With ExcludePredicates no index is read, so it is accepted
	// and ignored.
	MinRevisionField
}

// EntityTypeSummary represents one entity-type bucket — entities
// grouped by their 6-part ID's system.domain.type segments (parts
// 3.4.5 in 1-indexed terms; parts 2-4 in 0-indexed slice terms; ADR-102).
//
// Examples in this codebase:
//
//	acme.platform1.agentic-loop.agent.execution.<loopID>
//	→ type "agentic-loop.agent.execution"
//
//	acme.platform1.web.agent.observation.<hash>
//	→ type "web.agent.observation"
type EntityTypeSummary struct {
	// Type is the system.domain.type triple (positions 3-5 of the canonical
	// entity ID, by named field) — the bucket name.
	Type string `json:"type"`

	// Count is how many rows of the page read fall in this bucket: a
	// sample when Truncated is set on the parent SummaryData.
	Count int `json:"count"`

	// Examples is up to ExamplesPerType real entity IDs from this
	// bucket. The caller uses these as starting points for
	// query_entity / query_relationships without having to invent IDs.
	Examples []string `json:"examples"`
}

// SummaryData is the summary verb's reply, in a QueryResponse from graph-query: one prefix page and,
// unless excluded, the predicate list read before it, live and as listed, not a snapshot of the two.
// Caller-friendly text formatting happens at the tool-wrapper layer; this shape stays structured for
// non-LLM consumers (dashboards, audit, programmatic clients).
//
// With predicates excluded the reply's IndexedRevision is 0, which claims nothing: no index was read.
// Do not compare it against a kv_revision on that answer, since it never catches up and a retry loop
// would spin for ever.
type SummaryData struct {
	// TotalEntities is the number of distinct keys in the listing: the whole group as listed, before
	// the cursor and any limit. A listing under concurrent overwrites can omit keys, so it is not a
	// snapshot count (#512).
	TotalEntities int `json:"total_entities"`

	// EntitiesScanned is the rows on this page. The type counts count those rows alone.
	EntitiesScanned int `json:"entities_scanned"`

	// Truncated is true exactly when NextCursor is set, so false says only that this is the last page.
	// EntityTypes covers this page and no more: on a first page with Truncated true, a sample of the
	// lexically smallest IDs. A caller wanting the rest sends NextCursor as Cursor, paying one key scan
	// per page.
	Truncated bool `json:"truncated"`

	// NextCursor is the page's cursor, opaque, absent on the last page.
	NextCursor string `json:"next_cursor,omitempty"`

	// EntityTypes lists the type buckets on this page, by count descending then type. Never null.
	EntityTypes []EntityTypeSummary `json:"entity_types"`

	// Predicates is each predicate as listed, with its entity count from the predicate index, whole-graph and
	// unaffected by the page: null when the request excluded them, [] when there are none. Each
	// request for it reads it again, whatever the cursor.
	Predicates []PredicateSummary `json:"predicates"`
}
