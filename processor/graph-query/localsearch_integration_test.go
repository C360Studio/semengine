//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	graphlexical "github.com/c360studio/semengine/processor/graph-lexical"
	"github.com/c360studio/semengine/vocabulary"
)

const (
	localSearchSubject = "graph.query.localSearch"
	// systemGroup is drone1's level-0 group, and its record's key.
	systemGroup    = "c360.test.query.system"
	systemGroupKey = "0." + systemGroup
	// otherDrone is an entity outside systemGroup, in the same level-1 group.
	otherDrone = "c360.test.query.other.drone.001"
	// farAhead is a revision no test writes.
	farAhead = uint64(1) << 40
)

// communityRecords creates COMMUNITY_INDEX as its owner, graph-clustering, does, and returns it with a
// maintainer publishing to it whose replay is done.
func communityRecords(t *testing.T, client *natsclient.Client) (jetstream.KeyValue, *clustering.Maintainer) {
	t.Helper()
	kv, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketCommunityIndex)
	if err != nil {
		t.Fatalf("ensure %s: %v", graph.BucketCommunityIndex, err)
	}
	storage, err := clustering.NewNATSCommunityStorage(client.NewKVStore(kv))
	if err != nil {
		t.Fatal(err)
	}
	m, err := clustering.NewMaintainer(storage)
	if err != nil {
		t.Fatal(err)
	}
	m.BootstrapDone()
	return kv, m
}

func tick(t *testing.T, m *clustering.Maintainer) {
	t.Helper()
	if err := m.Tick(t.Context()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
}

// publishDrones publishes, through a real maintainer, the records of entities of type drone with the
// given IDs, the first at revision first, the next one above, and so on. No graph-ingest runs: the
// states are the test's.
func publishDrones(t *testing.T, m *clustering.Maintainer, first uint64, ids ...string) {
	t.Helper()
	for i, id := range ids {
		state := &graph.EntityState{ID: id, Triples: []message.Triple{{Subject: id, Predicate: "core.identity.type", Object: "drone"}}}
		if err := m.Upsert(id, first+uint64(i), state); err != nil {
			t.Fatalf("Upsert %s: %v", id, err)
		}
	}
	tick(t, m)
}

// codeOf is err's refusal code, or "" for a success or an error with none.
func codeOf(err error) string {
	var refusal *errs.ClassifiedError
	if errors.As(err, &refusal) {
		return refusal.Code
	}
	return ""
}

type searchOutcome struct {
	raw []byte
	err error
}

func search(t *testing.T, client *natsclient.Client, request string) ([]byte, error) {
	t.Helper()
	return client.RequestClassified(t.Context(), localSearchSubject, []byte(request), requestTimeout)
}

// awaitAnswer sends request until localSearch answers with data done accepts, or refuses, and returns
// that outcome decoded. Each answer done does not accept is the event that the view, which opens and
// catches up on its own goroutine, has not applied what the test wrote yet; go test -timeout ends a
// view that never does.
func awaitAnswer(t *testing.T, client *natsclient.Client, request string, done func(graph.LocalSearchData) bool) (wireEnvelope, graph.LocalSearchData) {
	t.Helper()
	got, err := probe.Await(t.Context(), func(ctx context.Context) (searchOutcome, error) {
		raw, err := client.RequestClassified(ctx, localSearchSubject, []byte(request), requestTimeout)
		return searchOutcome{raw, err}, nil
	}, func(o searchOutcome) bool {
		var reply graph.QueryResponse[graph.LocalSearchData]
		return o.err != nil || json.Unmarshal(o.raw, &reply) != nil || done(reply.Data)
	})
	if err != nil {
		t.Fatalf("await localSearch %s: %v", request, err)
	}
	return searchSuccess(t, got.raw, got.err)
}

// awaitCommunityView waits until c's view of COMMUNITY_INDEX holds key's record at entity_revision
// revision: the view's own Get, which every search reads it through.
func awaitCommunityView(t *testing.T, c *Component, key string, revision uint64) {
	t.Helper()
	got, err := probe.Await(t.Context(), func(context.Context) (uint64, error) {
		view := c.communities.Load()
		if view == nil {
			return 0, nil
		}
		record, _, err := view.Get(key)
		if err != nil {
			return 0, nil
		}
		return record.EntityRevision, nil
	}, func(reached uint64) bool { return reached == revision })
	if err != nil {
		t.Fatalf("await the community view's %s at %d: %v (last %d)", key, revision, err, got)
	}
}

// withCommunityRecord publishes drone1's group record at revision 1 before graph-query starts.
func withCommunityRecord(t *testing.T, client *natsclient.Client) {
	t.Helper()
	_, m := communityRecords(t, client)
	publishDrones(t, m, 1, drone1)
}

// storedRecord is key's record as the bucket holds it, read apart from graph-query.
func storedRecord(t *testing.T, kv jetstream.KeyValue, key string) clustering.Community {
	t.Helper()
	entry, err := kv.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("get %s: %v", key, err)
	}
	var record clustering.Community
	if err := json.Unmarshal(entry.Value(), &record); err != nil {
		t.Fatalf("decode %s: %v", key, err)
	}
	return record
}

// recordJSON is the reply's record for record, from the bucket's own bytes.
func recordJSON(t *testing.T, record clustering.Community) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"member_count": record.MemberCount, "community_summary": record.StatisticalSummary, "keywords": record.Keywords,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// startWordIndex starts a real graph-lexical on client, following graph-ingest's ENTITY_STATES.
func startWordIndex(t *testing.T, client *natsclient.Client) {
	t.Helper()
	lexical, err := graphlexical.CreateGraphLexical(json.RawMessage(`{}`), component.Dependencies{
		NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry(),
	})
	if err != nil {
		t.Fatalf("CreateGraphLexical: %v", err)
	}
	startLifecycle(t, "graph-lexical", lexical)
}

// wordSearchDirect asks graph-lexical's word search itself, apart from graph-query.
func wordSearchDirect(t *testing.T, client *natsclient.Client, request graph.LexicalSearchRequest) ([]byte, error) {
	t.Helper()
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphLexical, "search").Subject, data, requestTimeout)
}

// awaitWordIndexed waits until the word index has applied revision: until then it refuses
// index_not_ready or revision_not_reached, each refusal being the event.
func awaitWordIndexed(t *testing.T, client *natsclient.Client, revision uint64) {
	t.Helper()
	request := graph.LexicalSearchRequest{Query: "nothing", MinRevisionField: graph.MinRevisionField{MinRevision: revision}}
	got, err := probe.Await(t.Context(), func(context.Context) (searchOutcome, error) {
		raw, err := wordSearchDirect(t, client, request)
		return searchOutcome{raw, err}, nil
	}, func(o searchOutcome) bool {
		code := codeOf(o.err)
		return code != graph.ErrorCodeIndexNotReady && code != graph.ErrorCodeRevisionNotReached
	})
	if err != nil || got.err != nil {
		t.Fatalf("await the word index at %d: %v, %v", revision, err, got.err)
	}
}

// communityStack is real graph-ingest, graph-index and graph-lexical behind graph-query, with
// COMMUNITY_INDEX created before graph-query starts and a real maintainer the test feeds.
type communityStack struct {
	graphStack
	component  *Component
	records    jetstream.KeyValue
	maintainer *clustering.Maintainer
}

func startCommunityStack(t *testing.T) communityStack {
	t.Helper()
	stack := startIngestAndIndex(t)
	startWordIndex(t, stack.client)
	kv, m := communityRecords(t, stack.client)
	c := startComponent(t, stack.client, `{}`)
	return communityStack{graphStack: stack, component: c, records: kv, maintainer: m}
}

// write creates id through graph-ingest and applies the entity to the maintainer as graph-ingest holds
// it, at its kv_revision. Nothing else writes, so the maintainer is fed in ascending order.
func (s communityStack) write(t *testing.T, id string, statements ...[2]string) uint64 {
	t.Helper()
	revision := s.create(t, id, statements...)
	raw, err := s.client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphIngest, "entity").Subject,
		fmt.Appendf(nil, `{"id":%q}`, id), requestTimeout)
	if err != nil {
		t.Fatalf("read %s back: %v", id, err)
	}
	var exact graph.ExactEntity
	if err := json.Unmarshal(raw, &exact); err != nil || exact.Entity == nil || exact.KVRevision != revision {
		t.Fatalf("read %s back = %s, %v; want the entity at %d", id, raw, err, revision)
	}
	if err := s.maintainer.Upsert(id, revision, exact.Entity); err != nil {
		t.Fatalf("Upsert %s: %v", id, err)
	}
	return revision
}

// batchRows is each entity of a batch read of ids, by ID, as graph-ingest sends it.
func batchRows(t *testing.T, client *natsclient.Client, ids ...string) map[string]string {
	t.Helper()
	request, err := json.Marshal(graph.EntityBatchRequest{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphIngest, "batch").Subject, request, requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	var reply batchReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatal(err)
	}
	rows := map[string]string{}
	for _, row := range reply.Entities {
		var id struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(row, &id); err != nil {
			t.Fatal(err)
		}
		rows[id.ID] = string(row)
	}
	return rows
}

// searchSuccess decodes a localSearch success: the envelope strictly, and its data.
func searchSuccess(t *testing.T, raw []byte, err error) (wireEnvelope, graph.LocalSearchData) {
	t.Helper()
	if err != nil {
		t.Fatalf("localSearch refused: %v", err)
	}
	envelope := decodeEnvelope(t, raw)
	var data graph.LocalSearchData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode data %s: %v", envelope.Data, err)
	}
	return envelope, data
}

// lexicalReply is a word search success as graph-lexical sends it, at revision.
func lexicalReply(t *testing.T, revision uint64, data graph.LexicalSearchData) []byte {
	t.Helper()
	if data.Results == nil {
		data.Results = []graph.LexicalHit{}
	}
	response, err := graph.NewQueryResponse(graph.ResponderGraphLexical, revision, data)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// stubWordIndex stands in for graph-lexical's word search, answering every request with reply.
func stubWordIndex(t *testing.T, client *natsclient.Client, reply []byte) {
	t.Helper()
	serve(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject, func(context.Context, []byte) ([]byte, error) {
		return reply, nil
	})
}

// stubBatch stands in for graph-ingest's batch verb, answering every request with reply.
func stubBatch(t *testing.T, client *natsclient.Client, reply string) {
	t.Helper()
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "batch").Subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(reply), nil
	})
}

// titled is an entity row as graph-ingest sends one, holding a title.
func titled(id, title string) string {
	return fmt.Sprintf(`{"id":%q,"triples":[{"subject":%q,"predicate":%q,"object":%q}]}`, id, id, vocabulary.DCTermsTitle, title)
}

// matchIDs is the matches' entity IDs, in order.
func matchIDs(matches []graph.TextSearchMatch) []string {
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.ID)
	}
	return ids
}

// envelopeData is a reply's data as it was sent, for a failure message.
func envelopeData(raw []byte) string {
	var reply struct {
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(raw, &reply)
	return string(reply.Data)
}

// TestSearchRequestsDecodeStrictly: with the word index and the batch verb served, each bad request
// to either verb is invalid_request naming its key or field, with no request to either and no error
// counted (spec, "A bad local request", "A bad global request").
func TestSearchRequestsDecodeStrictly(t *testing.T) {
	client := fixtureClient(t)
	c := startComponent(t, client, `{}`)
	// Stubs answer the spies' markers; a bad request reaches neither.
	for _, row := range []graph.QueryVerb{verbRow(t, graph.ResponderGraphLexical, "search"), verbRow(t, graph.ResponderGraphIngest, "batch")} {
		serve(t, client, row.Subject, func(context.Context, []byte) ([]byte, error) {
			return nil, errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, errors.New("a marker"))
		})
	}
	wordRequests := spyOnDownstream(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject)
	batchRequests := spyOnDownstream(t, client, verbRow(t, graph.ResponderGraphIngest, "batch").Subject)

	for _, tc := range []struct{ name, request, says string }{
		{"an unknown key", `{"entity_id":"` + drone1 + `","query":"drone","scope":["c360"]}`, "scope"},
		{"a five-part entity_id", `{"entity_id":"c360.test.query.system.drone","query":"drone"}`, "entity_id"},
		{"level 3", `{"entity_id":"` + drone1 + `","query":"drone","level":3}`, "level"},
		{"a blank query", `{"entity_id":"` + drone1 + `","query":"  "}`, "query"},
		{"limit 101", `{"entity_id":"` + drone1 + `","query":"drone","limit":101}`, "limit"},
		{"not JSON", `not json`, "invalid character"},
	} {
		raw, err := search(t, client, tc.request)
		requireRefusal(t, raw, err, errs.ErrorInvalid, "invalid_request")
		if !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%s: refusal %q does not name %q", tc.name, err, tc.says)
		}
	}
	for _, tc := range []struct{ name, request, says string }{
		{"summarize_threshold", `{"query":"pump station","summarize_threshold":50}`, "summarize_threshold"},
		{"max_communities", `{"query":"pump station","max_communities":5}`, "max_communities"},
		{"include_summaries", `{"query":"pump station","include_summaries":true}`, "include_summaries"},
		{"a blank query", `{"query":" "}`, "query"},
		{"level 3", `{"query":"pump station","level":3}`, "level"},
		{"limit 101", `{"query":"pump station","limit":101}`, "limit"},
	} {
		raw, err := client.RequestClassified(t.Context(), globalSearchSubject, []byte(tc.request), requestTimeout)
		requireRefusal(t, raw, err, errs.ErrorInvalid, "invalid_request")
		if !strings.Contains(err.Error(), tc.says) {
			t.Errorf("global %s: refusal %q does not name %q", tc.name, err, tc.says)
		}
	}
	if got := wordRequests(); len(got) != 0 {
		t.Errorf("word search requests = %q, want none", got)
	}
	if got := batchRequests(); len(got) != 0 {
		t.Errorf("batch requests = %q, want none", got)
	}
	if got := c.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0", got)
	}
}

// TestLocalSearchIsScopedToTheGroup: over real graph-ingest and graph-lexical, an entity outside the
// group that the word index ranks first is not a match, and the group's own match is, read back as
// the batch verb sends it, with the word index's score and revision and the group's record attached
// (spec, "A better match outside the group"). At level 1 both entities share the group, in the word
// index's order.
func TestLocalSearchIsScopedToTheGroup(t *testing.T) {
	stack := startCommunityStack(t)
	stack.write(t, drone1, [2]string{vocabulary.DCTermsTitle, "pump station assembly line"})
	groupLast := stack.write(t, drone2, [2]string{vocabulary.DCTermsTitle, "valve"})
	last := stack.write(t, otherDrone, [2]string{vocabulary.DCTermsTitle, "pump"})
	tick(t, stack.maintainer)
	awaitWordIndexed(t, stack.client, last)
	awaitCommunityView(t, stack.component, systemGroupKey, groupLast)
	rows := batchRows(t, stack.client, drone1, otherDrone)
	record := storedRecord(t, stack.records, systemGroupKey)

	// The premise: unscoped, the entity outside the group is the better match.
	raw, err := wordSearchDirect(t, stack.client, graph.LexicalSearchRequest{Query: "pump"})
	if err != nil {
		t.Fatal(err)
	}
	var unscoped graph.QueryResponse[graph.LexicalSearchData]
	if err := json.Unmarshal(raw, &unscoped); err != nil {
		t.Fatal(err)
	}
	if hits := unscoped.Data.Results; len(hits) != 2 || hits[0].EntityID != otherDrone || hits[1].EntityID != drone1 {
		t.Fatalf("unscoped word search = %+v, want %s ranked above %s", hits, otherDrone, drone1)
	}
	scores := map[string]float64{}
	for _, hit := range unscoped.Data.Results {
		scores[hit.EntityID] = hit.Score
	}

	wordRequests := spyOnDownstream(t, stack.client, verbRow(t, graph.ResponderGraphLexical, "search").Subject)
	batchRequests := spyOnDownstream(t, stack.client, verbRow(t, graph.ResponderGraphIngest, "batch").Subject)
	raw, err = search(t, stack.client, fmt.Sprintf(`{"entity_id":%q,"query":"pump"}`, drone2))
	envelope, _ := searchSuccess(t, raw, err)
	if envelope.IndexedRevision != unscoped.IndexedRevision {
		t.Errorf("indexed_revision = %d, want the word index's %d", envelope.IndexedRevision, unscoped.IndexedRevision)
	}
	requireSameJSON(t, envelope.Data, fmt.Sprintf(`{"community_id":%q,"level":0,"record":%s,"record_state":"read",`+
		`"matches":[{"id":%q,"label":"pump station assembly line","score":%v,"entity":%s}],`+
		`"matched":1,"missing":0,"bodies_not_indexed":0,"truncated":false,"answer":%q}`,
		systemGroup, recordJSON(t, record), drone1, scores[drone1], rows[drone1],
		"Found 1 matching entities in c360.test.query.system. "+record.StatisticalSummary))
	if got := wordRequests(); len(got) != 1 {
		t.Errorf("word search requests = %q, want one", got)
	} else {
		requireSameJSON(t, []byte(got[0]), `{"query":"pump","limit":0,"scope":["c360.test.query.system"]}`)
	}
	if got := batchRequests(); len(got) != 1 {
		t.Errorf("batch requests = %q, want one", got)
	} else {
		requireSameJSON(t, []byte(got[0]), `{"ids":["`+drone1+`"]}`)
	}

	raw, err = search(t, stack.client, fmt.Sprintf(`{"entity_id":%q,"query":"pump","level":1}`, drone2))
	_, data := searchSuccess(t, raw, err)
	ids := matchIDs(data.Matches)
	if data.CommunityID != "c360.test.query" || data.Level != 1 || strings.Join(ids, " ") != otherDrone+" "+drone1 {
		t.Errorf("level 1: community %q level %d matches %v; want c360.test.query, 1, [%s %s]", data.CommunityID, data.Level, ids, otherDrone, drone1)
	}
}

// TestLocalSearchRecordStates: the same matches are answered whether the community view is missing,
// holds the group's record, holds none for it or holds an unreadable one; record_state says which,
// and only the unreadable record counts an error and logs a Warn naming the verb, the level, the
// count and why answering is safe (spec, "No community record", "An answer past unreadable records
// is signalled"). The handler logs and counts before it replies, so each read below is ordered
// after the reply it follows.
func TestLocalSearchRecordStates(t *testing.T) {
	client := fixtureClient(t)
	stubWordIndex(t, client, lexicalReply(t, 9, graph.LexicalSearchData{Results: []graph.LexicalHit{{EntityID: drone1, Score: 1.5}}, Matched: 1}))
	stubBatch(t, client, `{"entities":[`+titled(drone1, "pump")+`]}`)
	logs := &warnCapture{message: "local search answered past an unreadable community record"}
	built, err := CreateGraphQuery(json.RawMessage(`{}`), component.Dependencies{NATSClient: client, Logger: slog.New(logs)})
	if err != nil {
		t.Fatal(err)
	}
	startLifecycle(t, "graph-query", built)
	c := built.(*Component)
	const (
		inGroup     = `{"entity_id":"` + drone1 + `","query":"pump"}`
		unpublished = `{"entity_id":"c360.test.query.unpublished.drone.001","query":"pump"}`
		poisoned    = `{"entity_id":"c360.test.query.poison.drone.001","query":"pump"}`
	)
	answered := func(t *testing.T, data graph.LocalSearchData, state graph.RecordState) {
		t.Helper()
		if data.RecordState != state || (data.Record != nil) != (state == graph.RecordRead) {
			t.Errorf("record_state %q, record %+v; want %q, a record exactly when read", data.RecordState, data.Record, state)
		}
		if len(data.Matches) != 1 || data.Matches[0].ID != drone1 || data.Matched != 1 {
			t.Errorf("matches %+v, matched %d; want drone1 alone", data.Matches, data.Matched)
		}
	}

	t.Run("no view", func(t *testing.T) {
		raw, err := search(t, client, inGroup)
		_, data := searchSuccess(t, raw, err)
		answered(t, data, graph.RecordNotReady)
	})

	kv, m := communityRecords(t, client)
	publishDrones(t, m, 1, drone1)
	awaitCommunityView(t, c, systemGroupKey, 1)

	t.Run("a record", func(t *testing.T) {
		raw, err := search(t, client, inGroup)
		_, data := searchSuccess(t, raw, err)
		answered(t, data, graph.RecordRead)
		record := storedRecord(t, kv, systemGroupKey)
		if want := (graph.LocalSearchRecord{MemberCount: record.MemberCount, CommunitySummary: record.StatisticalSummary, Keywords: record.Keywords}); data.Record == nil ||
			data.Record.MemberCount != want.MemberCount || data.Record.CommunitySummary != want.CommunitySummary ||
			strings.Join(data.Record.Keywords, " ") != strings.Join(want.Keywords, " ") {
			t.Errorf("record = %+v, want the bucket's %+v", data.Record, want)
		}
	})

	t.Run("no record for the group", func(t *testing.T) {
		raw, err := search(t, client, unpublished)
		_, data := searchSuccess(t, raw, err)
		answered(t, data, graph.RecordNone)
	})
	if got := c.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0: a missing record is not an error", got)
	}
	if got := logs.snapshot(); len(got) != 0 {
		t.Errorf("warnings = %+v, want none: only an unreadable record is warned about", got)
	}

	t.Run("an unreadable record", func(t *testing.T) {
		if _, err := kv.Put(t.Context(), "0.c360.test.query.poison", []byte(`{"id":"c360.test.query.poison","level":0,"member_count":1}`)); err != nil {
			t.Fatal(err)
		}
		// Until the view applies the put, the group has no record.
		_, data := awaitAnswer(t, client, poisoned, func(d graph.LocalSearchData) bool { return d.RecordState != graph.RecordNone })
		answered(t, data, graph.RecordUnreadable)
		errorsBefore, warnsBefore := c.Health().ErrorCount, len(logs.snapshot())
		raw, err := search(t, client, poisoned)
		_, data = searchSuccess(t, raw, err)
		answered(t, data, graph.RecordUnreadable)
		if got := c.Health().ErrorCount; got != errorsBefore+1 {
			t.Errorf("ErrorCount = %d after one answer past an unreadable record, want %d", got, errorsBefore+1)
		}
		wantWarn := warnRecord{level: slog.LevelWarn, attrs: map[string]string{
			"verb": "localSearch", "level": "0", "unreadable": "1", "community_id": "c360.test.query.poison",
			"reason": "the record only describes the group; the matches come from the word index",
		}}
		if got := logs.snapshot(); len(got) != warnsBefore+1 || !reflect.DeepEqual(got[len(got)-1], wantWarn) {
			t.Errorf("warnings = %+v, want one more, %+v", got, wantWarn)
		}
	})
}

// TestLocalSearchPassesWordIndexRefusals: each refusal the word index sends reaches the caller with
// its class, code, detail and message, no batch request is made and no error is counted (spec, "A
// refusal from the word index").
func TestLocalSearchPassesWordIndexRefusals(t *testing.T) {
	client := fixtureClient(t)
	c := startComponent(t, client, `{}`)
	var refusal error
	serve(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject, func(context.Context, []byte) ([]byte, error) {
		return nil, refusal
	})
	stubBatch(t, client, `{"entities":[]}`)
	batchRequests := spyOnDownstream(t, client, verbRow(t, graph.ResponderGraphIngest, "batch").Subject)

	for _, want := range []*errs.ClassifiedError{
		errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady, errors.New("word index replaying")),
		errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired, errors.New("graph state reset")),
		errs.ClassifiedCodeDetail(errs.ErrorTransient, graph.ErrorCodeRevisionNotReached, map[string]any{"indexed_revision": 7},
			errors.New("min_revision 9 is above the indexed revision 7")),
		errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest,
			errors.New(`invalid request: "pump*"; put it in double quotes to search it as text`)),
	} {
		refusal = want
		raw, err := search(t, client, `{"entity_id":"`+drone1+`","query":"pump*"}`)
		got := requireRefusal(t, raw, err, want.Class, want.Code)
		if !strings.Contains(err.Error(), want.Err.Error()) {
			t.Errorf("%s: refusal %q does not carry the word index's %q", want.Code, err, want.Err)
		}
		if fmt.Sprint(got.Detail) != fmt.Sprint(roundTripped(want.Detail)) {
			t.Errorf("%s: detail = %v, want %v", want.Code, got.Detail, want.Detail)
		}
	}
	if got := batchRequests(); len(got) != 0 {
		t.Errorf("batch requests = %q, want none", got)
	}
	if got := c.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0: a responder's refusal is an answer", got)
	}
}

// roundTripped is detail as a JSON round trip gives it back, numbers as float64.
func roundTripped(detail map[string]any) map[string]any {
	if detail == nil {
		return nil
	}
	encoded, _ := json.Marshal(detail)
	var out map[string]any
	_ = json.Unmarshal(encoded, &out)
	return out
}

// TestSearchMatchesKeepHitOrder: the batch sends the hits' entities in another order, and the
// matches follow the word index's order, each with its score and the batch's bytes; the word search
// is asked with the request's query, limit and min_revision, scoped to the group, and the batch with
// the hits in order (spec, "Order and missing hits").
func TestSearchMatchesKeepHitOrder(t *testing.T) {
	client := fixtureClient(t)
	startComponent(t, client, `{}`)
	hits := []graph.LexicalHit{{EntityID: drone3, Score: 3.5}, {EntityID: drone1, Score: 2.25}, {EntityID: drone2, Score: 1}}
	stubWordIndex(t, client, lexicalReply(t, 42, graph.LexicalSearchData{Results: hits, Matched: 3, Excluded: []string{"test"}, BodiesNotIndexed: 2}))
	stubBatch(t, client, `{"entities":[`+titled(drone1, "pump one")+`,`+titled(drone2, "pump two")+`,`+titled(drone3, "pump three")+`]}`)
	wordRequests := spyOnDownstream(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject)
	batchRequests := spyOnDownstream(t, client, verbRow(t, graph.ResponderGraphIngest, "batch").Subject)

	raw, err := search(t, client, `{"entity_id":"`+drone1+`","query":"pump -test","limit":3,"min_revision":5}`)
	envelope, _ := searchSuccess(t, raw, err)
	if envelope.IndexedRevision != 42 {
		t.Errorf("indexed_revision = %d, want the word search's 42", envelope.IndexedRevision)
	}
	requireSameJSON(t, envelope.Data, `{"community_id":"c360.test.query.system","level":0,"record_state":"not_ready","matches":[`+
		`{"id":"`+drone3+`","label":"pump three","score":3.5,"entity":`+titled(drone3, "pump three")+`},`+
		`{"id":"`+drone1+`","label":"pump one","score":2.25,"entity":`+titled(drone1, "pump one")+`},`+
		`{"id":"`+drone2+`","label":"pump two","score":1,"entity":`+titled(drone2, "pump two")+`}],`+
		`"matched":3,"excluded":["test"],"missing":0,"bodies_not_indexed":2,"truncated":false,`+
		`"answer":"Found 3 matching entities in c360.test.query.system."}`)
	if got := wordRequests(); len(got) != 1 {
		t.Errorf("word search requests = %q, want one", got)
	} else {
		requireSameJSON(t, []byte(got[0]), `{"query":"pump -test","limit":3,"scope":["c360.test.query.system"],"min_revision":5}`)
	}
	if got := batchRequests(); len(got) != 1 {
		t.Errorf("batch requests = %q, want one", got)
	} else {
		requireSameJSON(t, []byte(got[0]), `{"ids":["`+drone3+`","`+drone1+`","`+drone2+`"]}`)
	}
}

// TestSearchDropsMissingHits: a hit the batch names missing is left out, counted and declared; one
// it neither sends nor names is graph-query's own refusal, counted, since answering would drop it
// without saying so; with no hits no batch request is made (spec, "Order and missing hits").
func TestSearchDropsMissingHits(t *testing.T) {
	client := fixtureClient(t)
	c := startComponent(t, client, `{}`)
	var wordReply, batch []byte
	serve(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject, func(context.Context, []byte) ([]byte, error) {
		return wordReply, nil
	})
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "batch").Subject, func(context.Context, []byte) ([]byte, error) {
		return batch, nil
	})
	const request = `{"entity_id":"` + drone1 + `","query":"pump"}`
	hits := []graph.LexicalHit{{EntityID: drone3, Score: 3}, {EntityID: drone2, Score: 2}, {EntityID: drone1, Score: 1}}
	wordReply = lexicalReply(t, 4, graph.LexicalSearchData{Results: hits, Matched: 3})

	batch = []byte(`{"entities":[` + titled(drone1, "one") + `,` + titled(drone3, "three") + `],"missing":[{"id":"` + drone2 + `","reason":"not_found"}]}`)
	raw, err := search(t, client, request)
	_, data := searchSuccess(t, raw, err)
	if len(data.Matches) != 2 || data.Matches[0].ID != drone3 || data.Matches[1].ID != drone1 || data.Missing != 1 || !data.Truncated ||
		data.Answer != "Found 3 matching entities in c360.test.query.system. Showing the first 2." {
		t.Errorf("matches %v, missing %d, truncated %v, answer %q; want drone3 then drone1, one missing, truncated, two shown",
			matchIDs(data.Matches), data.Missing, data.Truncated, data.Answer)
	}
	if got := c.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0: a hit gone since the search is not an error", got)
	}

	batch = []byte(`{"entities":[` + titled(drone1, "one") + `,` + titled(drone3, "three") + `]}`)
	raw, err = search(t, client, request)
	requireRefusal(t, raw, err, errs.ErrorTransient, "internal")
	if !strings.Contains(err.Error(), drone2) {
		t.Errorf("refusal %q does not name the unaccounted hit %s", err, drone2)
	}
	if got := c.Health().ErrorCount; got != 1 {
		t.Errorf("ErrorCount = %d, want 1", got)
	}

	wordReply = lexicalReply(t, 4, graph.LexicalSearchData{Matched: 0})
	batchRequests := spyOnDownstream(t, client, verbRow(t, graph.ResponderGraphIngest, "batch").Subject)
	raw, err = search(t, client, request)
	if _, data := searchSuccess(t, raw, err); data.Matches == nil || len(data.Matches) != 0 || data.Truncated {
		t.Errorf("no hits answered %+v, want an empty matches list, not truncated", data)
	}
	if got := batchRequests(); len(got) != 0 {
		t.Errorf("batch requests = %q, want none for no hits", got)
	}
}

// TestLocalSearchEntitiesPassUnchanged: a matching entity holding values graph-query does not read, an
// integer above 2^53 and a key it does not know, reaches the caller as the batch sent it (spec,
// "Entities pass unchanged"). A stub sends the batch: real graph-ingest decodes numbers itself.
func TestLocalSearchEntitiesPassUnchanged(t *testing.T) {
	client := fixtureClient(t)
	startComponent(t, client, `{}`)
	const row = `{"id":"c360.test.query.system.drone.001","triples":[{"subject":"c360.test.query.system.drone.001","predicate":"test.reading.count","object":9007199254740993}],"unknown":{"kept":[1.50,2e3]}}`
	stubWordIndex(t, client, lexicalReply(t, 1, graph.LexicalSearchData{Results: []graph.LexicalHit{{EntityID: drone1, Score: 1}}, Matched: 1}))
	stubBatch(t, client, `{"entities":[`+row+`]}`)

	raw, err := search(t, client, `{"entity_id":"`+drone1+`","query":"drone"}`)
	_, data := searchSuccess(t, raw, err)
	if len(data.Matches) != 1 || string(data.Matches[0].Entity) != row {
		t.Errorf("matches = %s, want exactly [%s]", envelopeData(raw), row)
	}
}

// TestLocalSearchFitsMaxPayload: on a broker whose max_payload the batch reply fits but the search
// reply would not, the lowest-ranked matches are left out until the reply fits, truncated is set and
// the answer says how many are shown (spec, "A reply over the payload limit").
func TestLocalSearchFitsMaxPayload(t *testing.T) {
	const limit = 8192
	// Own server: a small max_payload, which the shared server does not have.
	f := natsfixture.New(t, natsfixture.WithMaxPayload(limit))
	if err := f.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	client := natsfixture.Open(t, f.URL(), openClient)
	registry := metric.NewMetricsRegistry()
	startMetered(t, client, registry)

	// Rows until the batch reply is just under the limit; the hits come in descending ID order, so
	// the matches' order is the hits' and not the batch's.
	var rows []string
	var hits []graph.LexicalHit
	batch := func(rows []string) string { return `{"entities":[` + strings.Join(rows, ",") + `]}` }
	for i := 0; ; i++ {
		id := fmt.Sprintf("c360.test.query.system.drone.%03d", i)
		row := titled(id, strings.Repeat("x", 150))
		if len(batch(append(rows, row))) > limit-64 {
			break
		}
		rows = append(rows, row)
		hits = append([]graph.LexicalHit{{EntityID: id, Score: float64(i + 1)}}, hits...)
	}
	stubWordIndex(t, client, lexicalReply(t, 1, graph.LexicalSearchData{Results: hits, Matched: len(hits)}))
	stubBatch(t, client, batch(rows))

	raw, err := search(t, client, `{"entity_id":"`+drone1+`","query":"x","limit":100}`)
	_, data := searchSuccess(t, raw, err)
	if len(raw) > limit {
		t.Errorf("reply of %d bytes, over max_payload %d", len(raw), limit)
	}
	kept := len(data.Matches)
	if !data.Truncated || kept == 0 || kept >= len(hits) {
		t.Fatalf("kept %d of %d matches, truncated %v; want some left out and the flag set", kept, len(hits), data.Truncated)
	}
	for i, match := range data.Matches {
		if match.ID != hits[i].EntityID || string(match.Entity) != rows[len(rows)-1-i] {
			t.Fatalf("matches[%d] = %s %s, want the hit %s", i, match.ID, match.Entity, hits[i].EntityID)
		}
	}
	// Only the fit cut the answer; the reply returned after the handler counted.
	if got := truncatedCount(t, registry.PrometheusRegistry(), "localSearch"); got != 1 {
		t.Errorf("localSearch truncated count = %v, want 1", got)
	}
	want := fmt.Sprintf("Found %d matching entities in c360.test.query.system. Showing the first %d.", len(hits), kept)
	if data.Answer != want {
		t.Errorf("answer = %q, want %q", data.Answer, want)
	}
}

// TestLocalSearchBeforeCommunityIndexExists: with no COMMUNITY_INDEX, Start succeeds, localSearch
// answers with record_state not_ready, the other verbs answer, and Health counts no error (spec,
// "Before the bucket exists"). Nothing in this test creates the bucket, so no wait can make it ready.
func TestLocalSearchBeforeCommunityIndexExists(t *testing.T) {
	client := fixtureClient(t)
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "entity").Subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"entity":{"id":"` + drone1 + `"},"kv_revision":5}`), nil
	})
	stubWordIndex(t, client, lexicalReply(t, 5, graph.LexicalSearchData{}))
	c := startComponent(t, client, `{}`)

	raw, err := search(t, client, `{"entity_id":"`+drone1+`","query":"drone"}`)
	if _, data := searchSuccess(t, raw, err); data.RecordState != graph.RecordNotReady || data.Record != nil {
		t.Errorf("data = %+v, want record_state not_ready and no record", data)
	}
	if _, err := client.RequestClassified(t.Context(), "graph.query.entity", []byte(`{"id":"`+drone1+`"}`), requestTimeout); err != nil {
		t.Errorf("entity beside a missing COMMUNITY_INDEX: %v", err)
	}
	if health := c.Health(); !health.Healthy || health.ErrorCount != 0 {
		t.Errorf("Health = %+v, want healthy with no error", health)
	}
}

// TestLocalSearchAfterCommunityIndexAppears: a COMMUNITY_INDEX created and published after Start is
// attached without a restart, within one reopen interval, with no GRAPH_STATUS key for
// graph-clustering and no COMMUNITY_SUMMARIES (spec, "The bucket appears", "Readiness comes from the
// view alone"; ADR-087 Decision 5).
func TestLocalSearchAfterCommunityIndexAppears(t *testing.T) {
	client := fixtureClient(t)
	stubWordIndex(t, client, lexicalReply(t, 1, graph.LexicalSearchData{}))
	startComponent(t, client, `{}`)
	request := `{"entity_id":"` + drone1 + `","query":"drone"}`
	raw, err := search(t, client, request)
	if _, data := searchSuccess(t, raw, err); data.RecordState != graph.RecordNotReady {
		t.Fatalf("before the bucket: record_state %q, want not_ready", data.RecordState)
	}

	_, m := communityRecords(t, client)
	publishDrones(t, m, 1, drone1)
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	for _, bucket := range []string{graph.BucketCommunitySummaries, "GRAPH_STATUS"} {
		if _, err := js.KeyValue(t.Context(), bucket); !errors.Is(err, jetstream.ErrBucketNotFound) {
			t.Fatalf("%s: %v, want no such bucket", bucket, err)
		}
	}
	_, data := awaitAnswer(t, client, request, func(d graph.LocalSearchData) bool { return d.RecordState != graph.RecordNotReady })
	if data.RecordState != graph.RecordRead || data.CommunityID != systemGroup || data.Record == nil || data.Record.MemberCount != 1 {
		t.Errorf("data = %+v, want drone1's group record read", data)
	}
}

// TestLocalSearchAfterWatchLoss: COMMUNITY_INDEX deleted under a live view answers with record_state
// not_ready, never with the old projection's record, and attaches the new bucket's record once it is
// recreated and published, twice over (spec, "A lost watch"; ADR-081). Each new bucket's group holds
// a different number of members, so a record from an old projection would show.
func TestLocalSearchAfterWatchLoss(t *testing.T) {
	client := fixtureClient(t)
	stubWordIndex(t, client, lexicalReply(t, 1, graph.LexicalSearchData{}))
	_, m := communityRecords(t, client)
	publishDrones(t, m, 1, drone1)
	c := startComponent(t, client, `{}`)
	awaitCommunityView(t, c, systemGroupKey, 1)
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	const request = `{"entity_id":"` + drone1 + `","query":"drone"}`

	for round, members := range [][]string{{drone1, drone2}, {drone1, drone2, drone3}} {
		if err := js.DeleteKeyValue(t.Context(), graph.BucketCommunityIndex); err != nil {
			t.Fatalf("round %d: delete: %v", round, err)
		}
		// Each answer with the old record is the old view, until its loss is seen.
		awaitAnswer(t, client, request, func(d graph.LocalSearchData) bool { return d.RecordState == graph.RecordNotReady })
		raw, err := search(t, client, request)
		if _, data := searchSuccess(t, raw, err); data.RecordState != graph.RecordNotReady {
			t.Errorf("round %d: after the loss, record_state %q, want not_ready", round, data.RecordState)
		}

		_, m := communityRecords(t, client)
		revision := uint64(10 * (round + 1))
		publishDrones(t, m, revision, members...)
		_, data := awaitAnswer(t, client, request, func(d graph.LocalSearchData) bool { return d.RecordState == graph.RecordRead })
		if data.Record == nil || data.Record.MemberCount != len(members) {
			t.Errorf("round %d: record %+v, want the new bucket's, %d members", round, data.Record, len(members))
		}
	}
}

// TestLocalSearchDuringBlockedTick: with one group's next record written and another's not, as a
// tick held between two writes leaves the bucket, each group answers at once with its latest record
// (spec, "During a tick"). Only clustering's fake holds a real tick, so the next record is written
// straight to the bucket.
func TestLocalSearchDuringBlockedTick(t *testing.T) {
	client := fixtureClient(t)
	stubWordIndex(t, client, lexicalReply(t, 1, graph.LexicalSearchData{}))
	kv, m := communityRecords(t, client)
	publishDrones(t, m, 1, drone1, otherDrone)
	c := startComponent(t, client, `{}`)
	awaitCommunityView(t, c, systemGroupKey, 1)
	awaitCommunityView(t, c, "0.c360.test.query.other", 2)

	next := `{"id":"c360.test.query.system","level":0,"member_count":7,"entity_revision":30,"type_counts":{"drone":7},"statistical_summary":"the next record."}`
	if _, err := kv.Put(t.Context(), systemGroupKey, []byte(next)); err != nil {
		t.Fatal(err)
	}
	// Each answer with the old record is the event, until the view has applied the put; none may
	// lack a record.
	_, data := awaitAnswer(t, client, `{"entity_id":"`+drone1+`","query":"drone"}`, func(d graph.LocalSearchData) bool {
		if d.RecordState != graph.RecordRead {
			t.Errorf("drone1 during the tick answered record_state %q", d.RecordState)
			return true
		}
		return d.Record.MemberCount == 7
	})
	if data.Record == nil || data.Record.CommunitySummary != "the next record." {
		t.Errorf("drone1's record = %+v, want the next record", data.Record)
	}
	raw, err := search(t, client, `{"entity_id":"`+otherDrone+`","query":"drone"}`)
	_, data = searchSuccess(t, raw, err)
	if data.RecordState != graph.RecordRead || data.Record.MemberCount != 1 || data.CommunityID != "c360.test.query.other" {
		t.Errorf("the other group answered %+v, want its old record", data)
	}
}

// TestStopEndsTheCommunityView: Stop returns only after the view's Run has returned, whether a view is
// running or the supervisor is waiting to open the bucket again (spec, "Stop"). The view is withdrawn
// only once its Run has returned, so none held after Stop says Run returned before Stop did.
func TestStopEndsTheCommunityView(t *testing.T) {
	t.Run("a view running", func(t *testing.T) {
		client := fixtureClient(t)
		withCommunityRecord(t, client)
		c := newComponent(t, client)
		if err := c.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		awaitCommunityView(t, c, systemGroupKey, 1)
		if c.communities.Load() == nil {
			t.Fatal("no view held while it answers")
		}
		// Errorf, not Fatalf: a Stop that reports a watch it could not stop releases everything,
		// and the next Stop returns nil.
		if err := c.Stop(t.Context()); err != nil {
			t.Errorf("Stop: %v", err)
		}
		if c.communities.Load() != nil || c.supervised != nil {
			t.Error("a view or its supervisor is held after Stop returned")
		}
		if err := c.Stop(t.Context()); err != nil {
			t.Errorf("second Stop: %v", err)
		}
	})
	t.Run("waiting to reopen", func(t *testing.T) {
		client := fixtureClient(t)
		c, logs := newLogged(t, client)
		if err := c.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		// The supervisor logs the missing bucket just before it waits communityRecheck to reopen.
		logs.await(t, 1)
		// Own deadline: the bound is the behaviour under test; a Stop that waited out the reopen
		// timer would take communityRecheck and miss it.
		ctx, cancel := context.WithTimeout(t.Context(), communityRecheck/2)
		defer cancel()
		if err := c.Stop(ctx); err != nil {
			t.Fatalf("Stop during the reopen wait: %v", err)
		}
		if c.communities.Load() != nil || c.supervised != nil {
			t.Error("a view or its supervisor is held after Stop returned")
		}
	})
}

// supervisorLog is one record the community supervisor logged: its level and its error's text.
type supervisorLog struct {
	level slog.Level
	err   string
}

// logCapture is a slog.Handler keeping the community supervisor's records, at every level.
type logCapture struct {
	mu      sync.Mutex
	records []supervisorLog
}

func (l *logCapture) Enabled(context.Context, slog.Level) bool { return true }
func (l *logCapture) WithAttrs([]slog.Attr) slog.Handler       { return l }
func (l *logCapture) WithGroup(string) slog.Handler            { return l }

func (l *logCapture) Handle(_ context.Context, r slog.Record) error {
	if r.Message != communityUnavailable {
		return nil
	}
	record := supervisorLog{level: r.Level}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "error" {
			record.err = a.Value.String()
		}
		return true
	})
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, record)
	return nil
}

// await waits until the supervisor has logged n records, and returns them.
func (l *logCapture) await(t *testing.T, n int) []supervisorLog {
	t.Helper()
	records, err := probe.Await(t.Context(), func(context.Context) ([]supervisorLog, error) {
		l.mu.Lock()
		defer l.mu.Unlock()
		return append([]supervisorLog(nil), l.records...), nil
	}, func(records []supervisorLog) bool { return len(records) >= n })
	if err != nil {
		t.Fatalf("await %d supervisor records: %v", n, err)
	}
	return records
}

// newLogged builds and initializes graph-query with a logger that captures its supervisor's records.
func newLogged(t *testing.T, client *natsclient.Client) (*Component, *logCapture) {
	t.Helper()
	logs := &logCapture{}
	built, err := CreateGraphQuery(json.RawMessage(`{}`), component.Dependencies{NATSClient: client, Logger: slog.New(logs)})
	if err != nil {
		t.Fatal(err)
	}
	c := built.(*Component)
	if err := c.Initialize(); err != nil {
		t.Fatal(err)
	}
	return c, logs
}

// TestCommunityViewLogsEachNewFailureOnce: the supervisor warns on its first failure, logs the same
// failure again at Debug, and warns again when the failure changes or follows a view that opened,
// so a deployment with no COMMUNITY_INDEX writer is not one Warn every 5 s.
func TestCommunityViewLogsEachNewFailureOnce(t *testing.T) {
	client := fixtureClient(t)
	c, logs := newLogged(t, client)
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := c.Stop(ctx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
	const missing = "is not ready"
	records := logs.await(t, 2) // the second comes one reopen interval after the first
	if records[0].level != slog.LevelWarn || !strings.Contains(records[0].err, missing) {
		t.Fatalf("first record = %+v, want a Warn that the bucket is missing", records[0])
	}
	if records[1].level != slog.LevelDebug || records[1].err != records[0].err {
		t.Fatalf("second record = %+v, want the same failure at Debug", records[1])
	}

	_, m := communityRecords(t, client)
	publishDrones(t, m, 1, drone1)
	awaitCommunityView(t, c, systemGroupKey, 1)
	opened := len(logs.await(t, 0))
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if err := js.DeleteKeyValue(t.Context(), graph.BucketCommunityIndex); err != nil {
		t.Fatal(err)
	}
	records = logs.await(t, opened+1)
	if lost := records[opened]; lost.level != slog.LevelWarn || !strings.Contains(lost.err, "watcher lost") {
		t.Errorf("record after the view opened = %+v, want a Warn that its watch was lost", lost)
	}
}
