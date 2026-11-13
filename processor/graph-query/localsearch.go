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
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/graphview"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// minLocalSearchScore is the score an entity needs to match: a constant, since min_text_relevance
// stays refused.
const minLocalSearchScore = 0.3

// decodeLocalSearchRequest decodes strictly and checks every field, before any read. It returns the
// query's terms: lowercased, split on white space, each once, in the order first typed.
func decodeLocalSearchRequest(data []byte) (graph.LocalSearchRequest, []string, error) {
	var request graph.LocalSearchRequest
	if err := decodeStrict(data, &request); err != nil {
		return request, nil, err
	}
	if err := semtypes.ValidateEntityID(request.EntityID); err != nil {
		return request, nil, fmt.Errorf("entity_id: %w", err)
	}
	// GroupKey owns which levels exist; the key itself is read with the record.
	if _, err := clustering.GroupKey(request.EntityID, request.Level); err != nil {
		return request, nil, fmt.Errorf("level: %w", err)
	}
	terms, err := checkedSearchTerms(request.Query)
	if err != nil {
		return request, nil, err
	}
	return request, terms, nil
}

// checkedSearchTerms is searchTerms, refusing a query that holds no term or more than
// graph.MaxSearchTerms: the check localSearch and the text tier share.
func checkedSearchTerms(query string) ([]string, error) {
	terms := searchTerms(query)
	if len(terms) == 0 {
		return nil, errors.New("query holds no terms")
	}
	if len(terms) > graph.MaxSearchTerms {
		return nil, fmt.Errorf("query holds %d distinct terms, more than %d", len(terms), graph.MaxSearchTerms)
	}
	return terms, nil
}

// searchTerms lowercases query and splits it on white space, then removes repeats: a repeated term
// would weigh twice, so how a query is typed would change who matches. It stops at the first
// distinct term over graph.MaxSearchTerms, so a long query costs a bounded scan, never a quadratic
// one.
func searchTerms(query string) []string {
	var terms []string
	for term := range strings.FieldsSeq(strings.ToLower(query)) {
		if slices.Contains(terms, term) {
			continue
		}
		terms = append(terms, term)
		if len(terms) > graph.MaxSearchTerms {
			break
		}
	}
	return terms
}

// searchEntity is what the score reads of a page's entity. An object is kept as its JSON, so a value
// the score does not read, such as a large integer, is never converted.
type searchEntity struct {
	ID      string `json:"id"`
	Triples []struct {
		Predicate string          `json:"predicate"`
		Object    json.RawMessage `json:"object"`
	} `json:"triples"`
}

func decodeSearchEntity(raw json.RawMessage) (searchEntity, error) {
	var entity searchEntity
	if err := json.Unmarshal(raw, &entity); err != nil {
		return entity, fmt.Errorf("entity does not decode: %w", err)
	}
	return entity, nil
}

// score is the text score: per term, 1.0 when it equals the ID's type position, else 0.5 when it is
// within a predicate or a string value, all lowercased; the sum over the term count. An ID that
// does not parse has no type.
func (e searchEntity) score(terms []string) float64 {
	if len(terms) == 0 {
		return 0
	}
	var entityType string
	if eid, err := semtypes.ParseEntityID(e.ID); err == nil {
		entityType = strings.ToLower(eid.Type)
	}
	var text strings.Builder
	for _, triple := range e.Triples {
		text.WriteString(strings.ToLower(triple.Predicate))
		text.WriteString(" ")
		var value string
		if json.Unmarshal(triple.Object, &value) == nil {
			text.WriteString(strings.ToLower(value))
			text.WriteString(" ")
		}
	}
	searchable := text.String()
	total := 0.0
	for _, term := range terms {
		switch {
		case term == entityType:
			total += 1.0
		case strings.Contains(searchable, term):
			total += 0.5
		}
	}
	return total / float64(len(terms))
}

// label is the first non-empty string value, in triple order, of the first of predicates the entity
// holds one for: predicates is the label set, most salient first (labelOrder). An entity with none
// has no label.
func (e searchEntity) label(predicates []string) string {
	for _, predicate := range predicates {
		for _, triple := range e.Triples {
			var value string
			if triple.Predicate == predicate && json.Unmarshal(triple.Object, &value) == nil && value != "" {
				return value
			}
		}
	}
	return ""
}

// rankedEntity is a page's entity that matched: its ID, its label, its score and its bytes as the page
// sent them.
type rankedEntity struct {
	id    string
	label string
	score float64
	raw   json.RawMessage
}

// compareRanked orders matches best first, ties by entity ID.
func compareRanked(a, b rankedEntity) int {
	return cmp.Or(cmp.Compare(b.score, a.score), cmp.Compare(a.id, b.id))
}

// rankEntities returns the page's entities scoring at least minLocalSearchScore, best first, ties
// by entity ID, each labelled from labels (nil for no label), and the number of distinct entities
// read. An ID the page repeats is read once, its first copy. An entity that does not decode is
// refused, not skipped.
func rankEntities(page []json.RawMessage, terms, labels []string) ([]rankedEntity, int, error) {
	seen := make(map[string]bool, len(page))
	var matches []rankedEntity
	for _, raw := range page {
		entity, err := decodeSearchEntity(raw)
		if err != nil {
			return nil, 0, err
		}
		if seen[entity.ID] {
			continue
		}
		seen[entity.ID] = true
		if score := entity.score(terms); score >= minLocalSearchScore {
			matches = append(matches, rankedEntity{id: entity.ID, label: entity.label(labels), score: score, raw: raw})
		}
	}
	slices.SortFunc(matches, compareRanked)
	return matches, len(seen), nil
}

// localSearchAnswer is the answer text, {n} being the entities in the reply. A record's summary
// already states its member count and themes, so they are written only when it has none.
func localSearchAnswer(data graph.LocalSearchData) string {
	answer := fmt.Sprintf("Found %d matching entities among %d read from %s.",
		len(data.Entities), data.EntitiesScanned, data.CommunityID)
	if data.CommunitySummary != "" {
		return answer + " " + data.CommunitySummary
	}
	answer += fmt.Sprintf(" It has %d members.", data.MemberCount)
	if len(data.Keywords) > 0 {
		answer += " Key themes: " + strings.Join(data.Keywords[:min(len(data.Keywords), 5)], ", ") + "."
	}
	return answer
}

// fitLocalSearch encodes data, with its answer, keeping the most of its best entities whose
// encoding is at most limit bytes; leaving any out sets EntitiesTruncated. It also reports whether
// the encoded answer is truncated. It refuses when even no entities do not fit.
func fitLocalSearch(data graph.LocalSearchData, limit int, encode func(graph.LocalSearchData) ([]byte, error)) ([]byte, bool, error) {
	all := data.Entities
	encoded, cut, err := fitFirst(len(all), limit, func(n int) ([]byte, error) {
		kept := data
		kept.Entities = all[:n]
		kept.EntitiesTruncated = data.EntitiesTruncated || n < len(all)
		kept.Answer = localSearchAnswer(kept)
		return encode(kept)
	})
	return encoded, data.EntitiesTruncated || cut, err
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

// localSearch answers from the record of entity_id's group at level, then one page of the group
// Every check of the request comes before any read. A view that is missing, catching up, lost or
// stopped refuses index_not_ready, transient, without an error in Health. With no record, the
// entity is read: when it exists, the group is not yet indexed; when it does not, the entity verb's
// refusal is the answer, since no index state makes it exist. An unreadable record is refused
// transient internal naming its key, and counts as an error. min_revision is compared with the
// record's entity_revision, the reply's indexed_revision, before the page is read, so the page
// reflects at least as much as the revision claimed. A request is one input and at most one error,
// as for the other composed verbs.
func (c *Component) localSearch(entitySubject, prefixSubject string) queryHandler {
	return func(ctx context.Context, data []byte) ([]byte, error) {
		c.lifecycle.CountInput(len(data))
		request, terms, err := decodeLocalSearchRequest(data)
		if err != nil {
			return nil, invalidRequest(err)
		}
		key, err := clustering.GroupKey(request.EntityID, request.Level)
		if err != nil {
			return nil, invalidRequest(err) // decodeLocalSearchRequest has already checked both
		}
		record, err := c.communityRecord(key)
		if errors.Is(err, graphview.ErrKeyNotFound) {
			return nil, c.ungrouped(ctx, entitySubject, request.EntityID)
		}
		if err != nil {
			return nil, err
		}
		if request.MinRevision > record.EntityRevision {
			return nil, errs.ClassifiedCodeDetail(errs.ErrorTransient, graph.ErrorCodeRevisionNotReached,
				map[string]any{"indexed_revision": record.EntityRevision},
				fmt.Errorf("min_revision %d is above %s's entity_revision %d", request.MinRevision, key, record.EntityRevision))
		}
		page, err := c.readCountPage(ctx, prefixSubject, record.ID, "")
		if err != nil {
			return nil, err
		}
		matches, scanned, err := rankEntities(page.rows, terms, nil)
		if err != nil {
			return nil, c.brokenReply(prefixSubject, err)
		}
		entities := make([]json.RawMessage, 0, len(matches))
		for _, match := range matches {
			entities = append(entities, match.raw)
		}
		limit, err := c.natsClient.MaxPayload()
		if err != nil {
			return nil, c.unanswered(err)
		}
		encoded, truncated, err := fitLocalSearch(graph.LocalSearchData{
			CommunityID:       record.ID,
			Level:             record.Level,
			MemberCount:       record.MemberCount,
			CommunitySummary:  record.StatisticalSummary,
			Keywords:          record.Keywords,
			Entities:          entities,
			EntitiesScanned:   scanned,
			EntitiesTruncated: page.nextCursor != "",
		}, int(limit), func(data graph.LocalSearchData) ([]byte, error) {
			response, err := graph.NewQueryResponse(graph.ResponderGraphQuery, record.EntityRevision, data)
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
		return encoded, nil
	}
}

// communityRecord reads key's record from the current community view. No view, or one not caught up,
// lost or stopped, is index_not_ready; a poisoned key is graph-query's own internal refusal, counted;
// an absent key returns graphview.ErrKeyNotFound for the caller to read the entity.
func (c *Component) communityRecord(key string) (clustering.Community, error) {
	view := c.communities.Load()
	if view == nil {
		return clustering.Community{}, communityNotReady(errors.New("no view of " + graph.BucketCommunityIndex))
	}
	record, _, err := view.Get(key)
	var poison *graphview.PoisonError
	switch {
	case err == nil:
		return record, nil
	case errors.Is(err, graphview.ErrKeyNotFound):
		return clustering.Community{}, err
	case errors.As(err, &poison):
		return clustering.Community{}, c.unanswered(fmt.Errorf("community record %s is unreadable: %w", key, err))
	default:
		return clustering.Community{}, communityNotReady(err)
	}
}

// ungrouped answers a group with no record: entityID's own read decides. A refusal, or no answer,
// reaches the caller as ask returns it; an entity that exists is in a group not yet published.
func (c *Component) ungrouped(ctx context.Context, entitySubject, entityID string) error {
	request, err := json.Marshal(graph.EntityRequest{ID: entityID})
	if err != nil {
		return internalError(err)
	}
	if _, err := c.ask(ctx, entitySubject, request); err != nil {
		return err
	}
	return communityNotReady(fmt.Errorf("%s holds no record of %s's group", graph.BucketCommunityIndex, entityID))
}

func communityNotReady(err error) error {
	return errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady, fmt.Errorf("community index not ready: %w", err))
}

// unanswered is graph-query's own refusal of a request it could not answer, counted as an error.
func (c *Component) unanswered(err error) error {
	c.lifecycle.CountError()
	return internalError(err)
}
