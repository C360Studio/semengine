package graph

import "time"

// Reply types of the derived-index verbs graph-query forwards to: graph.embedding.query.search and
// similar, graph.spatial.query.bounds and graph.temporal.query.range. Each is the Data of a
// QueryResponse, and the verb table (QueryVerbs) names which verb sends which. Their responders arrive
// with later changes, except the embedding verbs', which processor/graph-embedding answers; these
// types declare what those responders send. The embedding verbs claim no ENTITY_STATES revision and
// send IndexedRevision 0. TemporalRangeRequest and SpatialBoundsRequest are what graph-query's
// global search sends to the range and bounds verbs, so what their responders must decode.

// SimilarityHit is one entity a semantic or similar lookup found.
type SimilarityHit struct {
	EntityID   string  `json:"entity_id"`
	Similarity float64 `json:"similarity"`
}

const (
	// DefaultSimilarityLimit is what a semantic or similar request's limit of 0 means.
	DefaultSimilarityLimit = 10
	// MaxSimilarityLimit is the largest limit a semantic or similar request may send; a larger or
	// negative one is refused, never clamped.
	MaxSimilarityLimit = 100
)

// SemanticRequest is the text search verb's request (graph.embedding.query.search): the entities whose
// embeddings best match Query, at most Limit of them (0 means DefaultSimilarityLimit, else 1 to
// MaxSimilarityLimit). Query is required and not blank. Scope, when not empty, keeps only entities
// whose ID matches one of its dot-delimited prefixes; "" matches every entity. A responder decodes it
// strictly, refusing an unknown key by name. The embedding verbs claim no ENTITY_STATES revision, so
// a min_revision above 0 is refused, never answered.
type SemanticRequest struct {
	Query string   `json:"query"`
	Limit int      `json:"limit"`
	Scope []string `json:"scope,omitempty"`
	MinRevisionField
}

// SimilarRequest is the similar verb's request (graph.embedding.query.similar): the entities whose
// embeddings are most like EntityID's, at most Limit of them, bounded as SemanticRequest's. EntityID
// is a literal entity ID and is never its own hit. A responder decodes it strictly, refusing an
// unknown key by name, and refuses a min_revision above 0 as SemanticRequest's does.
type SimilarRequest struct {
	EntityID string `json:"entity_id"`
	Limit    int    `json:"limit"`
	MinRevisionField
}

// SemanticData is the reply to a text search over the embeddings. EmbedderType names the embedder that
// answered ("bm25" or "http"). UnreadableRecords is the number of EMBEDDING_INDEX records the
// responder could not decode, and FailedRecords the number of entities whose latest embedding failed;
// neither entity can be a hit. A 0 in both says no record was left out for either reason; an entity whose
// embedding is still pending is left out too and counted in neither, so the counts never prove completeness.
type SemanticData struct {
	Results           []SimilarityHit `json:"results"`
	EmbedderType      string          `json:"embedder_type,omitempty"`
	UnreadableRecords int             `json:"unreadable_records"`
	FailedRecords     int             `json:"failed_records"`
}

// SimilarData is the reply to a lookup of the entities most like a given one. UnreadableRecords and
// FailedRecords count what SemanticData's do.
type SimilarData struct {
	Results           []SimilarityHit `json:"results"`
	UnreadableRecords int             `json:"unreadable_records"`
	FailedRecords     int             `json:"failed_records"`
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

// TemporalRangeRequest is the time-range verb's request (graph.temporal.query.range): the entities
// that changed from StartTime to EndTime, at most Limit of them. The times are RFC 3339 on the wire,
// under start_time and end_time, never startTime and endTime. A responder decodes it strictly,
// refusing an unknown key by name, and reads min_revision as every derived-index verb does.
type TemporalRangeRequest struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Limit     int       `json:"limit"`
	MinRevisionField
}

// SpatialBoundsRequest is the bounds verb's request (graph.spatial.query.bounds): the entities inside
// the box from South to North in latitude and West to East in longitude, in degrees, at most Limit of
// them. A responder decodes it strictly, refusing an unknown key by name, and reads min_revision as
// every derived-index verb does. The keyword classifier does not wrap a box across the antimeridian
// (graph/query/classifier.go:455-457, 520-522), so East can exceed 180 and West fall below -180; a
// responder must accept such a box, not refuse it.
type SpatialBoundsRequest struct {
	North float64 `json:"north"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	West  float64 `json:"west"`
	Limit int     `json:"limit"`
	MinRevisionField
}
