package graph

// MaxSearchTerms is the most distinct words a search query may hold, counted by the word index
// (graph/lexical's ParseQuery) after tokenizing, plain, phrase and excluded words together. A longer
// query is refused, never cut. 32 takes a pasted error line, which runs to about 20 words, while
// bounding the work one search does.
const MaxSearchTerms = 32

// LocalSearchRequest is the localSearch verb's request, decoded strictly: an unknown key is refused.
//
// EntityID is a literal entity ID; the search runs in its group at Level, 0 to 2, omitted 0. Query is
// the word index's query: it must not be blank, and the word index refuses what it cannot parse.
// Limit is the most matches, 0 meaning DefaultSimilarityLimit, else 1 to MaxSimilarityLimit.
type LocalSearchRequest struct {
	EntityID string `json:"entity_id"`
	Query    string `json:"query"`
	Level    int    `json:"level,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	MinRevisionField
}

// RecordState says whether a search could attach a group's COMMUNITY_INDEX record. A record only
// describes its group, so no state refuses a search.
type RecordState string

// The record states.
const (
	// RecordRead is a record read and attached.
	RecordRead RecordState = "read"
	// RecordNone is a community view that holds no record for the group.
	RecordNone RecordState = "none"
	// RecordNotReady is no community view, or one that is catching up, lost or stopped.
	RecordNotReady RecordState = "not_ready"
	// RecordUnreadable is a record the view holds but cannot decode.
	RecordUnreadable RecordState = "unreadable"
)

// LocalSearchRecord is a group's COMMUNITY_INDEX record as a search attaches it: its member count,
// its statistical summary and its keywords.
type LocalSearchRecord struct {
	MemberCount      int      `json:"member_count"`
	CommunitySummary string   `json:"community_summary,omitempty"`
	Keywords         []string `json:"keywords,omitempty"`
}

// LocalSearchData is the reply to a local search: the word index's matches within one group, the
// group's record when there is one, and an answer text.
//
// CommunityID and Level name the group searched. Record is its record, present exactly when
// RecordState is RecordRead. Matches are the word index's hits in its order, each read back as
// stored. Matched is the word index's count of matches in the group before the limit. Excluded is
// the query's excluded words, omitted when there are none. Missing counts hits that were gone when
// they were read back, and were left out. BodiesNotIndexed is the word index's count of entities
// whose offloaded body it did not hold. Truncated is true exactly when Matched is above the hits
// returned, a hit was missing, or matches were left out for the reply to fit the connection's
// max_payload.
type LocalSearchData struct {
	CommunityID      string             `json:"community_id"`
	Level            int                `json:"level"`
	Record           *LocalSearchRecord `json:"record,omitempty"`
	RecordState      RecordState        `json:"record_state"`
	Matches          []TextSearchMatch  `json:"matches"`
	Matched          int                `json:"matched"`
	Excluded         []string           `json:"excluded,omitempty"`
	Missing          int                `json:"missing"`
	BodiesNotIndexed int                `json:"bodies_not_indexed"`
	Truncated        bool               `json:"truncated"`
	Answer           string             `json:"answer"`
}
