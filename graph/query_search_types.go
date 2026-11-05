package graph

// Reply types of the derived-index verbs graph-query forwards to: graph.embedding.query.search and
// similar, graph.spatial.query.bounds and graph.temporal.query.range. Each is the Data of a
// QueryResponse, and the verb table (QueryVerbs) names which verb sends which. Their responders arrive
// with later changes; these types declare what those responders must send. What IndexedRevision means
// on the embedding verbs is theirs to settle.

// SimilarityHit is one entity a semantic or similar lookup found.
type SimilarityHit struct {
	EntityID   string  `json:"entity_id"`
	Similarity float64 `json:"similarity"`
}

// SemanticData is the reply to a text search over the embeddings. EmbedderType names the embedder that
// answered ("bm25" or "http").
type SemanticData struct {
	Results      []SimilarityHit `json:"results"`
	EmbedderType string          `json:"embedder_type,omitempty"`
}

// SimilarData is the reply to a lookup of the entities most like a given one.
type SimilarData struct {
	Results []SimilarityHit `json:"results"`
}

// SpatialHit is one entity a bounds lookup found, at its indexed position, so a caller gets geometry
// without reading each entity. Alt is 0 when the entity has no vertical component.
type SpatialHit struct {
	EntityID string  `json:"entity_id"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Alt      float64 `json:"alt,omitempty"`
}

// SpatialData is the reply to a bounds lookup. Truncated says more entities matched than Results holds.
type SpatialData struct {
	Results   []SpatialHit `json:"results"`
	Truncated bool         `json:"truncated"`
}

// TemporalHit is one entity a time-range lookup found.
type TemporalHit struct {
	EntityID string `json:"entity_id"`
}

// TemporalData is the reply to a time-range lookup. Truncated says more entities matched than Results
// holds.
type TemporalData struct {
	Results   []TemporalHit `json:"results"`
	Truncated bool          `json:"truncated"`
}
