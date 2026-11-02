// Package graphindex provides graph-index, the component that keeps the graph's relationship
// indexes: it follows the ENTITY_STATES bucket through graph/follower, writes five index
// buckets it owns (OUTGOING_INDEX, INCOMING_INDEX, ALIAS_INDEX, PREDICATE_INDEX and
// NAME_INDEX), and answers the eight graph.index.query.* verbs declared for it in
// graph.QueryVerbs. Each answer is a graph.QueryResponse carrying the revision it reflects
// (indexed_revision) and the producer that gave it.
//
// Until the index has rebuilt from ENTITY_STATES after a start, and for as long as it has lost
// its feed, every verb refuses with the transient error code index_not_ready; after a stored
// entity that cannot be decoded, it refuses with the fatal graph_state_reset_required.
//
// An alias lookup answers the one live entity carrying the alias (trimmed and case-folded, as
// names are), answers no ID when none does, and refuses with class invalid and code
// alias_ambiguous when two or more do. A dropped or changed alias, or a deleted entity's alias,
// stops resolving once the revision is indexed. The alias predicates are read once, when
// graph-index starts; a predicate registered later is not indexed until the next start.
//
// The package README.md describes its ports, configuration, query verbs, metrics and health.
package graphindex
