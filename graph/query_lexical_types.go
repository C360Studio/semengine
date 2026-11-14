package graph

// LexicalSearchRequest is the word search verb's request (graph.lexical.query.search): the entities
// whose words best match Query, at most Limit of them (0 means DefaultSimilarityLimit, else 1 to
// MaxSimilarityLimit; any other value is refused, never clamped). Query is read by
// graph/lexical's ParseQuery, and a query it refuses is refused. Scope, when not empty, keeps only
// entities whose ID matches one of its entity-ID prefixes, before the limit; "" matches every
// entity. A responder decodes it strictly, refusing an unknown key by name, and reads min_revision
// as graph-index's verbs do.
type LexicalSearchRequest struct {
	Query string   `json:"query"`
	Limit int      `json:"limit"`
	Scope []string `json:"scope,omitempty"`
	MinRevisionField
}

// LexicalHit is one entity a word search found. Score is the word index's BM25F score: higher is
// a better match, on no fixed scale, and comparable only within one reply.
type LexicalHit struct {
	EntityID string  `json:"entity_id"`
	Score    float64 `json:"score"`
}

// LexicalSearchData is the reply to a word search. Results holds the best hits, ordered by score
// and then by entity ID. Matched is the number of entities that matched within the scope, before
// the limit, so Matched above len(Results) says the limit cut the answer. BodiesNotIndexed is the
// number of entities in the whole index, not the scope or the hits, whose offloaded body the
// index did not hold when the reply was built: such an entity is found only by its inline text.
type LexicalSearchData struct {
	Results          []LexicalHit `json:"results"`
	Matched          int          `json:"matched"`
	BodiesNotIndexed int          `json:"bodies_not_indexed"`
}
