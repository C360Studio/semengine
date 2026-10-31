package clustering

// The bounds of a published group record (design D4). Each holds by construction, so a
// record is never truncated at run time.
const (
	// MaxRepEntities is the most representative entity IDs a record holds.
	MaxRepEntities = 5
	// MaxKeywords is the most keywords, and the most entity types, a record holds.
	MaxKeywords = 10
	// MaxEntityTerms is the most words one entity contributes to each of its groups.
	MaxEntityTerms = 32
	// MaxWordBytes is the longest word. A longer run of letters and digits is not a word.
	MaxWordBytes = 32
	// MaxRecordBytes bounds a record's JSON encoding whatever its members' values: a
	// 256-byte ID, MaxKeywords types of at most 246 bytes, MaxKeywords words,
	// MaxRepEntities 256-byte IDs and a summary naming three types and five keywords
	// encode to under 6 KiB (TestRecordIsBounded).
	MaxRecordBytes = 8 << 10
)

// Community is the published record of one group: every entity whose ID shares one
// prefix. Level 0 groups by org.platform.system.domain, level 1 by the source
// org.platform.system, level 2 by the deployment org.platform (ADR-102 d6); ID is that
// prefix. The record holds counts and samples, never the member list, so its size does
// not grow with the group.
type Community struct {
	// ID is the group's entity-ID prefix.
	ID string `json:"id"`

	// Level is the group's level: 0, 1 or 2.
	Level int `json:"level"`

	// MemberCount is the number of entities in the group.
	MemberCount int `json:"member_count"`

	// TypeCounts counts members by entity type (the ID's fifth position), for the
	// MaxKeywords most common types, ties broken lexically.
	TypeCounts map[string]int `json:"type_counts"`

	// Keywords are the group's MaxKeywords most frequent words, most frequent first,
	// ties broken lexically. Words come only from members' string values.
	Keywords []string `json:"keywords,omitempty"`

	// StatisticalSummary is a one-line text built from the counts, three types and five
	// keywords.
	StatisticalSummary string `json:"statistical_summary,omitempty"`

	// RepEntities are the lexically smallest IDs of members with words, at most
	// MaxRepEntities.
	RepEntities []string `json:"rep_entities,omitempty"`
}
