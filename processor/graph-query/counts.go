package graphquery

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/c360studio/semengine/graph"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// countPage is one prefix page: its IDs, which the count verbs read, and its rows as sent, which
// local search ranks. total is the group as listed, each ID once, and an ID may be missing from it
// under concurrent overwrites: nats.go does not promise a listing names every key then.
type countPage struct {
	ids        []string
	rows       []json.RawMessage // each row as the page sent it, in the page's order, beside its ID in ids
	total      int
	nextCursor string
}

// readCountPage reads one prefix page at the maximum limit, sending cursor unchanged. A refusal from
// the prefix verb reaches the caller as it arrived.
//
// A page that cannot be counted is refused rather than read as zero or skipped past: one with no
// entities or total key, since "{}" decodes, or a row whose ID does not parse or is not under prefix.
// Real graph-ingest sends entities null for an empty group, which is an answer.
func (c *Component) readCountPage(ctx context.Context, subject, prefix, cursor string) (countPage, error) {
	request, err := json.Marshal(graph.PrefixQueryRequest{Prefix: prefix, Limit: graph.MaxPrefixQueryLimit, Cursor: cursor})
	if err != nil {
		return countPage{}, internalError(err)
	}
	raw, err := c.ask(ctx, subject, request)
	if err != nil {
		return countPage{}, err
	}
	var reply struct {
		Entities   json.RawMessage `json:"entities"`
		Total      *int            `json:"total"`
		NextCursor string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return countPage{}, c.brokenReply(subject, err)
	}
	if len(reply.Entities) == 0 {
		return countPage{}, c.brokenReply(subject, errors.New("names no entities"))
	}
	if reply.Total == nil {
		return countPage{}, c.brokenReply(subject, errors.New("names no total"))
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(reply.Entities, &rows); err != nil {
		return countPage{}, c.brokenReply(subject, err)
	}
	ids := make([]string, 0, len(rows))
	for _, raw := range rows {
		var row struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &row); err != nil {
			return countPage{}, c.brokenReply(subject, err)
		}
		if !underPrefix(row.ID, prefix) {
			return countPage{}, c.brokenReply(subject, fmt.Errorf("row %q is not under prefix %q", row.ID, prefix))
		}
		ids = append(ids, row.ID)
	}
	if err := graph.ValidateDecodedEntityIDs(ids); err != nil {
		return countPage{}, c.brokenReply(subject, err)
	}
	return countPage{ids: ids, rows: rows, total: *reply.Total, nextCursor: reply.NextCursor}, nil
}

// underPrefix says whether id is prefix itself or within it, as graph-ingest's listing reads a prefix:
// whole segments. Empty matches every ID.
func underPrefix(id, prefix string) bool {
	return prefix == "" || id == prefix || strings.HasPrefix(id, prefix+".")
}

// hierarchyStats answers how one page of a group divides among its next segment. The reply is bare
// and min_revision is ignored, since the verb reads graph-ingest's live listing and no index.
// Counted as pathSearch is: a request is one input and at most one error.
func (c *Component) hierarchyStats(prefixSubject string) queryHandler {
	return func(ctx context.Context, data []byte) ([]byte, error) {
		c.lifecycle.CountInput(len(data))
		var request graph.HierarchyStatsRequest
		if err := decodeStrict(data, &request); err != nil {
			return nil, invalidRequest(err)
		}
		if request.Prefix != "" {
			if err := semtypes.ValidateEntityIDPrefix(request.Prefix); err != nil {
				return nil, err
			}
		}
		page, err := c.readCountPage(ctx, prefixSubject, request.Prefix, request.Cursor)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(graph.HierarchyStatsData{
			Prefix:          request.Prefix,
			TotalEntities:   page.total,
			EntitiesScanned: len(page.ids),
			Truncated:       page.nextCursor != "",
			NextCursor:      page.nextCursor,
			Children:        childCounts(request.Prefix, page.ids),
		})
		if err != nil {
			return nil, c.brokenReply(prefixSubject, err)
		}
		return encoded, nil
	}
}

// childCounts counts ids, each under prefix, by the prefix one segment longer, by count descending then
// prefix. An ID that is the prefix has no such segment and counts in no child. Never nil, so an empty
// answer's children key reads [].
func childCounts(prefix string, ids []string) []graph.HierarchyChild {
	lead := ""
	if prefix != "" {
		lead = prefix + "."
	}
	counts := make(map[string]int)
	for _, id := range ids {
		if id == prefix {
			continue
		}
		segment, _, _ := strings.Cut(strings.TrimPrefix(id, lead), ".")
		counts[lead+segment]++
	}
	children := make([]graph.HierarchyChild, 0, len(counts))
	for child, count := range counts {
		children = append(children, graph.HierarchyChild{Prefix: child, Count: count})
	}
	slices.SortFunc(children, func(a, b graph.HierarchyChild) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Prefix, b.Prefix))
	})
	return children
}
