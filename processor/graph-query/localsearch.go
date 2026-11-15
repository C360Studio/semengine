package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/pkg/graphview"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// localPastUnreadable is the Warn a local search answered past an unreadable group record logs, and
// localPastUnreadableReason why answering is safe.
const (
	localPastUnreadable       = "local search answered past an unreadable community record"
	localPastUnreadableReason = "the record only describes the group; the matches come from the word index"
)

// searchGroup is an entity's community group at one level: its COMMUNITY_INDEX key and its ID, the
// entity-ID prefix a local search is scoped to and a global search lists.
type searchGroup struct {
	key string
	id  string
}

// groupAt is entityID's group at level. GroupKey and LevelPrefix own which levels exist and how a
// group is named; either refuses an ID that is not a literal entity ID or a level outside 0 to 2.
func groupAt(entityID string, level int) (searchGroup, error) {
	key, err := clustering.GroupKey(entityID, level)
	if err != nil {
		return searchGroup{}, err
	}
	prefix, err := clustering.LevelPrefix(level)
	if err != nil {
		return searchGroup{}, err
	}
	return searchGroup{key: key, id: strings.TrimPrefix(key, prefix)}, nil
}

// decodeLocalSearchRequest decodes strictly and checks every field, before any request. It returns
// the group entity_id is in at level. The query is only checked for being blank: what it holds is
// the word index's to accept or refuse, so there is one rule for what a search accepts.
func decodeLocalSearchRequest(data []byte) (graph.LocalSearchRequest, searchGroup, error) {
	var request graph.LocalSearchRequest
	if err := decodeStrict(data, &request); err != nil {
		return request, searchGroup{}, err
	}
	if err := semtypes.ValidateEntityID(request.EntityID); err != nil {
		return request, searchGroup{}, fmt.Errorf("entity_id: %w", err)
	}
	group, err := groupAt(request.EntityID, request.Level)
	if err != nil {
		return request, searchGroup{}, fmt.Errorf("level: %w", err)
	}
	if strings.TrimSpace(request.Query) == "" {
		return request, searchGroup{}, errors.New("query is blank")
	}
	if err := checkSearchLimit(request.Limit); err != nil {
		return request, searchGroup{}, err
	}
	return request, group, nil
}

// checkSearchLimit refuses a search limit other than 0, meaning graph.DefaultSimilarityLimit, or 1
// to graph.MaxSimilarityLimit.
func checkSearchLimit(limit int) error {
	if limit < 0 || limit > graph.MaxSimilarityLimit {
		return fmt.Errorf("limit %d is not 0 or 1 to %d", limit, graph.MaxSimilarityLimit)
	}
	return nil
}

// groupRecord reads key's record from view, and says whether it could. A record only describes the
// group, so no outcome refuses the search: no view, or one catching up, lost or stopped, is
// not_ready; an absent key is none; a poisoned key is unreadable.
func groupRecord(view *communityView, key string) (clustering.Community, graph.RecordState) {
	if view == nil {
		return clustering.Community{}, graph.RecordNotReady
	}
	record, _, err := view.Get(key)
	var poison *graphview.PoisonError
	switch {
	case err == nil:
		return record, graph.RecordRead
	case errors.Is(err, graphview.ErrKeyNotFound):
		return clustering.Community{}, graph.RecordNone
	case errors.As(err, &poison):
		return clustering.Community{}, graph.RecordUnreadable
	default:
		return clustering.Community{}, graph.RecordNotReady
	}
}

// attachedRecord is record as a search reply attaches it: present exactly when state is read.
func attachedRecord(record clustering.Community, state graph.RecordState) *graph.LocalSearchRecord {
	if state != graph.RecordRead {
		return nil
	}
	return &graph.LocalSearchRecord{MemberCount: record.MemberCount, CommunitySummary: record.StatisticalSummary, Keywords: record.Keywords}
}

// localSearchAnswer is the answer text: the word index's count in the group; how many are shown when
// the answer is cut; and the record's summary when one was read.
func localSearchAnswer(data graph.LocalSearchData) string {
	answer := fmt.Sprintf("Found %d matching entities in %s.", data.Matched, data.CommunityID)
	if data.Truncated {
		answer += fmt.Sprintf(" Showing the first %d.", len(data.Matches))
	}
	if data.Record != nil && data.Record.CommunitySummary != "" {
		answer += " " + data.Record.CommunitySummary
	}
	return answer
}

// fitLocalSearch encodes data, with its answer, keeping the most of its best matches whose encoding
// is at most limit bytes; leaving any out sets Truncated. It also reports whether the encoded answer
// is truncated. It refuses when even no matches do not fit.
func fitLocalSearch(data graph.LocalSearchData, limit int, encode func(graph.LocalSearchData) ([]byte, error)) ([]byte, bool, error) {
	all := data.Matches
	encoded, n, err := fitFirst(len(all), limit, func(n int) ([]byte, error) {
		kept := data
		kept.Matches = all[:n]
		kept.Truncated = data.Truncated || n < len(all)
		kept.Answer = localSearchAnswer(kept)
		return encode(kept)
	})
	return encoded, data.Truncated || n < len(all), err
}

// fitFirst returns encodeFirst(n) and n, for the largest n up to count whose encoding is at most
// limit bytes. An encoding shrinks as items are left out, so n is found by bisection. It refuses
// when even encodeFirst(0) does not fit. Local and global search share it.
func fitFirst(count, limit int, encodeFirst func(n int) ([]byte, error)) ([]byte, int, error) {
	encoded, err := encodeFirst(count)
	if err != nil {
		return nil, 0, err
	}
	if len(encoded) <= limit {
		return encoded, count, nil
	}
	// The largest count in lo to hi that fits; all of them do not.
	lo, hi := 0, count-1
	var best []byte
	kept := 0
	for lo <= hi {
		mid := lo + (hi-lo)/2
		candidate, err := encodeFirst(mid)
		if err != nil {
			return nil, 0, err
		}
		if len(candidate) <= limit {
			best, kept, lo = candidate, mid, mid+1
		} else {
			hi = mid - 1
		}
	}
	if best == nil {
		return nil, 0, fmt.Errorf("a reply with no entities is over the %d-byte limit", limit)
	}
	return best, kept, nil
}

// localSearch answers with the word index's matches in entity_id's group at level: one word search
// scoped to the group's ID, passing query, limit and min_revision unchanged, then one batch read of
// its hits. Every check of the request comes before any request. A refusal from either verb reaches
// the caller as it arrived. The reply's indexed_revision is the word search's.
//
// The group's record is attached when the community view holds one, and never decides the answer:
// record_state says when it is missing. An answer past an unreadable record is a declared degrade,
// counting one error and logging one Warn once the reply is encoded. A request is one input and at
// most one error, as for the other composed verbs.
func (c *Component) localSearch(lexicalSubject, batchSubject string) queryHandler {
	return func(ctx context.Context, data []byte) ([]byte, error) {
		c.lifecycle.CountInput(len(data))
		request, group, err := decodeLocalSearchRequest(data)
		if err != nil {
			return nil, invalidRequest(err)
		}
		search, err := c.wordSearch(ctx, lexicalSubject, graph.LexicalSearchRequest{
			Query: request.Query, Limit: request.Limit, Scope: []string{group.id}, MinRevisionField: request.MinRevisionField,
		})
		if err != nil {
			return nil, err
		}
		hits := search.Data.Results
		matches, missing, err := c.readMatches(ctx, batchSubject, hits, c.labelPredicates)
		if err != nil {
			return nil, err
		}
		reply := graph.LocalSearchData{
			CommunityID:      group.id,
			Level:            request.Level,
			Matches:          matches,
			Matched:          search.Data.Matched,
			Excluded:         search.Data.Excluded,
			Missing:          missing,
			BodiesNotIndexed: search.Data.BodiesNotIndexed,
			Truncated:        search.Data.Matched > len(hits) || missing > 0,
		}
		var record clustering.Community
		record, reply.RecordState = groupRecord(c.communities.Load(), group.key)
		reply.Record = attachedRecord(record, reply.RecordState)
		limit, err := c.natsClient.MaxPayload()
		if err != nil {
			return nil, c.unanswered(err)
		}
		encoded, truncated, err := fitLocalSearch(reply, int(limit), func(data graph.LocalSearchData) ([]byte, error) {
			response, err := graph.NewQueryResponse(graph.ResponderGraphQuery, search.IndexedRevision, data)
			if err != nil {
				return nil, err
			}
			return json.Marshal(response)
		})
		if err != nil {
			return nil, c.unanswered(err)
		}
		if truncated {
			c.metrics.searchTruncated.WithLabelValues("localSearch").Inc()
		}
		if reply.RecordState == graph.RecordUnreadable {
			c.lifecycle.CountError()
			c.logger.Warn(localPastUnreadable, slog.String("verb", "localSearch"), slog.Int("level", request.Level),
				slog.Int("unreadable", 1), slog.String("community_id", group.id), slog.String("reason", localPastUnreadableReason))
		}
		return encoded, nil
	}
}

// unanswered is graph-query's own refusal of a request it could not answer, counted as an error.
func (c *Component) unanswered(err error) error {
	c.lifecycle.CountError()
	return internalError(err)
}
