// Package smoke is a test-only package: its integration tests boot the tier-0 graph stack the way a
// product's own main would, from one JSON configuration through config.Manager, ServiceManager and
// ComponentManager, in a child process, and check what a consumer can read of it (spec
// composed-stack).
//
// The stack is graph-ingest, graph-index, graph-embedding (BM25, no provider), graph-clustering,
// graph-lexical, graph-query, graph-gateway and an applied-state websocket output on one NATS
// server. The test is the producer, publishing a fixed set of entities on entity.>, and the
// observer: graph.query.* answers (semantic, similar, localSearch and the text search among them),
// graph-lexical's word search, graph-gateway's GraphQL answers over HTTP, the websocket output's
// frames folded against ENTITY_STATES, the GRAPH_STATUS readiness keys, component health over HTTP
// and the EMBEDDING_INDEX records. Every wait is ordered after the write it checks; a community
// answer waits on its record's entity_revision, a word search on its reply's indexed_revision.
//
// A second composition has no graph-embedding, neither configured nor registered:
// TestSmoke_NoEmbedderAnswers boots it and asks pkg/fusion, over pkg/fusion/fusionnats, the
// questions a stack without an embedder answers, and the word question it fails: no graph-lexical
// is composed there, so the error is transient internal naming graph-lexical's word search.
//
// The package holds no production code; everything else is in integration-tagged test files.
package smoke
