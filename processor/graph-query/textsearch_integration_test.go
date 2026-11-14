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

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/vocabulary"
)

// thirdDrone is an entity in a third level-0 group, beside systemGroup and otherDrone's.
const thirdDrone = "c360.test.query.third.drone.001"

// textAnswer is a global search reply's text as the wire carries it: the envelope's revision and the data.
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
// unless text is its one result: no entity and no walk beside it.
func decodeTextAnswer(t *testing.T, raw []byte) textAnswer {
	t.Helper()
	envelope, data := decodeGlobal(t, raw)
	if data.Entity != nil || data.Path != nil {
		t.Fatalf("data = %s, want text alone", envelope.Data)
	}
	return textAnswer{revision: envelope.IndexedRevision, data: *data.Text}
}

// groupOfID is an entity ID's group at level, as the test reads an ID: its first 4 - level parts.
func groupOfID(id string, level int) string {
	return strings.Join(strings.Split(id, ".")[:4-level], ".")
}

// TestGlobalSearchGroupsHits: over real graph-ingest, graph-lexical and community records, the
// groups are listed by their best match, each with its match count: on `pump`, where that order is
// neither key order nor count order. On `pump station` the matches are the word index's unscoped
// hits in its order, read back as the batch verb sends them, and each group also carries its record;
// the counts sum to the matches (spec, "Grouped hits"). At level 1 the four share one group. The
// groups are worked out here from the word index's own reply, by each ID's parts.
func TestGlobalSearchGroupsHits(t *testing.T) {
	stack := startCommunityStack(t)
	stack.write(t, drone1, [2]string{vocabulary.DCTermsTitle, "pump station assembly line"})
	stack.write(t, drone2, [2]string{vocabulary.DCTermsTitle, "pump valve housing unit"})
	stack.write(t, otherDrone, [2]string{vocabulary.DCTermsTitle, "pump intake filter screen panel mesh"})
	last := stack.write(t, thirdDrone, [2]string{vocabulary.DCTermsTitle, "pump"})
	tick(t, stack.maintainer)
	awaitWordIndexed(t, stack.client, last)
	keys := []string{systemGroupKey, "0.c360.test.query.other", "0.c360.test.query.third", "1.c360.test.query"}
	for _, key := range keys {
		awaitCommunityView(t, stack.component, key, storedRecord(t, stack.records, key).EntityRevision)
	}

	raw, err := wordSearchDirect(t, stack.client, graph.LexicalSearchRequest{Query: "pump"})
	if err != nil {
		t.Fatal(err)
	}
	var unscoped graph.QueryResponse[graph.LexicalSearchData]
	if err := json.Unmarshal(raw, &unscoped); err != nil {
		t.Fatal(err)
	}
	hits := unscoped.Data.Results
	// The premise: the best match's group is neither the first by key nor the one with most matches.
	var order []string
	counts := map[string]int{}
	for _, hit := range hits {
		group := groupOfID(hit.EntityID, 0)
		if counts[group] == 0 {
			order = append(order, group)
		}
		counts[group]++
	}
	if len(hits) != 4 || len(order) != 3 || order[0] != "c360.test.query.third" || counts["c360.test.query.system"] != 2 {
		t.Fatalf("word search = %+v, want four hits in three groups, the third group's first", hits)
	}
	// Here the third group, with one match, comes before the system group, with two.
	answer, err := askText(t, stack.client, `{"query":"pump"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.data.Groups) != len(order) {
		t.Errorf("pump: groups %+v, want %v", answer.data.Groups, order)
	}
	for i, group := range answer.data.Groups {
		if i < len(order) && (group.CommunityID != order[i] || group.Matches != counts[order[i]]) {
			t.Errorf("pump: groups[%d] = %+v, want %q with %d matches", i, group, order[i], counts[order[i]])
		}
	}
	rows := batchRows(t, stack.client, drone1, drone2, otherDrone, thirdDrone)

	answer, err = askText(t, stack.client, `{"query":"pump station"}`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = wordSearchDirect(t, stack.client, graph.LexicalSearchRequest{Query: "pump station"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &unscoped); err != nil {
		t.Fatal(err)
	}
	hits = unscoped.Data.Results
	if len(answer.data.Matches) != len(hits) || answer.revision != unscoped.IndexedRevision {
		t.Fatalf("matches %v at %d, want the word index's %d hits at %d", matchIDs(answer.data.Matches), answer.revision, len(hits), unscoped.IndexedRevision)
	}
	order, counts = nil, map[string]int{}
	for i, hit := range hits {
		match := answer.data.Matches[i]
		if match.ID != hit.EntityID || match.Score != hit.Score || string(match.Entity) != rows[hit.EntityID] {
			t.Errorf("matches[%d] = %s %v %s, want the hit %s %v as the batch sent it", i, match.ID, match.Score, match.Entity, hit.EntityID, hit.Score)
		}
		group := groupOfID(hit.EntityID, 0)
		if counts[group] == 0 {
			order = append(order, group)
		}
		counts[group]++
	}
	sum := 0
	for i, group := range answer.data.Groups {
		sum += group.Matches
		if i >= len(order) || group.CommunityID != order[i] || group.Matches != counts[order[i]] {
			t.Errorf("groups[%d] = %+v, want %q with %d matches", i, group, order[min(i, len(order)-1)], counts[order[min(i, len(order)-1)]])
			continue
		}
		got, err := json.Marshal(group.Record)
		if err != nil || group.RecordState != graph.RecordRead {
			t.Errorf("groups[%d] record %s, state %q; want it read", i, got, group.RecordState)
			continue
		}
		requireSameJSON(t, got, recordJSON(t, storedRecord(t, stack.records, "0."+group.CommunityID)))
	}
	if len(answer.data.Groups) != len(order) || sum != len(answer.data.Matches) {
		t.Errorf("%d groups holding %d matches, want %d groups holding %d", len(answer.data.Groups), sum, len(order), len(answer.data.Matches))
	}
	if want := fmt.Sprintf("Found %d matching entities in %d groups.", unscoped.Data.Matched, len(order)); answer.data.Answer != want {
		t.Errorf("answer = %q, want %q", answer.data.Answer, want)
	}

	answer, err = askText(t, stack.client, `{"query":"pump station","level":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if groups := answer.data.Groups; answer.data.Level != 1 || len(groups) != 1 || groups[0].CommunityID != "c360.test.query" ||
		groups[0].Matches != len(hits) || groups[0].RecordState != graph.RecordRead {
		t.Errorf("level 1: level %d, groups %+v; want the one group c360.test.query with %d matches, read", answer.data.Level, groups, len(hits))
	}
}

// TestGlobalSearchWithoutCommunityIndex: with no COMMUNITY_INDEX, global search answers its matches
// with every group's record_state not_ready and no record, and counts no error (spec, "Before any
// community record"). Nothing in this test creates the bucket, so no wait can make it ready.
func TestGlobalSearchWithoutCommunityIndex(t *testing.T) {
	client := fixtureClient(t)
	stubWordIndex(t, client, lexicalReply(t, 3, graph.LexicalSearchData{
		Results: []graph.LexicalHit{{EntityID: drone1, Score: 2}, {EntityID: otherDrone, Score: 1}}, Matched: 2,
	}))
	stubBatch(t, client, `{"entities":[`+titled(drone1, "pump")+`,`+titled(otherDrone, "pump")+`]}`)
	c := startComponent(t, client, `{}`)

	answer, err := askText(t, client, `{"query":"pump station"}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(answer.data.Groups)
	if err != nil {
		t.Fatal(err)
	}
	if want := `[{"community_id":"c360.test.query.system","matches":1,"record_state":"not_ready"},` +
		`{"community_id":"c360.test.query.other","matches":1,"record_state":"not_ready"}]`; string(got) != want {
		t.Errorf("groups = %s, want %s", got, want)
	}
	if answer.data.UnreadableGroups != 0 || len(answer.data.Matches) != 2 {
		t.Errorf("unreadable_groups %d, matches %v; want 0 and both hits", answer.data.UnreadableGroups, matchIDs(answer.data.Matches))
	}
	if health := c.Health(); !health.Healthy || health.ErrorCount != 0 {
		t.Errorf("Health = %+v, want healthy with no error", health)
	}
}

// TestGlobalSearchMatchesKeepWordIndexOrder: the batch sends the hits' entities in another order, and
// the matches follow the word index's order, each with its score and the batch's bytes; the word
// search is asked unscoped with the request's query, limit and min_revision, and the batch with the
// hits in order; the reply is under the word search's revision (spec, "Order and missing hits").
func TestGlobalSearchMatchesKeepWordIndexOrder(t *testing.T) {
	client := fixtureClient(t)
	startComponent(t, client, `{}`)
	hits := []graph.LexicalHit{{EntityID: drone3, Score: 3.5}, {EntityID: otherDrone, Score: 2.25}, {EntityID: drone1, Score: 1}}
	stubWordIndex(t, client, lexicalReply(t, 42, graph.LexicalSearchData{Results: hits, Matched: 3, Excluded: []string{"test"}, BodiesNotIndexed: 2}))
	stubBatch(t, client, `{"entities":[`+titled(drone1, "pump one")+`,`+titled(otherDrone, "pump two")+`,`+titled(drone3, "pump three")+`]}`)
	wordRequests := spyOnDownstream(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject)
	batchRequests := spyOnDownstream(t, client, verbRow(t, graph.ResponderGraphIngest, "batch").Subject)

	raw, err := client.RequestClassified(t.Context(), globalSearchSubject, []byte(`{"query":"pump -test","limit":3,"min_revision":5}`), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	envelope := decodeEnvelope(t, raw)
	if envelope.IndexedRevision != 42 {
		t.Errorf("indexed_revision = %d, want the word search's 42", envelope.IndexedRevision)
	}
	requireSameJSON(t, envelope.Data, `{"text":{"level":0,"groups":[`+
		`{"community_id":"c360.test.query.system","matches":2,"record_state":"not_ready"},`+
		`{"community_id":"c360.test.query.other","matches":1,"record_state":"not_ready"}],"matches":[`+
		`{"id":"`+drone3+`","label":"pump three","score":3.5,"entity":`+titled(drone3, "pump three")+`},`+
		`{"id":"`+otherDrone+`","label":"pump two","score":2.25,"entity":`+titled(otherDrone, "pump two")+`},`+
		`{"id":"`+drone1+`","label":"pump one","score":1,"entity":`+titled(drone1, "pump one")+`}],`+
		`"matched":3,"excluded":["test"],"missing":0,"bodies_not_indexed":2,"truncated":false,"unreadable_groups":0,`+
		`"answer":"Found 3 matching entities in 2 groups."},"hints":[]}`)
	if got := wordRequests(); len(got) != 1 {
		t.Errorf("word search requests = %q, want one", got)
	} else {
		requireSameJSON(t, []byte(got[0]), `{"query":"pump -test","limit":3,"min_revision":5}`)
	}
	if got := batchRequests(); len(got) != 1 {
		t.Errorf("batch requests = %q, want one", got)
	} else {
		requireSameJSON(t, []byte(got[0]), `{"ids":["`+drone3+`","`+otherDrone+`","`+drone1+`"]}`)
	}
}

// TestGlobalSearchPassesWordIndexRefusals: each refusal the word index sends reaches the caller with
// its class, code, detail and message, for a longer question and a one-word one alike, and no other
// request is made: no batch, no alias, no entity read. No error is counted (spec, "A refusal from the
// word index").
func TestGlobalSearchPassesWordIndexRefusals(t *testing.T) {
	client := fixtureClient(t)
	c := startComponent(t, client, `{}`)
	var refusal error
	serve(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject, func(context.Context, []byte) ([]byte, error) {
		return nil, refusal
	})
	stubBatch(t, client, `{"entities":[]}`)
	others := map[string]func() []string{}
	for _, row := range []graph.QueryVerb{
		verbRow(t, graph.ResponderGraphIngest, "batch"), verbRow(t, graph.ResponderGraphIndex, "alias"),
		verbRow(t, graph.ResponderGraphIngest, "entity"),
	} {
		others[row.Name] = spyOnDownstream(t, client, row.Subject)
	}

	for _, want := range []*errs.ClassifiedError{
		errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady, errors.New("word index replaying")),
		errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired, errors.New("graph state reset")),
		errs.ClassifiedCodeDetail(errs.ErrorTransient, graph.ErrorCodeRevisionNotReached, map[string]any{"indexed_revision": 7},
			errors.New("min_revision 9 is above the indexed revision 7")),
		errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest,
			errors.New(`invalid request: "pump*"; put it in double quotes to search it as text`)),
	} {
		refusal = want
		for _, request := range []string{`{"query":"pump* station"}`, `{"query":"pump*"}`} {
			raw, err := client.RequestClassified(t.Context(), globalSearchSubject, []byte(request), requestTimeout)
			got := requireRefusal(t, raw, err, want.Class, want.Code)
			if !strings.Contains(err.Error(), want.Err.Error()) {
				t.Errorf("%s %s: refusal %q does not carry the word index's %q", request, want.Code, err, want.Err)
			}
			if fmt.Sprint(got.Detail) != fmt.Sprint(roundTripped(want.Detail)) {
				t.Errorf("%s %s: detail = %v, want %v", request, want.Code, got.Detail, want.Detail)
			}
		}
	}
	for name, spy := range others {
		if got := spy(); len(got) != 0 {
			t.Errorf("%s requests = %q, want none", name, got)
		}
	}
	if got := c.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0: a responder's refusal is an answer", got)
	}
}

// TestSearchRevisionIsTheWordIndexs: both verbs answer under the word search's revision, not the
// community records' (spec, "The revision"). The record is at entity_revision 1 and the word index
// answers at 42.
func TestSearchRevisionIsTheWordIndexs(t *testing.T) {
	client := fixtureClient(t)
	stubWordIndex(t, client, lexicalReply(t, 42, graph.LexicalSearchData{Results: []graph.LexicalHit{{EntityID: drone1, Score: 1}}, Matched: 1}))
	stubBatch(t, client, `{"entities":[`+titled(drone1, "pump")+`]}`)
	withCommunityRecord(t, client)
	c := startComponent(t, client, `{}`)
	awaitCommunityView(t, c, systemGroupKey, 1)
	for verb, request := range map[string]string{
		"localSearch":  `{"entity_id":"` + drone1 + `","query":"pump"}`,
		"globalSearch": `{"query":"pump station"}`,
		"searchGraph":  `{"query":"pump station"}`,
	} {
		raw, err := client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphQuery, verb).Subject, []byte(request), requestTimeout)
		if err != nil {
			t.Fatalf("%s: %v", verb, err)
		}
		if got := decodeEnvelope(t, raw).IndexedRevision; got != 42 {
			t.Errorf("%s: indexed_revision = %d, want the word index's 42", verb, got)
		}
	}
}

// TestSearchDeclaresWhatItLeftOut: over real graph-ingest and graph-lexical, 40 entities match
// "pump" and one more holds the excluded word; with the default limit both verbs answer ten, with
// matched 40, truncated true and the excluded word declared (spec, "A limited answer").
func TestSearchDeclaresWhatItLeftOut(t *testing.T) {
	stack := startCommunityStack(t)
	var last uint64
	for i := 1; i <= 40; i++ {
		last = stack.create(t, fmt.Sprintf("c360.test.query.system.drone.%03d", i), [2]string{vocabulary.DCTermsTitle, "pump"})
	}
	last = stack.create(t, "c360.test.query.system.drone.041", [2]string{vocabulary.DCTermsTitle, "pump valve"})
	awaitWordIndexed(t, stack.client, last)

	raw, err := search(t, stack.client, `{"entity_id":"`+drone1+`","query":"pump -valve"}`)
	_, local := searchSuccess(t, raw, err)
	if local.Matched != 40 || len(local.Matches) != 10 || !local.Truncated || !slices.Equal(local.Excluded, []string{"valve"}) {
		t.Errorf("localSearch: matched %d, %d matches, truncated %v, excluded %q; want 40, 10, true, [valve]",
			local.Matched, len(local.Matches), local.Truncated, local.Excluded)
	}
	answer, err := askText(t, stack.client, `{"query":"pump -valve"}`)
	if err != nil {
		t.Fatal(err)
	}
	global := answer.data
	if global.Matched != 40 || len(global.Matches) != 10 || !global.Truncated || !slices.Equal(global.Excluded, []string{"valve"}) {
		t.Errorf("globalSearch: matched %d, %d matches, truncated %v, excluded %q; want 40, 10, true, [valve]",
			global.Matched, len(global.Matches), global.Truncated, global.Excluded)
	}
	if want := "Found 40 matching entities in 1 groups. Showing the first 10."; global.Answer != want {
		t.Errorf("globalSearch answer = %q, want %q", global.Answer, want)
	}
}

// TestSearchWithoutWordIndex: with nothing serving the word search, both verbs are refused transient
// internal naming its subject, each one error (spec, "No word index").
func TestSearchWithoutWordIndex(t *testing.T) {
	client := fixtureClient(t)
	c := startComponent(t, client, `{}`)
	subject := verbRow(t, graph.ResponderGraphLexical, "search").Subject
	for i, ask := range []struct{ verb, request string }{
		{localSearchSubject, `{"entity_id":"` + drone1 + `","query":"pump"}`},
		{globalSearchSubject, `{"query":"pump station"}`},
	} {
		raw, err := client.RequestClassified(t.Context(), ask.verb, []byte(ask.request), requestTimeout)
		requireRefusal(t, raw, err, errs.ErrorTransient, graph.ErrorCodeInternal)
		if !strings.Contains(err.Error(), subject) {
			t.Errorf("%s: refusal %q does not name %s", ask.verb, err, subject)
		}
		if got := c.Health().ErrorCount; got != i+1 {
			t.Errorf("%s: ErrorCount = %d, want %d", ask.verb, got, i+1)
		}
	}
}

// TestSearchRequestsAreBounded: counting subscribers beside real graph-ingest, graph-index and
// graph-lexical, a local search is one word search and one batch request; a text question is the
// same and reads no reference; a one-word question that names no entity adds one alias request and
// at most one entity request; a path question adds one alias request, one entity request for its
// start and the walk, at most one outgoing and one incoming request per entity it reaches, here the
// one entity written (spec, "Counting the requests").
func TestSearchRequestsAreBounded(t *testing.T) {
	stack := startCommunityStack(t)
	last := stack.write(t, drone1, [2]string{vocabulary.DCTermsTitle, "pump station"}, aliased("held-1"))
	stack.awaitIndexed(t, last)
	awaitWordIndexed(t, stack.client, last)
	spies := map[string]func() []string{}
	for _, row := range []graph.QueryVerb{
		verbRow(t, graph.ResponderGraphLexical, "search"), verbRow(t, graph.ResponderGraphIngest, "batch"),
		verbRow(t, graph.ResponderGraphIndex, "alias"), verbRow(t, graph.ResponderGraphIngest, "entity"),
		verbRow(t, graph.ResponderGraphIndex, "outgoing"), verbRow(t, graph.ResponderGraphIndex, "incoming"),
	} {
		spies[row.Name] = spyOnDownstream(t, stack.client, row.Subject)
	}
	for _, tc := range []struct {
		name, verb, request string
		at                  map[string]int // each verb's exact count, or its upper bound when bounded is true
		bounded             bool
	}{
		{"local", localSearchSubject, `{"entity_id":"` + drone1 + `","query":"pump"}`, map[string]int{"search": 1, "batch": 1}, false},
		{"text", globalSearchSubject, `{"query":"pump station"}`, map[string]int{"search": 1, "batch": 1}, false},
		{"one word naming no entity", globalSearchSubject, `{"query":"pump"}`, map[string]int{"search": 1, "batch": 1, "alias": 1, "entity": 1}, true},
		{"a path question", globalSearchSubject, `{"query":"what is connected to held-1 pump"}`,
			map[string]int{"search": 1, "batch": 1, "alias": 1, "entity": 1, "outgoing": 1, "incoming": 1}, false},
	} {
		raw, err := stack.client.RequestClassified(t.Context(), tc.verb, []byte(tc.request), requestTimeout)
		if err != nil || !bytes.Contains(raw, []byte(drone1)) {
			t.Fatalf("%s: %s, %v; want an answer naming %s", tc.name, raw, err, drone1)
		}
		for name, spy := range spies {
			got, want := len(spy()), tc.at[name]
			if got > want || (!tc.bounded && got != want) {
				t.Errorf("%s: %d %s requests, want %d", tc.name, got, name, want)
			}
		}
	}
}

// warnRecord is one Warn logged past unreadable records, with its attributes as text.
type warnRecord struct {
	level slog.Level
	attrs map[string]string
}

// warnCapture is a slog.Handler keeping the records logged with global search's unreadable message.
type warnCapture struct {
	mu      sync.Mutex
	records []warnRecord
}

func (w *warnCapture) Enabled(context.Context, slog.Level) bool { return true }
func (w *warnCapture) WithAttrs([]slog.Attr) slog.Handler       { return w }
func (w *warnCapture) WithGroup(string) slog.Handler            { return w }

func (w *warnCapture) Handle(_ context.Context, r slog.Record) error {
	if r.Message != "global search answered past unreadable community records" {
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

// TestUnreadableRecordIsDeclared: an answer whose matches' groups include one with an unreadable
// record says so in that group's record_state and in unreadable_groups, counts one error in Health
// and logs one Warn naming the verb, the level, the count and why answering is safe; after a valid
// write to that key the answer counts none and logs nothing (spec, "A corrupt record"). The handler
// logs and counts before it replies, so each read below is ordered after the reply it follows.
func TestUnreadableRecordIsDeclared(t *testing.T) {
	client := fixtureClient(t)
	const poisoned = "c360.test.query.poison.drone.001"
	stubWordIndex(t, client, lexicalReply(t, 1, graph.LexicalSearchData{
		Results: []graph.LexicalHit{{EntityID: drone1, Score: 2}, {EntityID: poisoned, Score: 1}}, Matched: 2,
	}))
	stubBatch(t, client, `{"entities":[`+titled(drone1, "pump")+`,`+titled(poisoned, "pump")+`]}`)
	kv, m := communityRecords(t, client)
	publishDrones(t, m, 1, drone1)
	if _, err := kv.Put(t.Context(), "0.c360.test.query.poison", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	logs := &warnCapture{}
	built, err := CreateGraphQuery(json.RawMessage(`{}`), component.Dependencies{NATSClient: client, Logger: slog.New(logs)})
	if err != nil {
		t.Fatal(err)
	}
	startLifecycle(t, "graph-query", built)
	c := built.(*Component)
	const request = `{"query":"pump station"}`
	// The view applies the bucket's writes in order; each answer before it has is the event.
	awaitText := func(done func(graph.TextSearchData) bool) graph.TextSearchData {
		t.Helper()
		got, err := probe.Await(t.Context(), func(ctx context.Context) (searchOutcome, error) {
			raw, err := client.RequestClassified(ctx, globalSearchSubject, []byte(request), requestTimeout)
			return searchOutcome{raw, err}, nil
		}, func(o searchOutcome) bool {
			var reply graph.QueryResponse[graph.GlobalSearchData]
			return o.err != nil || json.Unmarshal(o.raw, &reply) != nil || reply.Data.Text == nil || done(*reply.Data.Text)
		})
		if err != nil || got.err != nil {
			t.Fatalf("await globalSearch: %v, %v", err, got.err)
		}
		return decodeTextAnswer(t, got.raw).data
	}
	awaitText(func(d graph.TextSearchData) bool { return d.UnreadableGroups > 0 })
	errorsBefore, warnsBefore := c.Health().ErrorCount, len(logs.snapshot())

	answer, err := askText(t, client, request)
	if err != nil {
		t.Fatal(err)
	}
	if groups := answer.data.Groups; answer.data.UnreadableGroups != 1 || len(groups) != 2 ||
		groups[0].RecordState != graph.RecordRead || groups[1].RecordState != graph.RecordUnreadable || groups[1].Record != nil {
		t.Errorf("answer = %+v, want drone1's group read and the poisoned one unreadable", answer.data)
	}
	if got := c.Health().ErrorCount; got != errorsBefore+1 {
		t.Errorf("ErrorCount = %d, want %d", got, errorsBefore+1)
	}
	wantWarn := warnRecord{level: slog.LevelWarn, attrs: map[string]string{
		"verb": "globalSearch", "level": "0", "unreadable": "1",
		"reason": "a record only describes its group; the matches come from the word index",
	}}
	if got := logs.snapshot(); len(got) != warnsBefore+1 || !reflect.DeepEqual(got[len(got)-1], wantWarn) {
		t.Errorf("warnings = %+v, want one more, %+v", got, wantWarn)
	}

	if _, err := kv.Put(t.Context(), "0.c360.test.query.poison", []byte(`{"id":"c360.test.query.poison","level":0,"member_count":1,"entity_revision":6}`)); err != nil {
		t.Fatal(err)
	}
	awaitText(func(d graph.TextSearchData) bool { return d.UnreadableGroups == 0 })
	errorsBefore, warnsBefore = c.Health().ErrorCount, len(logs.snapshot())
	answer, err = askText(t, client, request)
	if err != nil || answer.data.UnreadableGroups != 0 || answer.data.Groups[1].RecordState != graph.RecordRead {
		t.Errorf("after a valid write: %+v, %v; want unreadable_groups 0 and the record read", answer.data, err)
	}
	if got := c.Health().ErrorCount; got != errorsBefore {
		t.Errorf("after a valid write: ErrorCount = %d, want %d", got, errorsBefore)
	}
	if got := logs.snapshot(); len(got) != warnsBefore {
		t.Errorf("after a valid write: %d warnings, want %d", len(got), warnsBefore)
	}
}
