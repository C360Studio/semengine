// Package graphembedding is the graph-embedding component: it follows ENTITY_STATES and keeps
// one embedding record per entity with text in EMBEDDING_INDEX, with its dedup keys in
// EMBEDDING_DEDUP, and reports its progress on the graph-embedding readiness key.
//
// graph-embedding has no embedder of its own. A product with one (tier 1, such as semembed)
// registers the factory with RegisterWithEmbedder, naming the embedder, and creates the
// component through the component manager, with a configuration that Config describes. The
// factory plain Register adds refuses construction, so a tier 0 composition leaves
// graph-embedding out.
package graphembedding
