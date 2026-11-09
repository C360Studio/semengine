// Package fusion is the deterministic lens-driven fusion engine (ADR-062): it
// resolves a query to seed entities, walks the structure around them, ranks and
// budgets the result, and returns an answer keyed by what a human reads, never
// by an entity ID. A product supplies a Lens (lens.go) for its domain and a
// RetrievalClient (retrieval.go) over the graph.
//
// It is a near-leaf package: its domain dependencies are message (Triple in
// Entity, StorageReference as the Hydrate handle), storage (the body store
// interface), and graph and pkg/errs (the classified codes it reads from
// replies). It imports no NATS client and asks no readiness status: a read
// whose index is not ready says so in its reply (ADR-109 D1).
package fusion
