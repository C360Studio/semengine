package graphquery

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/c360studio/semengine/graph"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// defaultExamplesPerType is enough for a caller to see the shape of an ID, while keeping the reply
// compact.
const defaultExamplesPerType = 2

// summary answers what the graph holds: its predicates, then one page of its entities by type.
// Predicates come first so an index that cannot answer, whose refusal reaches the caller as it
// arrived, costs no scan. The page is read after them, so the two are not a snapshot. Predicates
// excluded, no index is read and min_revision is ignored, and the answer's indexed_revision is 0,
// claiming nothing. Counted as pathSearch is: a request is one input and at most one error.
func (c *Component) summary(predicateSubject, prefixSubject string) queryHandler {
	return func(ctx context.Context, data []byte) ([]byte, error) {
		c.lifecycle.CountInput(len(data))
		var request graph.SummaryRequest
		if err := decodeStrict(data, &request); err != nil {
			return nil, invalidRequest(err)
		}
		if request.ExamplesPerType < 0 {
			return nil, invalidRequest(errors.New("examples_per_type is negative"))
		}
		if request.Prefix != "" {
			if !request.ExcludePredicates {
				return nil, invalidRequest(errors.New("prefix needs exclude_predicates: predicate counts cover the whole graph"))
			}
			if err := semtypes.ValidateEntityIDPrefix(request.Prefix); err != nil {
				return nil, err
			}
		}
		var predicates []graph.PredicateSummary
		var revision uint64
		if !request.ExcludePredicates {
			var err error
			predicates, revision, err = c.predicateRows(ctx, predicateSubject, request.MinRevisionField)
			if err != nil {
				return nil, err
			}
		}
		page, err := c.readCountPage(ctx, prefixSubject, request.Prefix, request.Cursor)
		if err != nil {
			return nil, err
		}
		examples := request.ExamplesPerType
		if examples == 0 {
			examples = defaultExamplesPerType
		}
		types, err := entityTypes(page.ids, examples)
		if err != nil {
			return nil, c.brokenReply(prefixSubject, err)
		}
		return envelope(c, prefixSubject, revision, graph.SummaryData{
			TotalEntities:   page.total,
			EntitiesScanned: len(page.ids),
			Truncated:       page.nextCursor != "",
			NextCursor:      page.nextCursor,
			EntityTypes:     types,
			Predicates:      predicates,
		})
	}
}

// predicateRows asks the predicateList verb and reads its reply, with the revision it answered at. A
// reply with no list is refused rather than read as none: real graph-index always sends one, and [] is
// how it says none.
func (c *Component) predicateRows(ctx context.Context, subject string, minRevision graph.MinRevisionField) ([]graph.PredicateSummary, uint64, error) {
	request, err := json.Marshal(graph.PredicateListQuery{MinRevisionField: minRevision})
	if err != nil {
		return nil, 0, internalError(err)
	}
	raw, err := c.ask(ctx, subject, request)
	if err != nil {
		return nil, 0, err
	}
	reply, err := decodeIndexReply[graph.PredicateListData](c, subject, raw)
	if err != nil {
		return nil, 0, err
	}
	if reply.Data.Predicates == nil {
		return nil, 0, c.brokenReply(subject, errors.New("holds no predicates"))
	}
	return reply.Data.Predicates, reply.IndexedRevision, nil
}

// entityTypes groups ids by system.domain.type, read by named field (ADR-102), by count descending then
// type, keeping the first examplesPerType of each in the order given. Never nil, so an empty answer's
// entity_types key reads []. An ID that does not parse is refused, never skipped, though the page read
// refuses such a row first.
func entityTypes(ids []string, examplesPerType int) ([]graph.EntityTypeSummary, error) {
	buckets := make(map[string]*graph.EntityTypeSummary)
	for _, id := range ids {
		parsed, err := semtypes.ParseEntityID(id)
		if err != nil {
			return nil, err
		}
		key := parsed.System + "." + parsed.Domain + "." + parsed.Type
		bucket, ok := buckets[key]
		if !ok {
			bucket = &graph.EntityTypeSummary{Type: key}
			buckets[key] = bucket
		}
		bucket.Count++
		if len(bucket.Examples) < examplesPerType {
			bucket.Examples = append(bucket.Examples, id)
		}
	}
	types := make([]graph.EntityTypeSummary, 0, len(buckets))
	for _, bucket := range buckets {
		types = append(types, *bucket)
	}
	slices.SortFunc(types, func(a, b graph.EntityTypeSummary) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Type, b.Type))
	})
	return types, nil
}
