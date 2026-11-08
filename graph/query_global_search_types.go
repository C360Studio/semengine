package graph

import "encoding/json"

// GlobalSearchRequest is the globalSearch verb's request, decoded strictly: an unknown key is refused,
// naming it. Query is the question, required and not blank. The question is classified by its words
// alone; no other key steers which strategy answers. Level is the community level the text strategy
// searches, 0 to 2, omitted 0; it is checked on every request and read by no other strategy.
type GlobalSearchRequest struct {
	Query string `json:"query"`
	Level int    `json:"level,omitempty"`
	MinRevisionField
}

// GlobalSearchData is the reply to a global search: the strategy that answered and that strategy's
// result, exactly one of the five fields below.
//
// Strategy is "entity", "path", "temporal", "spatial" or "text". Entity is the entity verb's reply
// data, its bytes as they came. Path is the path search from the resolved start. Temporal and Spatial
// are what the time-range and bounds verbs answered. Text is the text search over the community
// records at the request's level. Each is the result as read, never a summary of it.
type GlobalSearchData struct {
	Strategy string          `json:"strategy"`
	Entity   json.RawMessage `json:"entity,omitempty"`
	Path     *PathSearchData `json:"path,omitempty"`
	Temporal *TemporalData   `json:"temporal,omitempty"`
	Spatial  *SpatialData    `json:"spatial,omitempty"`
	Text     *TextSearchData `json:"text,omitempty"`
}

// TextSearchData is the text strategy's result: the matching entities of at most five community
// groups at Level, each entity's facts as stored with its name beside its ID, and the groups'
// records beside them, never instead.
//
// Groups are the records read, best first, ties by key; only records scoring above 0 for the
// question are read. Matches are the entities of one page of each group that match the question,
// best first, ties by entity ID. EntitiesScanned is the number of distinct entities the pages held.
// EntitiesTruncated is true exactly when a group had more than one page, or matches were left out
// for the reply to fit the connection's max_payload: the matches are then a sample, not every match.
// UnreadableGroups is the number of records at Level that could not be read, and so were not
// scored. UnhonouredHints names each hint the question carried that the text search does not
// honour ("path", "similarity", "ranking"), empty when none. Answer is a plain text built from these
// fields.
type TextSearchData struct {
	Level             int               `json:"level"`
	Groups            []TextSearchGroup `json:"groups"`
	Matches           []TextSearchMatch `json:"matches"`
	EntitiesScanned   int               `json:"entities_scanned"`
	EntitiesTruncated bool              `json:"entities_truncated"`
	UnreadableGroups  int               `json:"unreadable_groups"`
	UnhonouredHints   []string          `json:"unhonoured_hints"`
	Answer            string            `json:"answer"`
}

// TextSearchGroup is a community group's record as the text search read it: the group's entity-ID
// prefix, its member count, its statistical summary and its keywords.
type TextSearchGroup struct {
	CommunityID      string   `json:"community_id"`
	MemberCount      int      `json:"member_count"`
	CommunitySummary string   `json:"community_summary,omitempty"`
	Keywords         []string `json:"keywords,omitempty"`
}

// TextSearchMatch is one matching entity: its ID; its label, the first non-empty string value of its
// most salient label predicate (vocabulary.DiscoverLabelPredicates), omitted when it has none; its
// score for the question; and the entity as the page sent it, its bytes unchanged.
type TextSearchMatch struct {
	ID     string          `json:"id"`
	Label  string          `json:"label,omitempty"`
	Score  float64         `json:"score"`
	Entity json.RawMessage `json:"entity"`
}
