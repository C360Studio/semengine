package graph

// MergedSearchHit is one entity a merged search found. Score is the merged score when the reply's
// Merge is honoured: the sum, over the word list and the embedding list holding the entity, of
// 1 / (60 + its place there). Otherwise it is the word index's score. Either way a higher score is
// a better match, comparable only within one reply.
//
// VectorRank is the entity's 1-based place in the embedding list, omitted when the entity came
// from the word list alone or nothing was merged. A reply with no result carrying one holds no
// meaning result, even when its Merge is honoured: the embedding list may have been empty, or all
// its hits may have fallen below the limit.
type MergedSearchHit struct {
	EntityID   string  `json:"entity_id"`
	Score      float64 `json:"score"`
	VectorRank int     `json:"vector_rank,omitempty"`
}

// MergedSearchData is the reply to graph-query's mergedSearch, which takes a LexicalSearchRequest:
// the ranked entity IDs of a word search merged with an embedding search, with no entity read back.
// Results is at most the request's limit, best first, ties by entity ID. Excluded and
// BodiesNotIndexed are the word search's, as LexicalSearchData says; both are absent or 0 when the
// word index was not ready.
//
// Merge is there exactly when graph-query's embedding_enabled is true. It is the "semantic" hint
// GlobalSearchData.Merge describes: honoured with detail "merged" when the two lists were merged,
// honoured with detail "index_not_ready" when the word index was not ready and Results is the
// embedding list alone, and not honoured, naming why, when the embedding search was refused, broken
// or slow and Results is the word list alone. With no Merge, Results is the word list alone.
type MergedSearchData struct {
	Results          []MergedSearchHit `json:"results"`
	Excluded         []string          `json:"excluded,omitempty"`
	BodiesNotIndexed int               `json:"bodies_not_indexed"`
	Merge            *GlobalSearchHint `json:"merge,omitempty"`
}
