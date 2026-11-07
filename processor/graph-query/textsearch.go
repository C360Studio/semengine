package graphquery

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/graphview"
)

// maxTextGroups is the most groups the text tier reads for one question (ruling 2; design D2).
const maxTextGroups = 5

// A group record's score weights (design D2): the pin's ScoreWeightSummary and ScoreWeightKeyword
// (pin:graphrag.go:2416-2470).
const (
	summaryTermScore = 2.0
	keywordTermScore = 1.5
)

// textResult is what the text tier read for one question (design D2, D3, D5), from which task 3.3
// builds the reply. Its records and bytes are the view's and the pages', read only, and live as long
// as the request.
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
// entities as localSearch does (design D2, D3, D5). Its refusals, in order: more than
// graph.MaxSearchTerms terms is invalid_request; a view it cannot read or a level with no record,
// readable or not, is transient index_not_ready; every record unreadable is transient internal, one
// error; a min_revision above the mark is transient revision_not_reached naming it.
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
		matches, scanned, err := rankEntities(page.rows, terms)
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

// levelRecords lists view's records under levelPrefix, sorted by key, and counts the unreadable ones,
// in one read (design D2, D8). No view, a view catching up, lost or stopped, and a level holding no
// record, readable or not, are transient index_not_ready, counting no error.
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
// key (design D2). A record scoring 0 is never read: the pin read the first five whatever they scored
// (inventory P1).
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

// groupScore is the pin's group score (pin:graphrag.go:2416-2470) over terms, which searchTerms has
// lowercased and made distinct: summaryTermScore for each term contained in the record's statistical
// summary, and keywordTermScore for each keyword and term where one contains the other, lowercased.
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
