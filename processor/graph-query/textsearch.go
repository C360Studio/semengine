package graphquery

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

// textPastUnreadable is the Warn an answer past unreadable community records logs, and
// textPastUnreadableReason why answering is safe.
const (
	textPastUnreadable       = "global search answered past unreadable community records"
	textPastUnreadableReason = "a record only describes its group; the matches come from the word index"
)

// mergeFailed is the Warn a searchGraph answered without the embedding list logs, and
// mergeFailedReason why answering is safe.
const (
	mergeFailed       = "searchGraph answered the word list alone: the embedding search failed"
	mergeFailedReason = "the reply names the failed merge, and the word list is the answer searchGraph gives with no embedding search"
)

// globalText is global search's answer: one unscoped word search, passing query, limit and
// min_revision unchanged, then one batch read of its hits, grouped by their community at the
// request's level, then the route's reference verified, with the hints listed beside the matches. A
// refusal from either search verb reaches the caller as it arrived, before any hint is read; a hit
// whose ID names no group is the word index's broken reply. The reply, fit to max_payload, is under
// the word search's revision. verb is the verb asked, which the truncation counter and the Warn
// name.
//
// With subjects.semantic set (searchGraph when the composition runs graph-embedding), the answer is
// mergedText's.
//
// A group's record is attached when the community view holds one, and never decides the answer:
// record_state says when it is missing. An answer past unreadable records is a declared degrade: it
// sets unreadable_groups, counts one error and logs one Warn, both only once the reply is encoded,
// so a request that fails to fit counts the fit's one error alone.
func (c *Component) globalText(ctx context.Context, verb string, subjects globalSearchSubjects, request graph.GlobalSearchRequest, route globalRoute) ([]byte, error) {
	if subjects.semantic != "" {
		return c.mergedText(ctx, verb, subjects, request, route)
	}
	search, err := c.wordSearch(ctx, subjects.lexical, graph.LexicalSearchRequest{
		Query: request.Query, Limit: request.Limit, MinRevisionField: request.MinRevisionField,
	})
	if err != nil {
		return nil, err
	}
	hitGroups, err := c.groupHits(subjects.lexical, search.Data.Results, request.Level)
	if err != nil {
		return nil, err
	}
	answer := listedText{search: search, list: unranked(search.Data.Results), hitGroups: hitGroups}
	return c.answerText(ctx, verb, subjects, request, route, answer)
}

// mergedText is searchGraph's answer when the composition runs graph-embedding. It sends the word
// search and the embedding search, each graph.MaxSimilarityLimit deep whatever the request's limit,
// before waiting on either, so the embedding wait is not added to the word wait; merges the two
// lists (mergeRanks) and keeps the first limit of the merge. The merge hint says the embedding list
// was read. When the embedding search is refused, broken or silent past query_timeout_ms, the answer
// is the word list in its own order, with the merge hint not honoured, naming why: never a refusal.
// That answer is a declared degrade: once the reply is encoded it logs one Warn and adds one to the
// merge-failed counter, so a request that fails to fit counts nothing there.
//
// A word search refused index_not_ready is answered by semanticAlone. Any other word refusal is
// returned as it arrived; the embedding request is cancelled and the call waits for it to end, never
// for its reply, so nothing it started outlives it, and its end counts no error.
func (c *Component) mergedText(ctx context.Context, verb string, subjects globalSearchSubjects, request graph.GlobalSearchRequest, route globalRoute) ([]byte, error) {
	await, stop := goSearch(ctx, func(ctx context.Context) semanticOutcome {
		return c.semanticSearch(ctx, subjects.semantic, request.Query)
	})
	defer stop()
	search, err := c.wordSearch(ctx, subjects.lexical, graph.LexicalSearchRequest{
		Query: request.Query, Limit: graph.MaxSimilarityLimit, MinRevisionField: request.MinRevisionField,
	})
	if err != nil {
		if isIndexNotReady(err) {
			return c.semanticAlone(ctx, subjects, request, route, await(), err)
		}
		return nil, err
	}
	words := search.Data.Results
	hitGroups, err := c.groupHits(subjects.lexical, words, request.Level)
	if err != nil {
		return nil, err
	}
	semantic := c.readSemantic(subjects.semantic, await(), detailMerged)
	list := wordList(words)
	if semantic.hint.Honoured {
		// An embedding hit whose ID names no group is the embedding index's broken reply: the
		// answer is then the word list, the merge named as failed.
		vectorGroups, err := c.groupVectorHits(subjects.semantic, semantic.hits, request.Level)
		if err != nil {
			semantic = semanticRead{hint: notHonouredBy(semantic.hint, err)}
		} else {
			maps.Copy(hitGroups, vectorGroups)
			list = mergeRanks(words, semantic.hits)
		}
	}
	limit := cmp.Or(request.Limit, graph.DefaultSimilarityLimit)
	more := len(list) > limit
	list = list[:min(limit, len(list))]
	answer := listedText{
		search: search, list: list, more: more, hitGroups: hitGroups,
		merge: &semantic.hint, semantic: semantic.data,
	}
	encoded, err := c.answerText(ctx, verb, subjects, request, route, answer)
	if err != nil {
		return nil, err
	}
	if !semantic.hint.Honoured {
		c.metrics.searchMergeFailed.Inc()
		c.logger.Warn(mergeFailed, slog.String("verb", verb), slog.String("code", semantic.hint.Code),
			slog.String("error", semantic.hint.Reason), slog.String("reason", mergeFailedReason))
	}
	return encoded, nil
}

// listedText is what a global search read before its matches are read back: the word search's reply;
// list, the hits to read back, best first, each with the score and ranks its match carries; more,
// whether the lists read held more entities than list; each hit's group; and, for searchGraph when
// the composition runs graph-embedding, the merge hint and the embedding search's data.
type listedText struct {
	search    graph.QueryResponse[graph.LexicalSearchData]
	list      []mergedHit
	more      bool
	hitGroups map[string]searchGroup
	merge     *graph.GlobalSearchHint
	semantic  json.RawMessage
}

// groupHits is each word hit's group at level; a hit whose ID names no group is the word index's
// broken reply.
func (c *Component) groupHits(lexicalSubject string, hits []graph.LexicalHit, level int) (map[string]searchGroup, error) {
	hitGroups := make(map[string]searchGroup, len(hits))
	for _, hit := range hits {
		group, err := groupAt(hit.EntityID, level)
		if err != nil {
			return nil, c.brokenReply(lexicalSubject, err)
		}
		hitGroups[hit.EntityID] = group
	}
	return hitGroups, nil
}

// groupVectorHits is each embedding hit's group at level; a hit whose ID names no group is the
// embedding index's broken reply, counted.
func (c *Component) groupVectorHits(semanticSubject string, hits []graph.SimilarityHit, level int) (map[string]searchGroup, error) {
	hitGroups := make(map[string]searchGroup, len(hits))
	for _, hit := range hits {
		group, err := groupAt(hit.EntityID, level)
		if err != nil {
			return nil, c.brokenReply(semanticSubject, err)
		}
		hitGroups[hit.EntityID] = group
	}
	return hitGroups, nil
}

// answerText reads answer's list back with one batch request, each match carrying its list entry's
// score and ranks, verifies the route's reference, and encodes the reply, fit to max_payload, under
// the word search's revision. Truncated is set when the lists read held more than the list, the
// word index matched more than it returned, a hit was missing or damaged, or matches were left out
// to fit.
func (c *Component) answerText(ctx context.Context, verb string, subjects globalSearchSubjects, request graph.GlobalSearchRequest, route globalRoute, answer listedText) ([]byte, error) {
	hits := make([]graph.LexicalHit, 0, len(answer.list))
	ranks := make(map[string]mergedHit, len(answer.list))
	for _, hit := range answer.list {
		hits = append(hits, graph.LexicalHit{EntityID: hit.entityID, Score: hit.score})
		ranks[hit.entityID] = hit
	}
	matches, gaps, err := c.readMatches(ctx, subjects.batch, hits, c.labelPredicates)
	if err != nil {
		return nil, err
	}
	for i := range matches {
		matches[i].WordRank, matches[i].VectorRank = ranks[matches[i].ID].wordRank, ranks[matches[i].ID].vectorRank
	}
	search := answer.search
	data := graph.TextSearchData{
		Level:            request.Level,
		Matches:          matches,
		Matched:          search.Data.Matched,
		Excluded:         search.Data.Excluded,
		Missing:          gaps.missing,
		Damaged:          gaps.damaged,
		BodiesNotIndexed: search.Data.BodiesNotIndexed,
		Truncated:        answer.more || search.Data.Matched > len(search.Data.Results) || gaps.left(),
	}
	reply := c.hinted(ctx, subjects, route, request.MinRevision)
	reply.Merge, reply.Semantic = answer.merge, answer.semantic
	limit, err := c.natsClient.MaxPayload()
	if err != nil {
		return nil, c.unanswered(err)
	}
	groupOf := matchGroups(c.communities.Load(), matches, answer.hitGroups)
	encoded, sent, err := fitTextSearch(data, groupOf, int(limit), func(data graph.TextSearchData) ([]byte, error) {
		reply.Text = &data
		response, err := graph.NewQueryResponse(graph.ResponderGraphQuery, search.IndexedRevision, reply)
		if err != nil {
			return nil, err
		}
		return json.Marshal(response)
	})
	if err != nil {
		return nil, c.unanswered(err)
	}
	if sent.Truncated {
		c.metrics.searchTruncated.WithLabelValues(verb).Inc()
	}
	if sent.UnreadableGroups > 0 {
		c.lifecycle.CountError()
		c.logger.Warn(textPastUnreadable, slog.String("verb", verb), slog.Int("level", request.Level),
			slog.Int("unreadable", sent.UnreadableGroups), slog.String("reason", textPastUnreadableReason))
	}
	return encoded, nil
}

// hinted is a global search answer with no text yet: the route's reference verified, first, then
// the hints no read can verify.
func (c *Component) hinted(ctx context.Context, subjects globalSearchSubjects, route globalRoute, minRevision uint64) graph.GlobalSearchData {
	answer := graph.GlobalSearchData{Hints: []graph.GlobalSearchHint{}}
	if verified, ok := c.verify(ctx, subjects, route, minRevision); ok {
		answer.Hints = append(answer.Hints, verified.hint)
		answer.Entity, answer.Path = verified.entity, verified.path
	}
	answer.Hints = append(answer.Hints, route.hints...)
	return answer
}

// semanticAlone answers a searchGraph whose word search was refused index_not_ready, refusal, from
// the embedding search's outcome: text null, since the word index was not read and an empty text
// would say it matched nothing, with the semantic data and the hints, under revision 0, since no
// ENTITY_STATES revision was read. An embedding search that failed returns refusal, the first one,
// as it arrived.
func (c *Component) semanticAlone(ctx context.Context, subjects globalSearchSubjects, request graph.GlobalSearchRequest, route globalRoute, outcome semanticOutcome, refusal error) ([]byte, error) {
	semantic := c.readSemantic(subjects.semantic, outcome, graph.ErrorCodeIndexNotReady)
	if !semantic.hint.Honoured {
		return nil, refusal
	}
	answer := c.hinted(ctx, subjects, route, request.MinRevision)
	answer.Merge, answer.Semantic = &semantic.hint, semantic.data
	return envelope(c, subjects.semantic, 0, answer)
}

// isIndexNotReady says err is a refusal with code index_not_ready.
func isIndexNotReady(err error) bool {
	var refusal *errs.ClassifiedError
	return errors.As(err, &refusal) && refusal.Code == graph.ErrorCodeIndexNotReady
}

// matchGroups is each match's group, by entity ID, as the reply lists it: the group's ID, its
// record state and its record, each group's record read once from view. Matches is left 0 for
// textGroups to count. hitGroups is each hit's group.
func matchGroups(view *communityView, matches []graph.TextSearchMatch, hitGroups map[string]searchGroup) map[string]graph.TextSearchGroup {
	byKey := map[string]graph.TextSearchGroup{}
	groupOf := make(map[string]graph.TextSearchGroup, len(matches))
	for _, match := range matches {
		group := hitGroups[match.ID]
		listed, ok := byKey[group.key]
		if !ok {
			record, state := groupRecord(view, group.key)
			listed = graph.TextSearchGroup{CommunityID: group.id, RecordState: state, Record: attachedRecord(record, state)}
			byKey[group.key] = listed
		}
		groupOf[match.ID] = listed
	}
	return groupOf
}

// textGroups lists the groups holding matches, each once, in the order of its best match, since
// matches are best first; each with the number of matches it holds. It also counts the groups whose
// record is unreadable.
func textGroups(matches []graph.TextSearchMatch, groupOf map[string]graph.TextSearchGroup) ([]graph.TextSearchGroup, int) {
	groups := []graph.TextSearchGroup{}
	for _, match := range matches {
		group := groupOf[match.ID]
		at := slices.IndexFunc(groups, func(listed graph.TextSearchGroup) bool { return listed.CommunityID == group.CommunityID })
		if at < 0 {
			groups = append(groups, group)
			at = len(groups) - 1
		}
		groups[at].Matches++
	}
	unreadable := 0
	for _, group := range groups {
		if group.RecordState == graph.RecordUnreadable {
			unreadable++
		}
	}
	return groups, unreadable
}

// textSearchAnswer is the answer text: the word index's count and the number of groups holding the
// reply's matches; in a merged list, how many matches were found by meaning alone, those with a
// vector rank and no word rank; and how many are shown when the answer is cut.
func textSearchAnswer(data graph.TextSearchData) string {
	extra := 0
	for _, match := range data.Matches {
		if match.WordRank == 0 && match.VectorRank > 0 {
			extra++
		}
	}
	answer := fmt.Sprintf("Found %d matching entities in %d groups.", data.Matched, len(data.Groups))
	if extra > 0 {
		answer = fmt.Sprintf("Found %d matching entities in %d groups, and %d more by meaning.", data.Matched, len(data.Groups), extra)
	}
	if data.Truncated {
		answer += fmt.Sprintf(" Showing the first %d.", len(data.Matches))
	}
	return answer
}

// fitTextSearch encodes data, with its groups and answer, keeping the most of its best matches whose
// encoding is at most limit bytes; leaving any out sets Truncated, and the groups are those of the
// matches kept, so their counts sum to the matches sent. groupOf is each match's group, from
// matchGroups. It returns the encoding and the data it encodes, and refuses when even no matches do
// not fit.
func fitTextSearch(data graph.TextSearchData, groupOf map[string]graph.TextSearchGroup, limit int, encode func(graph.TextSearchData) ([]byte, error)) ([]byte, graph.TextSearchData, error) {
	keep := func(n int) graph.TextSearchData {
		kept := data
		kept.Matches = data.Matches[:n]
		kept.Groups, kept.UnreadableGroups = textGroups(kept.Matches, groupOf)
		kept.Truncated = data.Truncated || n < len(data.Matches)
		kept.Answer = textSearchAnswer(kept)
		return kept
	}
	encoded, n, err := fitFirst(len(data.Matches), limit, func(n int) ([]byte, error) { return encode(keep(n)) })
	if err != nil {
		return nil, graph.TextSearchData{}, err
	}
	return encoded, keep(n), nil
}

// labelOrder is the label set DiscoverLabelPredicates returns, most salient first: the lowest
// priority number, ties by predicate name.
func labelOrder(priorities map[string]int) []string {
	order := make([]string, 0, len(priorities))
	for predicate := range priorities {
		order = append(order, predicate)
	}
	slices.SortFunc(order, func(a, b string) int {
		return cmp.Or(cmp.Compare(priorities[a], priorities[b]), cmp.Compare(a, b))
	})
	return order
}
