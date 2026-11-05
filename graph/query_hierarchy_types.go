package graph

// HierarchyStatsRequest is the hierarchyStats verb's request, decoded strictly: an unknown key is
// refused. Prefix, when not empty, is a dot-delimited entity-ID prefix with no trailing dot; empty
// counts the whole graph. Cursor is a NextCursor from an earlier reply; empty reads the first page.
//
// The verb reads no index, so it has no revision to compare: MinRevision is accepted and ignored.
type HierarchyStatsRequest struct {
	Prefix string `json:"prefix,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	MinRevisionField
}

// HierarchyChild is one prefix one segment longer than the request's, with how many entities on the
// page sit under it.
type HierarchyChild struct {
	Prefix string `json:"prefix"`
	Count  int    `json:"count"`
}

// HierarchyStatsData is the reply to hierarchyStats, read from one prefix page, live and as listed,
// not a snapshot. It is bare, since no index is read.
//
// TotalEntities is the length of the key listing: the whole group as listed, before the cursor and any
// limit. EntitiesScanned is the rows on this page. Children, by count descending then prefix, count
// those rows alone, so they cover this page and no more: on a first page with Truncated true, a sample
// of the lexically smallest IDs; on a page read with a cursor, that page. An entity whose ID is the
// prefix is scanned and in no child. Truncated is true exactly when NextCursor is set, so false says
// only that this is the last page; a caller wanting the rest sends NextCursor as Cursor, paying one
// key scan per page.
type HierarchyStatsData struct {
	Prefix          string           `json:"prefix"`
	TotalEntities   int              `json:"total_entities"`
	EntitiesScanned int              `json:"entities_scanned"`
	Truncated       bool             `json:"truncated"`
	NextCursor      string           `json:"next_cursor,omitempty"`
	Children        []HierarchyChild `json:"children"`
}
