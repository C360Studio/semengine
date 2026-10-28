// Package embedding turns the text of graph entities into vectors, stores them, and finds the
// nearest ones. It needs no external provider: its embedder, BM25Embedder, runs in the process.
//
// Embedding is two hops. Hop 1 is a component outside this package that writes one pending
// record per entity revision to the EMBEDDING_INDEX bucket (Storage.SavePendingGuarded). Hop 2 is
// the Worker, which turns each pending record into a generated one holding the vector, or a
// failed one holding a reason. The Worker is what this package runs; the hop-1 component is
// processor/graph-embedding, which is not part of this package.
//
// # Parts
//
// Embedder is the interface an embedding source implements, for documents (Generate) and for
// search queries (GenerateQuery). BM25Embedder is the tier-0 implementation: it hashes terms
// into a fixed number of dimensions and weights them with BM25, so it matches shared words, not
// meaning. A product that wants neural vectors supplies its own Embedder.
//
// Storage reads and writes two NATS KV buckets: EMBEDDING_INDEX, one record per entity, and
// EMBEDDING_DEDUP, one vector per content key so identical text is embedded once. Every
// read-modify-write is a compare-and-set through natsclient.KVStore.UpdateWithRetryRead: the
// decision is made from the bytes and revision it read, and a lost race is retried. Saves are
// ordered by source revision (the ENTITY_STATES revision a record was made from): a save for an
// older revision never replaces a generated record for a newer one, and returns
// ErrSupersededRevision or ErrRecordGone without writing. Neither is a failure.
//
// When a record's text resolves to nothing, the Worker removes the record only if its key is
// still at the KV revision the Worker read; if newer work has moved the key, nothing is removed
// and the outcome is OutcomeDeleted.
//
// VectorCache keeps the generated vectors in memory by watching EMBEDDING_INDEX, and FindSimilar
// ranks them by cosine similarity. It answers only while it is authoritative: before it has read
// the bucket's initial contents, after a record failed to decode, after the watch was lost, and
// after Run returned, it answers false and the caller reads the bucket.
//
// # Running
//
// Worker.Run(ctx) and VectorCache.Run(ctx) each block until ctx ends or their watch is lost, and
// return ctx.Err() or an error saying the watch was lost. The caller owns ctx and ends it to stop
// them. Each refuses a nil context and a second Run while one is running. A lost watch is an
// error to act on, not a normal stop: the caller decides whether to report it or run again.
//
// # Ordering
//
// The Worker hands each watch entry to a keyed pool (internal/dispatch.KeyedPool), keyed by
// entity ID. One entity's entries are processed one at a time, in the order the watch delivered
// them, from reading the record through the store write to GeneratedCallback and
// TerminalCallback. Different entities run in parallel on the WithWorkers lanes (default 5).
// Callbacks for different entities are unordered. The package promises nothing about the order
// the watch delivers entries in; the revision checks in Storage are what keep an older save from
// replacing a newer generated record.
//
// # Where this differs from SemStreams
//
// The package is a port of SemStreams' graph/embedding without its HTTP embedder. The changes
// are listed in the admission ledger row for graph/embedding.
package embedding
