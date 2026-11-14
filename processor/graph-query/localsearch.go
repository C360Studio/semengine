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
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/graphview"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// localPastUnreadable is the Warn a local search answered past an unreadable group record logs, and
// localPastUnreadableReason why answering is safe.
const (
	localPastUnreadable       = "local search answered past an unreadable community record"
	localPastUnreadableReason = "the record only describes the group; the matches come from the word index"
)

// localGroup is the group a local search runs in: its COMMUNITY_INDEX key and its ID, the entity-ID
// prefix the word search is scoped to.
type localGroup struct {
	key string
	id  string
}

// decodeLocalSearchRequest decodes strictly and checks every field, before any request. It returns
// the group entity_id is in at level. The query is only checked for being blank: what it holds is
// the word index's to accept or refuse, so there is one rule for what a search accepts.
func decodeLocalSearchRequest(data []byte) (graph.LocalSearchRequest, localGroup, error) {
	var request graph.LocalSearchRequest
	if err := decodeStrict(data, &request); err != nil {
		return request, localGroup{}, err
	}
	if err := semtypes.ValidateEntityID(request.EntityID); err != nil {
		return request, localGroup{}, fmt.Errorf("entity_id: %w", err)
	}
	// GroupKey and LevelPrefix own which levels exist and how a group is named.
	key, err := clustering.GroupKey(request.EntityID, request.Level)
	if err != nil {
		return request, localGroup{}, fmt.Errorf("level: %w", err)
	}
	prefix, err := clustering.LevelPrefix(request.Level)
	if err != nil {
		return request, localGroup{}, fmt.Errorf("level: %w", err)
	}
	if strings.TrimSpace(request.Query) == "" {
		return request, localGroup{}, errors.New("query is blank")
	}
	if request.Limit < 0 || request.Limit > graph.MaxSimilarityLimit {
		return request, localGroup{}, fmt.Errorf("limit %d is not 0 or 1 to %d", request.Limit, graph.MaxSimilarityLimit)
	}
	return request, localGroup{key: key, id: strings.TrimPrefix(key, prefix)}, nil
}

// groupRecord reads key's record from the current community view, and says whether it could. A
// record only describes the group, so no outcome refuses the search: no view, or one catching up,
// lost or stopped, is not_ready; an absent key is none; a poisoned key is unreadable.
func (c *Component) groupRecord(key string) (clustering.Community, graph.RecordState) {
	view := c.communities.Load()
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
	encoded, cut, err := fitFirst(len(all), limit, func(n int) ([]byte, error) {
		kept := data
		kept.Matches = all[:n]
		kept.Truncated = data.Truncated || n < len(all)
		kept.Answer = localSearchAnswer(kept)
		return encode(kept)
	})
	return encoded, data.Truncated || cut, err
}

// fitFirst returns encodeFirst(n) for the largest n up to count whose encoding is at most limit
// bytes, and whether that n is below count. An encoding shrinks as items are left out, so n is
// found by bisection. It refuses when even encodeFirst(0) does not fit. localSearch and the text
// search share it.
func fitFirst(count, limit int, encodeFirst func(n int) ([]byte, error)) ([]byte, bool, error) {
	encoded, err := encodeFirst(count)
	if err != nil {
		return nil, false, err
	}
	if len(encoded) <= limit {
		return encoded, false, nil
	}
	// The largest count in lo to hi that fits; all of them do not.
	lo, hi := 0, count-1
	var best []byte
	for lo <= hi {
		mid := lo + (hi-lo)/2
		candidate, err := encodeFirst(mid)
		if err != nil {
			return nil, false, err
		}
		if len(candidate) <= limit {
			best, lo = candidate, mid+1
		} else {
			hi = mid - 1
		}
	}
	if best == nil {
		return nil, false, fmt.Errorf("a reply with no entities is over the %d-byte limit", limit)
	}
	return best, true, nil
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
		record, reply.RecordState = c.groupRecord(group.key)
		if reply.RecordState == graph.RecordRead {
			reply.Record = &graph.LocalSearchRecord{
				MemberCount: record.MemberCount, CommunitySummary: record.StatisticalSummary, Keywords: record.Keywords,
			}
		}
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

func communityNotReady(err error) error {
	return errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady, fmt.Errorf("community index not ready: %w", err))
}

// unanswered is graph-query's own refusal of a request it could not answer, counted as an error.
func (c *Component) unanswered(err error) error {
	c.lifecycle.CountError()
	return internalError(err)
}
