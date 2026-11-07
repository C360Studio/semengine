//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// textGroup is a level-0 group the text tier tests write records for: no entity of the graph stack's
// lives under it, so its page is empty.
const textGroup = "c360.test.text."

// putRecord writes value at key of COMMUNITY_INDEX, as its owner, graph-clustering, would; a value
// that is not key's record is a poisoned key.
func putRecord(t *testing.T, kv jetstream.KeyValue, key, value string) {
	t.Helper()
	if _, err := kv.Put(t.Context(), key, []byte(value)); err != nil {
		t.Fatalf("put %s: %v", key, err)
	}
}

// literalRecord is a level-0 record of textGroup+id at entity_revision revision, with summary and
// keywords as given.
func literalRecord(t *testing.T, id string, revision uint64, summary string, keywords ...string) (string, string) {
	t.Helper()
	encoded, err := json.Marshal(keywords)
	if err != nil {
		t.Fatal(err)
	}
	return "0." + textGroup + id, fmt.Sprintf(`{"id":%q,"level":0,"member_count":1,"entity_revision":%d,"statistical_summary":%q,"keywords":%s}`,
		textGroup+id, revision, summary, encoded)
}

// askText answers request as globalSearch's text tier does, through the request decode globalSearch
// runs, and returns what the tier read. Task 3.3 makes it the globalSearch reply.
func askText(t *testing.T, c *Component, request string) (textResult, error) {
	t.Helper()
	decoded, levelPrefix, err := decodeGlobalSearchRequest([]byte(request))
	if err != nil {
		t.Fatalf("decode %s: %v", request, err)
	}
	return c.searchText(t.Context(), verbRow(t, graph.ResponderGraphIngest, "prefix").Subject, decoded, levelPrefix)
}

// awaitTextMark waits until the text tier reads level at mark: a question far ahead of it is refused
// revision_not_reached naming that mark, before any page read and counting no error. The view applies
// COMMUNITY_INDEX's writes in order, so every write before the one that set the mark is applied too.
func awaitTextMark(t *testing.T, client *natsclient.Client, level int, mark uint64) {
	t.Helper()
	request := fmt.Sprintf(`{"query":"no such words","level":%d,"min_revision":%d}`, level, farAhead)
	got, err := probe.Await(t.Context(), func(ctx context.Context) (searchOutcome, error) {
		raw, err := client.RequestClassified(ctx, globalSearchSubject, []byte(request), requestTimeout)
		return searchOutcome{raw, err}, nil
	}, func(o searchOutcome) bool {
		var refusal *errs.ClassifiedError
		return errors.As(o.err, &refusal) && refusal.Code == graph.ErrorCodeRevisionNotReached &&
			refusal.Detail["indexed_revision"] == float64(mark)
	})
	if err != nil {
		t.Fatalf("await level %d at %d: %v (last %v)", level, mark, err, got.err)
	}
}

// prefixesOf is the prefix each request body names, in the order sent.
func prefixesOf(t *testing.T, bodies []string) []string {
	t.Helper()
	prefixes := make([]string, 0, len(bodies))
	for _, body := range bodies {
		var request struct {
			Prefix string `json:"prefix"`
			Limit  int    `json:"limit"`
		}
		if err := json.Unmarshal([]byte(body), &request); err != nil || request.Limit != graph.MaxPrefixQueryLimit {
			t.Fatalf("prefix request %s: %v, want limit %d", body, err, graph.MaxPrefixQueryLimit)
		}
		prefixes = append(prefixes, request.Prefix)
	}
	return prefixes
}

// TestTextTierReadsAtMostFiveGroups: seven groups at level 0 match the question and one does not;
// exactly five prefix requests are made, one per group, for the five best, best first, ties by key,
// each at the maximum limit (ruling 2; spec, "Seven matching groups"). A counting subscriber on the
// prefix verb reads every request.
func TestTextTierReadsAtMostFiveGroups(t *testing.T) {
	stack := startCommunityStack(t)
	// Scores for "pump valve": e 7, c 4, g 3.5, a 3, b 2, f 2, d 1.5, h 0; b's key is before f's.
	for i, record := range [][]string{
		{"a", "", "pump", "valve"},
		{"b", "pump"},
		{"c", "pump valve"},
		{"d", "", "pumps"},
		{"e", "pump valve", "pump", "valve"},
		{"f", "valve"},
		{"g", "pump", "valve"},
		{"h", "gear", "gear"},
	} {
		key, value := literalRecord(t, record[0], uint64(i+1), record[1], record[2:]...)
		putRecord(t, stack.records, key, value)
	}
	awaitTextMark(t, stack.client, 0, 8)
	prefixRequests := spyOnPrefixVerb(t, stack.client)
	want := []string{textGroup + "e", textGroup + "c", textGroup + "g", textGroup + "a", textGroup + "b"}

	// The reply is task 3.3's; the requests are made before it.
	_, _ = stack.askGlobal(t, `{"query":"pump valve"}`)
	if got := prefixesOf(t, prefixRequests()); !slices.Equal(got, want) {
		t.Errorf("prefix requests = %q, want %q", got, want)
	}
	result, err := askText(t, stack.component, `{"query":"pump valve"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := groupIDs(result.groups); !slices.Equal(got, want) {
		t.Errorf("groups = %q, want %q", got, want)
	}
	prefixRequests()
}

// TestTextTierSkipsUnmatchedGroups: P1's seven records, none matching "pump": no group, no match, no
// prefix request, and no error (spec, "No group matches").
func TestTextTierSkipsUnmatchedGroups(t *testing.T) {
	stack := startCommunityStack(t)
	for i := range 7 {
		key, value := literalRecord(t, fmt.Sprintf("g%d", i), uint64(i+1), "sensors and valves", "valve")
		putRecord(t, stack.records, key, value)
	}
	awaitTextMark(t, stack.client, 0, 7)
	prefixRequests := spyOnPrefixVerb(t, stack.client)
	result, err := askText(t, stack.component, `{"query":"pump"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.groups) != 0 || len(result.matches) != 0 || result.scanned != 0 || result.revision != 7 {
		t.Errorf("result = %+v, want no groups, no matches, nothing scanned, revision 7", result)
	}
	if got := prefixRequests(); len(got) != 0 {
		t.Errorf("prefix requests = %q, want none", got)
	}
	if got := stack.component.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0", got)
	}
}

// TestTextTierRefusesAnEmptyLevel: with records at level 0 only, a level-1 question is transient
// index_not_ready, before any page read and counting no error; after a level-1 write the same request
// answers (spec, "An empty level"; ruling 3).
func TestTextTierRefusesAnEmptyLevel(t *testing.T) {
	stack := startCommunityStack(t)
	key, value := literalRecord(t, "a", 3, "pump station", "pump")
	putRecord(t, stack.records, key, value)
	awaitTextMark(t, stack.client, 0, 3)
	prefixRequests := spyOnPrefixVerb(t, stack.client)

	const request = `{"query":"pump station","level":1}`
	raw, err := stack.askGlobal(t, request)
	requireRefusal(t, raw, err, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
	if want := `community index not ready: COMMUNITY_INDEX holds no record under "1."`; err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
	if got := prefixRequests(); len(got) != 0 {
		t.Errorf("prefix requests = %q, want none", got)
	}
	if got := stack.component.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0", got)
	}

	putRecord(t, stack.records, "1.c360.test", `{"id":"c360.test","level":1,"member_count":1,"entity_revision":4,"keywords":["pump"]}`)
	awaitTextMark(t, stack.client, 1, 4)
	result, err := askText(t, stack.component, request)
	if err != nil {
		t.Fatalf("after a level-1 write: %v", err)
	}
	if got := groupIDs(result.groups); !slices.Equal(got, []string{"c360.test"}) || result.revision != 4 {
		t.Errorf("groups %q at revision %d, want [c360.test] at 4", got, result.revision)
	}
}

// TestTextTierReadsTheAskedLevel: one write in each of two places gives groups at every level, and a
// question at each level reads only that level's records, through a real maintainer (spec, "The text
// tier reads at most five matching groups, one page each"; ADR-085).
func TestTextTierReadsTheAskedLevel(t *testing.T) {
	stack := startCommunityStack(t)
	const elsewhere = "c360.test.other.site.drone.001"
	stack.write(t, drone1, [2]string{"dc.terms.title", "zeta"})
	last := stack.write(t, elsewhere, [2]string{"dc.terms.title", "beta"})
	tick(t, stack.maintainer)
	rows := pageRows(t, stack.client, systemGroup)

	for level, group := range []string{systemGroup, "c360.test.query", "c360.test"} {
		awaitTextMark(t, stack.client, level, last)
		result, err := askText(t, stack.component, fmt.Sprintf(`{"query":"zeta","level":%d}`, level))
		if err != nil {
			t.Fatalf("level %d: %v", level, err)
		}
		if len(result.groups) != 1 || result.groups[0].ID != group || result.groups[0].Level != level {
			t.Errorf("level %d: groups %+v, want only %s at level %d", level, result.groups, group, level)
		}
		if got := matchesOf(result); !slices.Equal(got, []string{fmt.Sprintf("%s 0.5 %s", drone1, rows[drone1])}) {
			t.Errorf("level %d: matches %q, want %s alone", level, got, drone1)
		}
	}
}

// matchesOf is each match as "id score bytes", in order.
func matchesOf(result textResult) []string {
	out := make([]string, 0, len(result.matches))
	for _, match := range result.matches {
		out = append(out, fmt.Sprintf("%s %v %s", match.id, match.score, match.raw))
	}
	return out
}

// TestTextTierMatchesEqualAFilter: the matches of the groups read are the entities an independent
// filter, written here, keeps from their pages, as graph-ingest sent them, merged across groups by
// score then ID, and a group not read adds none; no page had more, so the result is not cut (spec,
// "Groups that fit a page"; design D3).
func TestTextTierMatchesEqualAFilter(t *testing.T) {
	stack := startCommunityStack(t)
	title := func(text string) [2]string { return [2]string{"dc.terms.title", text} }
	const (
		zeta  = "c360.test.query.system.zeta.001" // its type is the term: 1.0
		other = "c360.test.query.other.drone.001"
		third = "c360.test.query.third.drone.001"
	)
	stack.write(t, drone1, title("zeta survey"))
	stack.write(t, drone2, title("zetas"))
	stack.write(t, drone3, title("beta"))
	stack.write(t, zeta)
	stack.write(t, other, title("zeta"))
	last := stack.write(t, third, title("gamma"))
	tick(t, stack.maintainer)
	awaitTextMark(t, stack.client, 0, last)
	rows := pageRows(t, stack.client, systemGroup)
	for id, row := range pageRows(t, stack.client, "c360.test.query.other") {
		rows[id] = row
	}
	prefixRequests := spyOnPrefixVerb(t, stack.client)

	result, err := askText(t, stack.component, `{"query":"zeta"}`)
	if err != nil {
		t.Fatal(err)
	}
	// The filter: the type zeta scores 1.0, a title holding zeta 0.5 (TestLocalSearchScoresAsThePin);
	// drone3's beta and the third group, whose record holds no zeta, are not read for it.
	want := []string{
		fmt.Sprintf("%s 1 %s", zeta, rows[zeta]),
		fmt.Sprintf("%s 0.5 %s", other, rows[other]),
		fmt.Sprintf("%s 0.5 %s", drone1, rows[drone1]),
		fmt.Sprintf("%s 0.5 %s", drone2, rows[drone2]),
	}
	if got := matchesOf(result); !slices.Equal(got, want) {
		t.Errorf("matches =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if result.scanned != 5 || result.pageCut {
		t.Errorf("scanned %d, cut %v; want 5, false", result.scanned, result.pageCut)
	}
	// The system group's record holds zeta and zetas, so it is read first.
	if got, want := prefixesOf(t, prefixRequests()), []string{systemGroup, "c360.test.query.other"}; !slices.Equal(got, want) {
		t.Errorf("prefix requests = %q, want %q", got, want)
	}
}

// TestTextTierMarksACutPage: a group past one page is read one page, and the result says it is cut
// (spec, "Text matches are the pages' entities as stored, best first").
func TestTextTierMarksACutPage(t *testing.T) {
	stack := startCommunityStack(t)
	const bulk = "c360.test.query.bulk"
	stack.write(t, bulk+".drone.0000", [2]string{"dc.terms.title", "zeta"})
	var last uint64
	for i := 1; i <= graph.MaxPrefixQueryLimit; i++ {
		last = stack.write(t, fmt.Sprintf("%s.drone.%04d", bulk, i))
	}
	tick(t, stack.maintainer)
	awaitTextMark(t, stack.client, 0, last)
	result, err := askText(t, stack.component, `{"query":"zeta"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !result.pageCut || result.scanned != graph.MaxPrefixQueryLimit || len(result.matches) != 1 || result.matches[0].id != bulk+".drone.0000" {
		t.Errorf("cut %v, scanned %d, matches %q; want true, %d, the one zeta", result.pageCut, result.scanned, matchesOf(result), graph.MaxPrefixQueryLimit)
	}
}

// TestTextTierRevision: the text tier's revision is the highest entity_revision among the level's
// records; a min_revision above it is transient revision_not_reached naming it, before any page read
// and counting no error, and one at it answers (spec, "A revision not reached"; design D5).
func TestTextTierRevision(t *testing.T) {
	stack := startCommunityStack(t)
	stack.write(t, drone1, [2]string{"dc.terms.title", "zeta"})
	last := stack.write(t, "c360.test.query.other.drone.001", [2]string{"dc.terms.title", "zeta"})
	tick(t, stack.maintainer)
	awaitTextMark(t, stack.client, 0, last)
	prefixRequests := spyOnPrefixVerb(t, stack.client)

	raw, err := stack.askGlobal(t, fmt.Sprintf(`{"query":"zeta survey","min_revision":%d}`, last+1))
	refusal := requireRefusal(t, raw, err, errs.ErrorTransient, graph.ErrorCodeRevisionNotReached)
	if got := refusal.Detail["indexed_revision"]; got != float64(last) {
		t.Errorf("indexed_revision = %v, want %d", got, last)
	}
	if got := prefixRequests(); len(got) != 0 {
		t.Errorf("prefix requests = %q, want none", got)
	}
	if got := stack.component.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0", got)
	}
	result, err := askText(t, stack.component, fmt.Sprintf(`{"query":"zeta","min_revision":%d}`, last))
	if err != nil || result.revision != last || len(result.groups) != 2 {
		t.Errorf("at the mark: %+v, %v; want two groups at revision %d", result, err, last)
	}
}

// TestTextTierRevisionIgnoresUnreadable: an unreadable record that holds a higher entity_revision
// never counts toward the mark: the mark is the readable record's, a min_revision above it is refused
// before any page read, and one at it answers with the unreadable record counted (design D5).
func TestTextTierRevisionIgnoresUnreadable(t *testing.T) {
	stack := startCommunityStack(t)
	// Key p holds q's record, at 99: DecodeCommunity refuses it.
	putRecord(t, stack.records, "0."+textGroup+"p", fmt.Sprintf(`{"id":%q,"level":0,"member_count":1,"entity_revision":99}`, textGroup+"q"))
	key, value := literalRecord(t, "a", 5, "pump station", "pump")
	putRecord(t, stack.records, key, value)
	awaitTextMark(t, stack.client, 0, 5) // after the unreadable write, which comes first
	prefixRequests := spyOnPrefixVerb(t, stack.client)

	raw, err := stack.askGlobal(t, `{"query":"pump station","min_revision":6}`)
	refusal := requireRefusal(t, raw, err, errs.ErrorTransient, graph.ErrorCodeRevisionNotReached)
	if got := refusal.Detail["indexed_revision"]; got != float64(5) {
		t.Errorf("indexed_revision = %v, want 5", got)
	}
	if got := prefixRequests(); len(got) != 0 {
		t.Errorf("prefix requests = %q, want none", got)
	}
	result, err := askText(t, stack.component, `{"query":"pump station","min_revision":5}`)
	if err != nil || result.revision != 5 || result.unreadable != 1 || len(result.groups) != 1 {
		t.Errorf("at the mark: %+v, %v; want one group, one unreadable, revision 5", result, err)
	}
}

// TestTextTierAllUnreadable: when every record at the level is unreadable, the question is transient
// internal naming the count, one error in Health, before any page read; no 0 stands for the unknown
// revision (spec, "Every record unreadable"; design D5).
func TestTextTierAllUnreadable(t *testing.T) {
	stack := startCommunityStack(t)
	putRecord(t, stack.records, "1.c360.test.x", `{}`)
	putRecord(t, stack.records, "1.c360.test.y", `not json`)
	key, value := literalRecord(t, "a", 5, "pump station", "pump")
	putRecord(t, stack.records, key, value)
	awaitTextMark(t, stack.client, 0, 5) // after both unreadable writes
	prefixRequests := spyOnPrefixVerb(t, stack.client)

	errorsBefore := stack.component.Health().ErrorCount
	raw, err := stack.askGlobal(t, `{"query":"pump station","level":1}`)
	requireRefusal(t, raw, err, errs.ErrorTransient, graph.ErrorCodeInternal)
	if want := `internal error: all 2 community records under "1." are unreadable`; err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
	if got := stack.component.Health().ErrorCount; got != errorsBefore+1 {
		t.Errorf("ErrorCount = %d, want %d", got, errorsBefore+1)
	}
	if got := prefixRequests(); len(got) != 0 {
		t.Errorf("prefix requests = %q, want none", got)
	}
}
