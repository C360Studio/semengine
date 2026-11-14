package graph

import "encoding/json"

// MaxSearchTerms is the most distinct words a search query may hold, counted by the word index
// (graph/lexical's ParseQuery) after tokenizing, plain, phrase and excluded words together. A longer
// query is refused, never cut. 32 takes a pasted error line, which runs to about 20 words, while
// bounding the work one search does.
const MaxSearchTerms = 32

// LocalSearchRequest is the localSearch verb's request, decoded strictly: an unknown key is refused.
//
// EntityID is a literal entity ID; the search runs in its group at Level, 0 to 2, omitted 0. Query is
// lowercased and split on white space, repeats removed, and must leave 1 to MaxSearchTerms terms.
type LocalSearchRequest struct {
	EntityID string `json:"entity_id"`
	Query    string `json:"query"`
	Level    int    `json:"level,omitempty"`
	MinRevisionField
}

// LocalSearchData is the reply to a local search: the group's record, the matches among one page of the
// group, and an answer text.
//
// Entities are the matches, best first, ties by entity ID, each as the page sent it. EntitiesScanned is
// the number of distinct entities read. EntitiesTruncated is true exactly when the page had more to read,
// or matches were left out for the reply to fit the connection's max_payload; the matches kept are then
// the best of what was read, not of the group.
type LocalSearchData struct {
	CommunityID       string            `json:"community_id"`
	Level             int               `json:"level"`
	MemberCount       int               `json:"member_count"`
	CommunitySummary  string            `json:"community_summary,omitempty"`
	Keywords          []string          `json:"keywords,omitempty"`
	Entities          []json.RawMessage `json:"entities"`
	EntitiesScanned   int               `json:"entities_scanned"`
	EntitiesTruncated bool              `json:"entities_truncated"`
	Answer            string            `json:"answer"`
}
