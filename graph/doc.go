// Package graph holds the data model and the wire types of SemEngine's entity
// graph. It imports no NATS client, so code that only reads a graph type does
// not pull in the transport.
//
// The data model is EntityState, one entity's stored state in the
// ENTITY_STATES bucket, and Graphable, the interface a payload implements to
// declare its entity and statements. Beside them sit the write rules
// (ReplaceBySource), the validating encoder and decoder for stored entities
// (MarshalEntityState, UnmarshalEntityState), StateContractError for state
// that must be reset, the entity-ID prefix helpers, the predicate codec, and
// the Bucket* names.
//
// The wire types are the request and reply bodies of the graph's request/reply
// subjects: the query types (QueryVerbs declares the three verbs graph-ingest
// answers on graph.ingest.query.*), the mutation requests and responses,
// ExactEntity with its reader, and IndexStatusResponse, the readiness envelope
// stored in GRAPH_STATUS. A request/reply body is identified by the subject it
// travels on, so these types are not registered in the payload registry.
//
// Three sub-packages hold the rest: graph/kvcatalog describes and opens the
// buckets, graph/readiness computes and publishes the readiness envelope, and
// graph/inference infers hierarchy statements.
package graph
