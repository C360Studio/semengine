package graph

import "encoding/json"

// GlobalSearchRequest is the globalSearch verb's request, decoded strictly: an unknown key is refused,
// naming it. Query is the question, required and not blank. The question is classified by its words
// alone; no other key steers which strategy answers. Level is the community level the text strategy
// groups its matches at, 0 to 2, omitted 0; it is checked on every request and read by no other
// strategy. Limit is the most matches the text strategy answers, 0 meaning DefaultSimilarityLimit,
// else 1 to MaxSimilarityLimit.
type GlobalSearchRequest struct {
	Query string `json:"query"`
	Level int    `json:"level,omitempty"`
	Limit int    `json:"limit,omitempty"`
	MinRevisionField
}

// GlobalSearchData is the reply to a global search: the strategy that answered and that strategy's
// result, exactly one of the five fields below.
//
// Strategy is "entity", "path", "temporal", "spatial" or "text". Entity is the entity verb's reply
// data, its bytes as they came. Path is the path search from the resolved start. Temporal and Spatial
// are what the time-range and bounds verbs answered. Text is the word index's matches, grouped by
// community at the request's level. Each is the result as read, never a summary of it.
type GlobalSearchData struct {
	Strategy string          `json:"strategy"`
	Entity   json.RawMessage `json:"entity,omitempty"`
	Path     *PathSearchData `json:"path,omitempty"`
	Temporal *TemporalData   `json:"temporal,omitempty"`
	Spatial  *SpatialData    `json:"spatial,omitempty"`
	Text     *TextSearchData `json:"text,omitempty"`
}

// TextSearchData is the text strategy's result: the word index's matches over every entity, each
// read back as stored, and the community groups at Level that hold them.
//
// Matches are the word index's hits in its order, best first. Groups lists each group holding a
// match once, in the order of its best match. Matched is the word index's count of matches before
// the limit. Excluded is the query's excluded words, omitted when there are none. Missing counts
// hits that were gone when they were read back, and were left out. BodiesNotIndexed is the word
// index's count of entities whose offloaded body it did not hold. Truncated is true exactly when
// Matched is above the hits returned, a hit was missing, or matches were left out for the reply to
// fit the connection's max_payload. UnreadableGroups is the number of Groups whose record could not
// be read. UnhonouredHints names each hint the question carried that the text strategy does not
// honour ("path", "similarity", "ranking"), empty when none. Answer is a plain text built from
// these fields.
type TextSearchData struct {
	Level            int               `json:"level"`
	Groups           []TextSearchGroup `json:"groups"`
	Matches          []TextSearchMatch `json:"matches"`
	Matched          int               `json:"matched"`
	Excluded         []string          `json:"excluded,omitempty"`
	Missing          int               `json:"missing"`
	BodiesNotIndexed int               `json:"bodies_not_indexed"`
	Truncated        bool              `json:"truncated"`
	UnreadableGroups int               `json:"unreadable_groups"`
	UnhonouredHints  []string          `json:"unhonoured_hints"`
	Answer           string            `json:"answer"`
}

// TextSearchGroup is one community group holding matches: the group's entity-ID prefix, how many
// of the reply's matches it holds, and its COMMUNITY_INDEX record, present exactly when RecordState
// is RecordRead.
type TextSearchGroup struct {
	CommunityID string             `json:"community_id"`
	Matches     int                `json:"matches"`
	RecordState RecordState        `json:"record_state"`
	Record      *LocalSearchRecord `json:"record,omitempty"`
}

// TextSearchMatch is one matching entity, in a global or a local search: its ID; its label, the
// first non-empty string value of its most salient label predicate
// (vocabulary.DiscoverLabelPredicates), omitted when it has none; the word index's score for the
// question; and the entity as it was read, its bytes unchanged.
type TextSearchMatch struct {
	ID     string          `json:"id"`
	Label  string          `json:"label,omitempty"`
	Score  float64         `json:"score"`
	Entity json.RawMessage `json:"entity"`
}
