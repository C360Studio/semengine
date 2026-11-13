//go:build integration

package graphquery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
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

// textAnswer is a text strategy reply as the wire carries it: the envelope's revision and the data.
type textAnswer struct {
	revision uint64
	data     graph.TextSearchData
}

// askText asks globalSearch request and reads its text answer strictly; a refusal returns as it came.
func askText(t *testing.T, client *natsclient.Client, request string) (textAnswer, error) {
	t.Helper()
	raw, err := client.RequestClassified(t.Context(), globalSearchSubject, []byte(request), requestTimeout)
	if err != nil {
		return textAnswer{}, err
	}
	return decodeTextAnswer(t, raw), nil
}

// decodeTextAnswer decodes a globalSearch success strictly, the envelope and then its data, and fails
// unless the strategy is text and text is its one result.
func decodeTextAnswer(t *testing.T, raw []byte) textAnswer {
	t.Helper()
	envelope := decodeEnvelope(t, raw)
	var data graph.GlobalSearchData
	decoder := json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil {
		t.Fatalf("decode data %s: %v", envelope.Data, err)
	}
	if data.Strategy != strategyText || data.Text == nil || data.Entity != nil || data.Path != nil || data.Temporal != nil || data.Spatial != nil {
		t.Fatalf("data = %s, want strategy text and text alone", envelope.Data)
	}
	return textAnswer{revision: envelope.IndexedRevision, data: *data.Text}
}

// textGroupIDs is each group's community_id, in order.
func textGroupIDs(groups []graph.TextSearchGroup) []string {
	ids := make([]string, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.CommunityID)
	}
	return ids
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
// each at the maximum limit (spec, "Seven matching groups"). A counting subscriber on the prefix
// verb reads every request.
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

	answer, err := askText(t, stack.client, `{"query":"pump valve"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := prefixesOf(t, prefixRequests()); !slices.Equal(got, want) {
		t.Errorf("prefix requests = %q, want %q", got, want)
	}
	if got := textGroupIDs(answer.data.Groups); !slices.Equal(got, want) {
		t.Errorf("groups = %q, want %q", got, want)
	}
}

// TestTextTierSkipsUnmatchedGroups: the seven records of TestTextTierScore's "no record scores"
// case, none matching "pump": no group, no match, no prefix request, and no error (spec, "No group
// matches").
func TestTextTierSkipsUnmatchedGroups(t *testing.T) {
	stack := startCommunityStack(t)
	for i := range 7 {
		key, value := literalRecord(t, fmt.Sprintf("g%d", i), uint64(i+1), "sensors and valves", "valve")
		putRecord(t, stack.records, key, value)
	}
	awaitTextMark(t, stack.client, 0, 7)
	prefixRequests := spyOnPrefixVerb(t, stack.client)
	answer, err := askText(t, stack.client, `{"query":"pump"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.data.Groups) != 0 || len(answer.data.Matches) != 0 || answer.data.EntitiesScanned != 0 || answer.revision != 7 {
		t.Errorf("answer = %+v, want no groups, no matches, nothing scanned, revision 7", answer)
	}
	if want := "No group at level 0 matched the question."; answer.data.Answer != want {
		t.Errorf("answer text = %q, want %q", answer.data.Answer, want)
	}
	if got := prefixRequests(); len(got) != 0 {
		t.Errorf("prefix requests = %q, want none", got)
	}
	if got := stack.component.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0", got)
	}
}

// TestTextTierRefusesAnEmptyLevel: with records at level 0 only, a level-1 question is transient
// index_not_ready, before any page read and counting no error; after a level-1 write the same
// request answers (spec, "An empty level").
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
	answer, err := askText(t, stack.client, request)
	if err != nil {
		t.Fatalf("after a level-1 write: %v", err)
	}
	if got := textGroupIDs(answer.data.Groups); !slices.Equal(got, []string{"c360.test"}) || answer.revision != 4 {
		t.Errorf("groups %q at revision %d, want [c360.test] at 4", got, answer.revision)
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
		answer, err := askText(t, stack.client, fmt.Sprintf(`{"query":"zeta","level":%d}`, level))
		if err != nil {
			t.Fatalf("level %d: %v", level, err)
		}
		if got := textGroupIDs(answer.data.Groups); !slices.Equal(got, []string{group}) || answer.data.Level != level {
			t.Errorf("level %d: groups %q at level %d, want only %s", level, got, answer.data.Level, group)
		}
		if got := matchesOf(answer.data); !slices.Equal(got, []string{fmt.Sprintf("%s 0.5 %s", drone1, rows[drone1])}) {
			t.Errorf("level %d: matches %q, want %s alone", level, got, drone1)
		}
	}
}

// matchesOf is each match as "id score bytes", in order.
func matchesOf(data graph.TextSearchData) []string {
	out := make([]string, 0, len(data.Matches))
	for _, match := range data.Matches {
		out = append(out, fmt.Sprintf("%s %v %s", match.ID, match.Score, match.Entity))
	}
	return out
}

// TestTextTierMatchesEqualAFilter: the matches of the groups read are the entities an independent
// filter, written here, keeps from their pages, as graph-ingest sent them, merged across groups by
// score then ID, and a group not read adds none; no page had more, so the result is not cut (spec,
// "Groups that fit a page").
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

	answer, err := askText(t, stack.client, `{"query":"zeta"}`)
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
	if got := matchesOf(answer.data); !slices.Equal(got, want) {
		t.Errorf("matches =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if answer.data.EntitiesScanned != 5 || answer.data.EntitiesTruncated {
		t.Errorf("scanned %d, truncated %v; want 5, false", answer.data.EntitiesScanned, answer.data.EntitiesTruncated)
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
	answer, err := askText(t, stack.client, `{"query":"zeta"}`)
	if err != nil {
		t.Fatal(err)
	}
	data := answer.data
	if !data.EntitiesTruncated || data.EntitiesScanned != graph.MaxPrefixQueryLimit || len(data.Matches) != 1 || data.Matches[0].ID != bulk+".drone.0000" {
		t.Errorf("truncated %v, scanned %d, matches %q; want true, %d, the one zeta", data.EntitiesTruncated, data.EntitiesScanned, matchesOf(data), graph.MaxPrefixQueryLimit)
	}
}

// TestTextTierRevision: the text tier's revision is the highest entity_revision among the level's
// records; a min_revision above it is transient revision_not_reached naming it, before any page
// read and counting no error, and one at it answers (spec, "A revision not reached").
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
	// Two words, so the question is not first read as an alias at that revision.
	answer, err := askText(t, stack.client, fmt.Sprintf(`{"query":"zeta survey","min_revision":%d}`, last))
	if err != nil || answer.revision != last || len(answer.data.Groups) != 2 {
		t.Errorf("at the mark: %+v, %v; want two groups at revision %d", answer, err, last)
	}
}

// TestTextTierRevisionIgnoresUnreadable: an unreadable record that holds a higher entity_revision
// never counts toward the mark: the mark is the readable record's, a min_revision above it is
// refused before any page read, and one at it answers with the unreadable record counted.
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
	answer, err := askText(t, stack.client, `{"query":"pump station","min_revision":5}`)
	if err != nil || answer.revision != 5 || answer.data.UnreadableGroups != 1 || len(answer.data.Groups) != 1 {
		t.Errorf("at the mark: %+v, %v; want one group, one unreadable, revision 5", answer, err)
	}
}

// TestTextTierAllUnreadable: when every record at the level is unreadable, the question is
// transient internal naming the count, one error in Health, before any page read; no 0 stands for
// the unknown revision (spec, "Every record unreadable").
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

// servePages stands in for graph-ingest's prefix verb, answering each prefix with its page in pages,
// and an empty page for any other. It returns the prefixes asked, in order, read after the reply
// they led to: the handler records before it replies.
func servePages(t *testing.T, client *natsclient.Client, pages map[string]string) func() []string {
	t.Helper()
	var mu sync.Mutex
	var asked []string
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "prefix").Subject, func(_ context.Context, data []byte) ([]byte, error) {
		var request graph.PrefixQueryRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		mu.Lock()
		asked = append(asked, request.Prefix)
		mu.Unlock()
		if page, ok := pages[request.Prefix]; ok {
			return []byte(page), nil
		}
		return []byte(`{"entities":null,"total":0}`), nil
	})
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(asked)
	}
}

// The text reply tests' records and pages: group a matches "forklift leak" by its summary and a
// keyword (3.5), group b by a keyword (1.5), group c not at all; c holds the level's highest revision.
const (
	replyRecordA = `{"id":"c360.test.text.a","level":0,"member_count":3,"entity_revision":3,"statistical_summary":"Forklift fleet.","keywords":["forklift","hydraulic"]}`
	replyRecordB = `{"id":"c360.test.text.b","level":0,"member_count":1,"entity_revision":4,"keywords":["leak"]}`
	replyRecordC = `{"id":"c360.test.text.c","level":0,"member_count":1,"entity_revision":5,"statistical_summary":"Gear box.","keywords":["gear"]}`
	// A1 scores (1.0 for its type + 0.5 for leak) / 2, A2 1.0 / 2, B1 1.0 / 2; A3 0, but it is read.
	replyRowA1 = `{"id":"c360.test.text.a.forklift.001","triples":[{"predicate":"dc.terms.title","object":"Leak report"}]}`
	replyRowA2 = `{"id":"c360.test.text.a.forklift.002","triples":[]}`
	replyRowA3 = `{"id":"c360.test.text.a.drone.003","triples":[{"predicate":"dc.terms.title","object":"gear"}]}`
	replyRowB1 = `{"id":"c360.test.text.b.leak.001","triples":[{"predicate":"dc.terms.title","object":"Hose leak"}]}`
)

// startTextReply writes the reply tests' records, starts graph-query, waits until it reads them, and
// serves their pages; it returns the component and the prefixes asked.
func startTextReply(t *testing.T, client *natsclient.Client) (*Component, func() []string) {
	t.Helper()
	kv, _ := communityRecords(t, client)
	putRecord(t, kv, "0.c360.test.text.a", replyRecordA)
	putRecord(t, kv, "0.c360.test.text.b", replyRecordB)
	putRecord(t, kv, "0.c360.test.text.c", replyRecordC)
	c := startComponent(t, client, `{}`)
	awaitTextMark(t, client, 0, 5)
	asked := servePages(t, client, map[string]string{
		"c360.test.text.a": `{"entities":[` + replyRowA1 + `,` + replyRowA2 + `,` + replyRowA3 + `],"total":3}`,
		"c360.test.text.b": `{"entities":[` + replyRowB1 + `],"total":1}`,
	})
	return c, asked
}

// TestTextTierReply: a free-text question's reply, as literal JSON: strategy text and text alone;
// the groups read, best first, with their record fields; the matches best first, ties by ID, each
// with its label (none for an entity with no label predicate), score and bytes as the page sent
// them; the counts and the answer, under the level's highest revision, counting no error (spec, "A
// text answer", "The text reply declares what it could not read").
func TestTextTierReply(t *testing.T) {
	client := fixtureClient(t)
	c, asked := startTextReply(t, client)

	raw, err := client.RequestClassified(t.Context(), globalSearchSubject, []byte(`{"query":"forklift leak"}`), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	envelope := decodeEnvelope(t, raw)
	want := `{"strategy":"text","text":{"level":0,"groups":[` +
		`{"community_id":"c360.test.text.a","member_count":3,"community_summary":"Forklift fleet.","keywords":["forklift","hydraulic"]},` +
		`{"community_id":"c360.test.text.b","member_count":1,"keywords":["leak"]}],` +
		`"matches":[{"id":"c360.test.text.a.forklift.001","label":"Leak report","score":0.75,"entity":` + replyRowA1 + `},` +
		`{"id":"c360.test.text.a.forklift.002","score":0.5,"entity":` + replyRowA2 + `},` +
		`{"id":"c360.test.text.b.leak.001","label":"Hose leak","score":0.5,"entity":` + replyRowB1 + `}],` +
		`"entities_scanned":4,"entities_truncated":false,"unreadable_groups":0,"unhonoured_hints":[],` +
		`"answer":"Found 3 matching entities among 4 read from 2 groups at level 0. c360.test.text.a: Forklift fleet. c360.test.text.b: It has 1 members. Key themes: leak."}}`
	if string(envelope.Data) != want {
		t.Errorf("data =\n%s\nwant\n%s", envelope.Data, want)
	}
	if envelope.IndexedRevision != 5 {
		t.Errorf("indexed_revision = %d, want 5, record c's", envelope.IndexedRevision)
	}
	if got, want := asked(), []string{"c360.test.text.a", "c360.test.text.b"}; !slices.Equal(got, want) {
		t.Errorf("prefixes asked = %q, want %q", got, want)
	}
	if got := c.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0", got)
	}
}

// TestGlobalSearchTextQuestion: a free-text question is answered by the text tier naming no hint,
// and one whose only hint is path words with no start names it, in unhonoured_hints and in the
// answer (spec, "A free-text question", "Trigger words in free text").
func TestGlobalSearchTextQuestion(t *testing.T) {
	client := fixtureClient(t)
	startTextReply(t, client)
	for _, tc := range []struct {
		question string
		hints    []string
		answer   string
	}{
		{"forklift hydraulic leak", []string{}, "Found 3 matching entities among 4 read from 2 groups at level 0. c360.test.text.a: Forklift fleet. c360.test.text.b: It has 1 members. Key themes: leak."},
		{"http connection pool", []string{"path"}, "No group at level 0 matched the question. Not honoured: path."},
		{"top 5 file path handling", []string{"path", "ranking"}, "No group at level 0 matched the question. Not honoured: path, ranking."},
	} {
		answer, err := askText(t, client, fmt.Sprintf(`{"query":%q}`, tc.question))
		if err != nil {
			t.Fatalf("%q: %v", tc.question, err)
		}
		if !slices.Equal(answer.data.UnhonouredHints, tc.hints) || answer.data.UnhonouredHints == nil {
			t.Errorf("%q: unhonoured_hints = %q, want %q", tc.question, answer.data.UnhonouredHints, tc.hints)
		}
		if answer.data.Answer != tc.answer {
			t.Errorf("%q: answer = %q, want %q", tc.question, answer.data.Answer, tc.answer)
		}
	}
}

// TestTextTierFitsMaxPayload: a text reply over the connection's max_payload keeps the most of its
// best matches that fit, sets entities_truncated, says it is a sample and counts one truncated
// globalSearch (spec, "Text matches are the pages' entities as stored, best first").
func TestTextTierFitsMaxPayload(t *testing.T) {
	const limit = 8192
	// Own server: a small max_payload, which the shared server does not have.
	f := natsfixture.New(t, natsfixture.WithMaxPayload(limit))
	if err := f.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	client := natsfixture.Open(t, f.URL(), openClient)
	kv, _ := communityRecords(t, client)
	key, value := literalRecord(t, "a", 1, "", "drone")
	putRecord(t, kv, key, value)
	registry := metric.NewMetricsRegistry()
	startMetered(t, client, registry)
	awaitTextMark(t, client, 0, 1)

	// Rows until the page is just under the limit; every row matches, by its type, at 0.5.
	var rows []string
	page := func(rows []string) string {
		return `{"entities":[` + strings.Join(rows, ",") + `],"total":` + fmt.Sprint(len(rows)) + `}`
	}
	for i := 0; ; i++ {
		row := fmt.Sprintf(`{"id":"c360.test.text.a.drone.%03d","triples":[{"predicate":"acme.note.text","object":%q}]}`, i, strings.Repeat("x", 150))
		if len(page(append(rows, row))) > limit-64 {
			break
		}
		rows = append(rows, row)
	}
	servePages(t, client, map[string]string{"c360.test.text.a": page(rows)})

	raw, err := client.RequestClassified(t.Context(), globalSearchSubject, []byte(`{"query":"drone fleet"}`), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > limit {
		t.Errorf("reply of %d bytes, over max_payload %d", len(raw), limit)
	}
	data := decodeTextAnswer(t, raw).data
	kept := len(data.Matches)
	if !data.EntitiesTruncated || kept == 0 || kept >= len(rows) {
		t.Fatalf("kept %d of %d matches, truncated %v; want some left out and the flag set", kept, len(rows), data.EntitiesTruncated)
	}
	for i, match := range data.Matches {
		if string(match.Entity) != rows[i] { // the rows' IDs ascend, and the scores tie
			t.Fatalf("matches[%d] = %s, want %s", i, match.Entity, rows[i])
		}
	}
	// The page had no continuation, so only the fit cut the answer; the reply returned after the handler counted.
	if got := truncatedCount(t, registry.PrometheusRegistry(), "globalSearch"); got != 1 {
		t.Errorf("globalSearch truncated count = %v, want 1", got)
	}
	want := fmt.Sprintf("Found %d matching entities among %d read from 1 groups at level 0. This is a sample: a group had more than one page, or matches were left out to fit. c360.test.text.a: It has 1 members. Key themes: drone.", kept, len(rows))
	if data.Answer != want {
		t.Errorf("answer = %q, want %q", data.Answer, want)
	}
}

// warnRecord is one Warn the text tier logged past unreadable records, with its attributes as text.
type warnRecord struct {
	level slog.Level
	attrs map[string]string
}

// warnCapture is a slog.Handler keeping the records logged with the text tier's unreadable message.
type warnCapture struct {
	mu      sync.Mutex
	records []warnRecord
}

func (w *warnCapture) Enabled(context.Context, slog.Level) bool { return true }
func (w *warnCapture) WithAttrs([]slog.Attr) slog.Handler       { return w }
func (w *warnCapture) WithGroup(string) slog.Handler            { return w }

func (w *warnCapture) Handle(_ context.Context, r slog.Record) error {
	if r.Message != "text search answered past unreadable community records" {
		return nil
	}
	record := warnRecord{level: r.Level, attrs: map[string]string{}}
	r.Attrs(func(a slog.Attr) bool {
		record.attrs[a.Key] = a.Value.String()
		return true
	})
	w.mu.Lock()
	defer w.mu.Unlock()
	w.records = append(w.records, record)
	return nil
}

func (w *warnCapture) snapshot() []warnRecord {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.records)
}

// TestTextTierPoisonIsDeclared: an answer past an unreadable record at the level says so in
// unreadable_groups, counts one error in Health and logs one Warn naming the verb, the level, the
// count and why answering is safe; after a valid write to that key the answer counts none and logs
// nothing (spec, "A corrupt record"). The handler logs and counts before it replies, so each read
// below is ordered after the reply it follows.
func TestTextTierPoisonIsDeclared(t *testing.T) {
	client := fixtureClient(t)
	kv, _ := communityRecords(t, client)
	putRecord(t, kv, "0."+textGroup+"p", `{}`)
	key, value := literalRecord(t, "a", 5, "Pump station.", "pump")
	putRecord(t, kv, key, value)
	logs := &warnCapture{}
	built, err := CreateGraphQuery(json.RawMessage(`{}`), component.Dependencies{NATSClient: client, Logger: slog.New(logs)})
	if err != nil {
		t.Fatal(err)
	}
	startLifecycle(t, "graph-query", built)
	c := built.(*Component)
	awaitTextMark(t, client, 0, 5) // after the unreadable write, which comes first
	servePages(t, client, nil)

	answer, err := askText(t, client, `{"query":"pump station"}`)
	if err != nil {
		t.Fatal(err)
	}
	if answer.data.UnreadableGroups != 1 || len(answer.data.Groups) != 1 {
		t.Errorf("answer = %+v, want one group and one unreadable", answer.data)
	}
	if want := "Found 0 matching entities among 0 read from 1 groups at level 0. c360.test.text.a: Pump station. 1 group records at this level could not be read."; answer.data.Answer != want {
		t.Errorf("answer text = %q, want %q", answer.data.Answer, want)
	}
	if got := c.Health().ErrorCount; got != 1 {
		t.Errorf("ErrorCount = %d, want 1", got)
	}
	wantWarn := warnRecord{level: slog.LevelWarn, attrs: map[string]string{
		"verb": "globalSearch", "level": "0", "unreadable": "1",
		"reason": "community records are independent, and the answer reads only the readable ones",
	}}
	if got := logs.snapshot(); len(got) != 1 || !reflect.DeepEqual(got[0], wantWarn) {
		t.Errorf("warnings = %+v, want one %+v", got, wantWarn)
	}

	putRecord(t, kv, "0."+textGroup+"p", `{"id":"c360.test.text.p","level":0,"member_count":1,"entity_revision":6,"keywords":["valve"]}`)
	awaitTextMark(t, client, 0, 6)
	answer, err = askText(t, client, `{"query":"pump station"}`)
	if err != nil || answer.data.UnreadableGroups != 0 {
		t.Errorf("after a valid write: %+v, %v; want unreadable_groups 0", answer.data, err)
	}
	if got := c.Health().ErrorCount; got != 1 {
		t.Errorf("after a valid write: ErrorCount = %d, want 1", got)
	}
	if got := logs.snapshot(); len(got) != 1 {
		t.Errorf("after a valid write: %d warnings, want the first alone", len(got))
	}
}
