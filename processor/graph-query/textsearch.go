package graphquery

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	"github.com/c360studio/semengine/graph"
)

// textPastUnreadable is the Warn an answer past unreadable community records logs, and
// textPastUnreadableReason why answering is safe.
const (
	textPastUnreadable       = "global search answered past unreadable community records"
	textPastUnreadableReason = "a record only describes its group; the matches come from the word index"
)

// globalText is global search's answer: one unscoped word search, passing query, limit and
// min_revision unchanged, then one batch read of its hits, grouped by their community at the
// request's level, then the route's reference verified, with the hints listed beside the matches. A
// refusal from either search verb reaches the caller as it arrived, before any hint is read; a hit
// whose ID names no group is the word index's broken reply. The reply, fit to max_payload, is under
// the word search's revision. verb is the verb asked, which the truncation counter and the Warn
// name.
//
// A group's record is attached when the community view holds one, and never decides the answer:
// record_state says when it is missing. An answer past unreadable records is a declared degrade: it
// sets unreadable_groups, counts one error and logs one Warn, both only once the reply is encoded,
// so a request that fails to fit counts the fit's one error alone.
func (c *Component) globalText(ctx context.Context, verb string, subjects globalSearchSubjects, request graph.GlobalSearchRequest, route globalRoute) ([]byte, error) {
	search, err := c.wordSearch(ctx, subjects.lexical, graph.LexicalSearchRequest{
		Query: request.Query, Limit: request.Limit, MinRevisionField: request.MinRevisionField,
	})
	if err != nil {
		return nil, err
	}
	hits := search.Data.Results
	hitGroups := make(map[string]searchGroup, len(hits))
	for _, hit := range hits {
		group, err := groupAt(hit.EntityID, request.Level)
		if err != nil {
			return nil, c.brokenReply(subjects.lexical, err)
		}
		hitGroups[hit.EntityID] = group
	}
	matches, missing, err := c.readMatches(ctx, subjects.batch, hits, c.labelPredicates)
	if err != nil {
		return nil, err
	}
	data := graph.TextSearchData{
		Level:            request.Level,
		Matches:          matches,
		Matched:          search.Data.Matched,
		Excluded:         search.Data.Excluded,
		Missing:          missing,
		BodiesNotIndexed: search.Data.BodiesNotIndexed,
		Truncated:        search.Data.Matched > len(hits) || missing > 0,
	}
	answer := graph.GlobalSearchData{Hints: []graph.GlobalSearchHint{}}
	if verified, ok := c.verify(ctx, subjects, route, request.MinRevision); ok {
		answer.Hints = append(answer.Hints, verified.hint)
		answer.Entity, answer.Path = verified.entity, verified.path
	}
	answer.Hints = append(answer.Hints, route.hints...)
	limit, err := c.natsClient.MaxPayload()
	if err != nil {
		return nil, c.unanswered(err)
	}
	groupOf := matchGroups(c.communities.Load(), matches, hitGroups)
	encoded, sent, err := fitTextSearch(data, groupOf, int(limit), func(data graph.TextSearchData) ([]byte, error) {
		answer.Text = &data
		response, err := graph.NewQueryResponse(graph.ResponderGraphQuery, search.IndexedRevision, answer)
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
// reply's matches; and how many are shown when the answer is cut.
func textSearchAnswer(data graph.TextSearchData) string {
	answer := fmt.Sprintf("Found %d matching entities in %d groups.", data.Matched, len(data.Groups))
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
