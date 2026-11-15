// Package smoke is a test-only package: its integration tests boot the tier-0 graph stack the way a
// product's own main would, from one JSON configuration through config.Manager, ServiceManager and
// ComponentManager, in a child process, and check what a consumer can read of it (spec
// composed-stack).
//
// The stack is graph-ingest, graph-index, graph-clustering, graph-lexical, graph-query,
// graph-gateway and an applied-state websocket output on one NATS server. It has no graph-embedding:
// tier 0 has no embedder, so word questions answer from graph-lexical's word index. The test is the
// producer, publishing a fixed set of entities on entity.>, and the observer: graph.query.* answers
// (localSearch and the global text search among them), graph-lexical's word search, pkg/fusion's
// symbol, prefix and word questions over pkg/fusion/fusionnats, graph-gateway's GraphQL answers over
// HTTP, the websocket output's frames folded against ENTITY_STATES, the GRAPH_STATUS readiness keys
// and component health over HTTP. Every wait is ordered after the write it checks; a community
// answer waits on its record's entity_revision, a word search on its reply's indexed_revision.
//
// The package holds no production code; everything else is in integration-tagged test files.
package smoke
