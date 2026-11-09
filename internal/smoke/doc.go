// Package smoke is a test-only package: its integration tests boot the tier-0 graph stack the way a
// product's own main would, from one JSON configuration through config.Manager, ServiceManager and
// ComponentManager, in a child process, and check what a consumer can read of it (openspec change
// test-557-composed-stack-smoke, capability composed-stack).
//
// The stack is graph-ingest, graph-index, graph-embedding (BM25, no provider) and graph-query on one
// NATS server. The test is the producer, publishing a fixed set of entities on entity.>, and the
// observer: graph.query.* answers (semantic and similar among them), the GRAPH_STATUS readiness
// keys, component health over HTTP and the EMBEDDING_INDEX records. Every wait is ordered after the write it checks.
//
// The package holds no production code; everything else is in integration-tagged test files.
package smoke
