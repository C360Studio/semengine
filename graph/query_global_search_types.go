package graph

import "encoding/json"

// GlobalSearchRequest is the globalSearch verb's request, decoded strictly: an unknown key is refused,
// naming it. Query is the question, required and not blank; every question is searched for its
// words, and no other key steers the answer. Level is the community level the matches are grouped
// at, 0 to 2, omitted 0. Limit is the most matches answered, 0 meaning DefaultSimilarityLimit, else 1
// to MaxSimilarityLimit.
type GlobalSearchRequest struct {
	Query string `json:"query"`
	Level int    `json:"level,omitempty"`
	Limit int    `json:"limit,omitempty"`
	MinRevisionField
}

// GlobalSearchData is the reply to a global search. Text, the word search's matches, is always
// there. Entity, the entity verb's reply data as its bytes came, is there only beside an honoured
// "entity" hint, and Path, the path search from the question's start, only beside an honoured "path"
// hint. Hints lists what the question asked for beyond its words, honoured or not, empty when
// nothing.
type GlobalSearchData struct {
	Text   *TextSearchData    `json:"text"`
	Entity json.RawMessage    `json:"entity,omitempty"`
	Path   *PathSearchData    `json:"path,omitempty"`
	Hints  []GlobalSearchHint `json:"hints"`
}

// GlobalSearchHint is one thing a question asked for beyond its words. Hint is its kind: "entity" (a
// one-word question, read as a reference), "path" (a walk from a start), "time_range", "area",
// "aggregation", "similarity", "ranking", or "classification" (the question classifier refused the
// question). Detail is what the question named for it, such as the reference, the start or the
// operation, omitted when it named nothing. Honoured is true only when the reply holds its answer.
// Reason says why it was not honoured, and Code is the refusal's code when a refusal caused it.
type GlobalSearchHint struct {
	Hint     string `json:"hint"`
	Detail   string `json:"detail,omitempty"`
	Honoured bool   `json:"honoured"`
	Reason   string `json:"reason,omitempty"`
	Code     string `json:"code,omitempty"`
}

// TextSearchData is the word search's result: the word index's matches over every entity, each
// read back as stored, and the community groups at Level that hold them.
//
// Matches are the word index's hits in its order, best first. Groups lists each group holding a
// match once, in the order of its best match. Matched is the word index's count of matches before
// the limit. Excluded is the query's excluded words, omitted when there are none. Missing counts
// hits that were gone when they were read back, and were left out. BodiesNotIndexed is the word
// index's count of entities whose offloaded body it did not hold. Truncated is true exactly when
// Matched is above the hits returned, a hit was missing, or matches were left out for the reply to
// fit the connection's max_payload. UnreadableGroups is the number of Groups whose record could not
// be read. Answer is a plain text built from these fields.
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
