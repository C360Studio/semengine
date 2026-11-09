// Package graphembedding is the graph-embedding component: it follows ENTITY_STATES and keeps
// one BM25 embedding record per entity with text in EMBEDDING_INDEX, with its dedup keys in
// EMBEDDING_DEDUP, and reports its progress on the graph-embedding readiness key. At tier 0 the
// only embedder is BM25, which runs in-process; nothing reaches an external provider.
//
// A composition registers the factory with Register and creates the component through the
// component manager, with a configuration that Config describes.
package graphembedding
