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

// maxTextGroups is the most groups the text tier reads for one question.
const maxTextGroups = 5

// A group record's score weights: a summary match counts more than a keyword match.
const (
	summaryTermScore = 2.0
	keywordTermScore = 1.5
)

// textPastUnreadable is the Warn an answer past unreadable community records logs, and
// textPastUnreadableReason why answering is safe.
const (
	textPastUnreadable       = "text search answered past unreadable community records"
	textPastUnreadableReason = "community records are independent, and the answer reads only the readable ones"
)

// textResult is what the text tier read for one question, from which globalText builds the reply.
// Its records and bytes are the view's and the pages', read only, and live as long as the request.
type textResult struct {
	groups     []clustering.Community // the records read, best first
	matches    []rankedEntity         // their pages' matches, best first, ties by ID
	scanned    int                    // the distinct entities the pages held
	pageCut    bool                   // a group read had more than one page
	unreadable int                    // the records at the level that could not be read
	revision   uint64                 // the highest entity_revision among the level's readable records
}

// searchText reads the community records at the request's level once, refuses before any page read
// what it cannot answer, then reads one prefix page of each group textGroups chooses and ranks its
// entities by the text score. Its refusals, in order: more than graph.MaxSearchTerms terms is
// invalid_request; a view it cannot read or a level with no record, readable or not, is transient
// index_not_ready; every record unreadable is transient internal, one error; a min_revision above
// the mark is transient revision_not_reached naming it.
func (c *Component) searchText(ctx context.Context, prefixSubject string, request graph.GlobalSearchRequest, levelPrefix string) (textResult, error) {
	terms, err := checkedSearchTerms(request.Query)
	if err != nil {
		return textResult{}, invalidRequest(err)
	}
	records, unreadable, err := levelRecords(c.communities.Load(), levelPrefix)
	if err != nil {
		return textResult{}, err
	}
	if len(records) == 0 {
		// An unreadable record's revision is unknown, so there is no mark to answer at.
		return textResult{}, c.unanswered(fmt.Errorf("all %d community records under %q are unreadable", unreadable, levelPrefix))
	}
	result := textResult{unreadable: unreadable}
	for _, record := range records {
		result.revision = max(result.revision, record.Value.EntityRevision)
	}
	if request.MinRevision > result.revision {
		return textResult{}, errs.ClassifiedCodeDetail(errs.ErrorTransient, graph.ErrorCodeRevisionNotReached,
			map[string]any{"indexed_revision": result.revision},
			fmt.Errorf("min_revision %d is above the highest entity_revision %d under %q", request.MinRevision, result.revision, levelPrefix))
	}
	result.groups = textGroups(records, terms)
	for _, group := range result.groups {
		page, err := c.readCountPage(ctx, prefixSubject, group.ID, "")
		if err != nil {
			return textResult{}, err
		}
		matches, scanned, err := rankEntities(page.rows, terms, c.labelPredicates)
		if err != nil {
			return textResult{}, c.brokenReply(prefixSubject, err)
		}
		// The groups at one level are disjoint, so the merged list is their union.
		result.matches = append(result.matches, matches...)
		result.scanned += scanned
		result.pageCut = result.pageCut || page.nextCursor != ""
	}
	slices.SortFunc(result.matches, compareRanked)
	return result, nil
}

// levelRecords lists view's records under levelPrefix, sorted by key, and counts the unreadable
// ones, in one read. No view, a view catching up, lost or stopped, and a level holding no record,
// readable or not, are transient index_not_ready, counting no error.
func levelRecords(view *communityView, levelPrefix string) ([]graphview.KeyedEntry[clustering.Community], int, error) {
	if view == nil {
		return nil, 0, communityNotReady(errors.New("no view of " + graph.BucketCommunityIndex))
	}
	records, unreadable, err := view.ListCounted(levelPrefix)
	if err != nil {
		return nil, 0, communityNotReady(err)
	}
	if len(records) == 0 && unreadable == 0 {
		return nil, 0, communityNotReady(fmt.Errorf("%s holds no record under %q", graph.BucketCommunityIndex, levelPrefix))
	}
	return records, unreadable, nil
}

// textGroups returns up to maxTextGroups of records scoring above 0 over terms, best first, ties by
// key. A record scoring 0 is never read, so the groups read are the ones the question matches, not
// the first five by key.
func textGroups(records []graphview.KeyedEntry[clustering.Community], terms []string) []clustering.Community {
	type scored struct {
		key    string
		score  float64
		record clustering.Community
	}
	var matched []scored
	for _, record := range records {
		if score := groupScore(record.Value, terms); score > 0 {
			matched = append(matched, scored{key: record.Key, score: score, record: record.Value})
		}
	}
	slices.SortFunc(matched, func(a, b scored) int {
		return cmp.Or(cmp.Compare(b.score, a.score), cmp.Compare(a.key, b.key))
	})
	groups := make([]clustering.Community, 0, min(len(matched), maxTextGroups))
	for _, m := range matched[:min(len(matched), maxTextGroups)] {
		groups = append(groups, m.record)
	}
	return groups
}

// groupScore is the group score over terms, which searchTerms has lowercased and made distinct:
// summaryTermScore for each term contained in the record's statistical summary, and
// keywordTermScore for each keyword and term where one contains the other, lowercased.
func groupScore(record clustering.Community, terms []string) float64 {
	summary := strings.ToLower(record.StatisticalSummary)
	score := 0.0
	for _, term := range terms {
		if strings.Contains(summary, term) {
			score += summaryTermScore
		}
		for _, keyword := range record.Keywords {
			keyword = strings.ToLower(keyword)
			if strings.Contains(keyword, term) || strings.Contains(term, keyword) {
				score += keywordTermScore
			}
		}
	}
	return score
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

// textSearchData is the reply's result for what the text tier read at level, with the hints the
// question carried that it did not honour, before the fit and the answer. Groups, matches and hints
// are never null: an empty list says none.
func textSearchData(result textResult, level int, unhonoured []string) graph.TextSearchData {
	groups := make([]graph.TextSearchGroup, 0, len(result.groups))
	for _, group := range result.groups {
		groups = append(groups, graph.TextSearchGroup{
			CommunityID:      group.ID,
			MemberCount:      group.MemberCount,
			CommunitySummary: group.StatisticalSummary,
			Keywords:         group.Keywords,
		})
	}
	matches := make([]graph.TextSearchMatch, 0, len(result.matches))
	for _, match := range result.matches {
		matches = append(matches, graph.TextSearchMatch{ID: match.id, Label: match.label, Score: match.score, Entity: match.raw})
	}
	return graph.TextSearchData{
		Level:             level,
		Groups:            groups,
		Matches:           matches,
		EntitiesScanned:   result.scanned,
		EntitiesTruncated: result.pageCut,
		UnreadableGroups:  result.unreadable,
		UnhonouredHints:   append([]string{}, unhonoured...),
	}
}

// textSearchAnswer is the answer text, {n} being the matches in the reply: the counts, or that no
// group matched; that it is a sample when it is; each group's summary, or its member count and
// first five keywords when it has none; the hints not honoured; the records that could not be read.
func textSearchAnswer(data graph.TextSearchData) string {
	var sentences []string
	if len(data.Groups) == 0 {
		sentences = append(sentences, fmt.Sprintf("No group at level %d matched the question.", data.Level))
	} else {
		sentences = append(sentences, fmt.Sprintf("Found %d matching entities among %d read from %d groups at level %d.",
			len(data.Matches), data.EntitiesScanned, len(data.Groups), data.Level))
	}
	if data.EntitiesTruncated {
		sentences = append(sentences, "This is a sample: a group had more than one page, or matches were left out to fit.")
	}
	for _, group := range data.Groups {
		if group.CommunitySummary != "" {
			sentences = append(sentences, group.CommunityID+": "+group.CommunitySummary)
			continue
		}
		sentence := fmt.Sprintf("%s: It has %d members.", group.CommunityID, group.MemberCount)
		if len(group.Keywords) > 0 {
			sentence += " Key themes: " + strings.Join(group.Keywords[:min(len(group.Keywords), 5)], ", ") + "."
		}
		sentences = append(sentences, sentence)
	}
	if len(data.UnhonouredHints) > 0 {
		sentences = append(sentences, "Not honoured: "+strings.Join(data.UnhonouredHints, ", ")+".")
	}
	if data.UnreadableGroups > 0 {
		sentences = append(sentences, fmt.Sprintf("%d group records at this level could not be read.", data.UnreadableGroups))
	}
	return strings.Join(sentences, " ")
}

// fitTextSearch encodes data, with its answer, keeping the most of its best matches whose encoding
// is at most limit bytes; leaving any out sets EntitiesTruncated, as fitLocalSearch does. It also
// reports whether the encoded answer is truncated, and refuses when even no matches do not fit.
func fitTextSearch(data graph.TextSearchData, limit int, encode func(graph.TextSearchData) ([]byte, error)) ([]byte, bool, error) {
	all := data.Matches
	encoded, cut, err := fitFirst(len(all), limit, func(n int) ([]byte, error) {
		kept := data
		kept.Matches = all[:n]
		kept.EntitiesTruncated = data.EntitiesTruncated || n < len(all)
		kept.Answer = textSearchAnswer(kept)
		return encode(kept)
	})
	return encoded, data.EntitiesTruncated || cut, err
}

// minTextScore is the score an entity needs to match in the text tier: a constant, since
// min_text_relevance stays refused.
const minTextScore = 0.3

// checkedSearchTerms is searchTerms, refusing a query that holds no term or more than
// graph.MaxSearchTerms.
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

// rankEntities returns the page's entities scoring at least minTextScore, best first, ties by
// entity ID, each labelled from labels (nil for no label), and the number of distinct entities
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
		if score := entity.score(terms); score >= minTextScore {
			matches = append(matches, rankedEntity{id: entity.ID, label: entity.label(labels), score: score, raw: raw})
		}
	}
	slices.SortFunc(matches, compareRanked)
	return matches, len(seen), nil
}
