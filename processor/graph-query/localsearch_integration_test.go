//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

const (
	localSearchSubject = "graph.query.localSearch"
	// systemGroup is drone1's level-0 group, and its record's key.
	systemGroup    = "c360.test.query.system"
	systemGroupKey = "0." + systemGroup
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

// awaitSearch sends request until localSearch stops refusing index_not_ready, each refusal being the
// event, and returns the first other outcome. The view opens and catches up on its own goroutine;
// go test -timeout ends a view that never does.
func awaitSearch(t *testing.T, client *natsclient.Client, request string) ([]byte, error) {
	t.Helper()
	got, err := probe.Await(t.Context(), func(ctx context.Context) (searchOutcome, error) {
		raw, err := client.RequestClassified(ctx, localSearchSubject, []byte(request), requestTimeout)
		return searchOutcome{raw, err}, nil
	}, func(o searchOutcome) bool { return codeOf(o.err) != graph.ErrorCodeIndexNotReady })
	if err != nil {
		t.Fatalf("await localSearch %s: %v", request, err)
	}
	return got.raw, got.err
}

// awaitCommunityView waits until the view holds id's group record at entity_revision revision: a
// request far ahead of it is refused revision_not_reached naming that revision. That refusal comes
// before any downstream request, and counts no error.
func awaitCommunityView(t *testing.T, client *natsclient.Client, id string, revision uint64) {
	t.Helper()
	request := fmt.Sprintf(`{"entity_id":%q,"query":"drone","min_revision":%d}`, id, farAhead)
	got, err := probe.Await(t.Context(), func(ctx context.Context) (searchOutcome, error) {
		raw, err := client.RequestClassified(ctx, localSearchSubject, []byte(request), requestTimeout)
		return searchOutcome{raw, err}, nil
	}, func(o searchOutcome) bool {
		var refusal *errs.ClassifiedError
		return errors.As(o.err, &refusal) && refusal.Code == graph.ErrorCodeRevisionNotReached &&
			refusal.Detail["indexed_revision"] == float64(revision)
	})
	if err != nil {
		t.Fatalf("await the community view at %d: %v (last %v)", revision, err, got.err)
	}
}

// withCommunityRecord publishes drone1's group record at revision 1 before graph-query starts, then
// waits for the view, so localSearch's first downstream request is its page read.
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

// recordJSON is the reply's record fields for record, from the bucket's own bytes.
func recordJSON(t *testing.T, record clustering.Community) string {
	t.Helper()
	keywords, err := json.Marshal(record.Keywords)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := json.Marshal(record.StatisticalSummary)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf(`"community_id":%q,"level":%d,"member_count":%d,"community_summary":%s,"keywords":%s`,
		record.ID, record.Level, record.MemberCount, summary, keywords)
}

// communityStack is real graph-ingest and graph-index behind graph-query, with COMMUNITY_INDEX
// created before graph-query starts and a real maintainer the test feeds.
type communityStack struct {
	graphStack
	component  *Component
	records    jetstream.KeyValue
	maintainer *clustering.Maintainer
}

func startCommunityStack(t *testing.T) communityStack {
	t.Helper()
	stack := startIngestAndIndex(t)
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

// pageRows is each entity of prefix's first page, by ID, as graph-ingest sends it.
func pageRows(t *testing.T, client *natsclient.Client, prefix string) map[string]string {
	t.Helper()
	raw, err := client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphIngest, "prefix").Subject,
		fmt.Appendf(nil, `{"prefix":%q,"limit":%d}`, prefix, graph.MaxPrefixQueryLimit), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Entities []json.RawMessage `json:"entities"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	rows := map[string]string{}
	for _, row := range page.Entities {
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

// TestLocalSearchReadsOnePage: beside real graph-ingest, a request with a record is one prefix request
// for the record's group at the maximum limit, and no entity read (ruling 2; spec, "A group larger
// than a page", "A group that fits a page", "The reply's fields").
func TestLocalSearchReadsOnePage(t *testing.T) {
	stack := startCommunityStack(t)
	title := func(text string) [2]string { return [2]string{"dc.terms.title", text} }
	// Every write comes before the spies: write reads each entity back, which a spy would hold.
	stack.write(t, drone1, title("alpha survey"))
	stack.write(t, drone2, title("beta survey"))
	small := stack.write(t, drone3, title("alpha"))
	const bulk = "c360.test.query.bulk"
	for i := range graph.MaxPrefixQueryLimit {
		stack.write(t, fmt.Sprintf("%s.drone.%04d", bulk, i))
	}
	// The bulk group's only match sorts after every other ID, so past the first page.
	last := fmt.Sprintf("%s.drone.%04d", bulk, graph.MaxPrefixQueryLimit)
	stack.write(t, last, title("zebra"))
	tick(t, stack.maintainer)
	rows := pageRows(t, stack.client, systemGroup)
	prefixRequests := spyOnPrefixVerb(t, stack.client)
	entityRequests := spyOnDownstream(t, stack.client, verbRow(t, graph.ResponderGraphIngest, "entity").Subject)

	t.Run("a group that fits a page", func(t *testing.T) {
		record := storedRecord(t, stack.records, systemGroupKey)
		raw, err := awaitSearch(t, stack.client, fmt.Sprintf(`{"entity_id":%q,"query":"Alpha"}`, drone2))
		envelope, _ := searchSuccess(t, raw, err)
		if envelope.IndexedRevision != small {
			t.Errorf("indexed_revision = %d, want the record's %d", envelope.IndexedRevision, small)
		}
		// The independent filter: the titles holding "alpha", by ID.
		requireSameJSON(t, envelope.Data, fmt.Sprintf(`{%s,"entities":[%s,%s],"entities_scanned":3,"entities_truncated":false,"answer":%q}`,
			recordJSON(t, record), rows[drone1], rows[drone3],
			"Found 2 matching entities among 3 read from c360.test.query.system. "+record.StatisticalSummary))
		// awaitSearch may have been refused before the view caught up, which reads nothing.
		if got := prefixRequests(); len(got) != 1 {
			t.Fatalf("prefix requests = %q, want one", got)
		} else {
			requireSameJSON(t, []byte(got[0]), `{"prefix":"c360.test.query.system","limit":1000}`)
		}
		if got := entityRequests(); len(got) != 0 {
			t.Errorf("entity requests = %q, want none: the record was there", got)
		}
	})

	t.Run("a group larger than a page", func(t *testing.T) {
		record := storedRecord(t, stack.records, "0."+bulk)
		raw, err := search(t, stack.client, fmt.Sprintf(`{"entity_id":%q,"query":"zebra"}`, last))
		envelope, _ := searchSuccess(t, raw, err)
		requireSameJSON(t, envelope.Data, fmt.Sprintf(`{%s,"entities":[],"entities_scanned":1000,"entities_truncated":true,"answer":%q}`,
			recordJSON(t, record), "Found 0 matching entities among 1000 read from c360.test.query.bulk. "+record.StatisticalSummary))
		if got := prefixRequests(); len(got) != 1 {
			t.Fatalf("prefix requests = %d, want one", len(got))
		}
		if got := entityRequests(); len(got) != 0 {
			t.Errorf("entity requests = %q, want none", got)
		}
	})
}

// TestLocalSearchReadsTheAskedLevel: an entity's groups at levels 0 and 1 both have records, and each
// request answers from the level it asked (spec, "The asked level"; ADR-085).
func TestLocalSearchReadsTheAskedLevel(t *testing.T) {
	stack := startCommunityStack(t)
	const elsewhere = "c360.test.query.other.drone.001"
	stack.write(t, drone1)
	stack.write(t, elsewhere)
	tick(t, stack.maintainer)

	for _, tc := range []struct {
		request, community string
		level, members     int
		entities           []string
	}{
		{fmt.Sprintf(`{"entity_id":%q,"query":"drone"}`, drone1), systemGroup, 0, 1, []string{drone1}},
		{fmt.Sprintf(`{"entity_id":%q,"query":"drone","level":1}`, drone1), "c360.test.query", 1, 2, []string{elsewhere, drone1}},
		{fmt.Sprintf(`{"entity_id":%q,"query":"drone","level":2}`, drone1), "c360.test", 2, 2, []string{elsewhere, drone1}},
	} {
		raw, err := awaitSearch(t, stack.client, tc.request)
		_, data := searchSuccess(t, raw, err)
		var ids []string
		for _, entity := range data.Entities {
			var row struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(entity, &row); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, row.ID)
		}
		if data.CommunityID != tc.community || data.Level != tc.level || data.MemberCount != tc.members ||
			strings.Join(ids, " ") != strings.Join(tc.entities, " ") {
			t.Errorf("%s: community %q level %d members %d entities %v, want %q %d %d %v",
				tc.request, data.CommunityID, data.Level, data.MemberCount, ids, tc.community, tc.level, tc.members, tc.entities)
		}
	}
}

// TestLocalSearchReadsItsWriteBack: a write W to an entity of the group is refused revision_not_reached
// with the record's revision until the group's next tick, then answers at or above W (spec, "Reading
// a write back through a group"; design D2).
func TestLocalSearchReadsItsWriteBack(t *testing.T) {
	stack := startCommunityStack(t)
	before := stack.write(t, drone1)
	tick(t, stack.maintainer)
	if _, err := awaitSearch(t, stack.client, fmt.Sprintf(`{"entity_id":%q,"query":"drone","min_revision":%d}`, drone1, before)); err != nil {
		t.Fatalf("the record's own revision: %v", err)
	}

	written := stack.write(t, drone2)
	request := fmt.Sprintf(`{"entity_id":%q,"query":"drone","min_revision":%d}`, drone2, written)
	// The maintainer has drone2, but no tick has published it: the record is still at before.
	raw, err := search(t, stack.client, request)
	if reached := indexedRevisionOf(t, raw, err); reached != before {
		t.Errorf("refusal before the tick names indexed_revision %d, want the record's %d", reached, before)
	}

	tick(t, stack.maintainer)
	// The view applies the tick's write on its own goroutine: each refusal until then is the event.
	got, err := probe.Await(t.Context(), func(ctx context.Context) (searchOutcome, error) {
		raw, err := stack.client.RequestClassified(ctx, localSearchSubject, []byte(request), requestTimeout)
		return searchOutcome{raw, err}, nil
	}, func(o searchOutcome) bool { return codeOf(o.err) != graph.ErrorCodeRevisionNotReached })
	if err != nil {
		t.Fatal(err)
	}
	envelope, data := searchSuccess(t, got.raw, got.err)
	if envelope.IndexedRevision < written {
		t.Errorf("indexed_revision = %d, below the write's %d", envelope.IndexedRevision, written)
	}
	if !strings.Contains(string(envelope.Data), drone2) || data.MemberCount != 2 {
		t.Errorf("data = %s, want drone2 among two members", envelope.Data)
	}
}

// TestLocalSearchRefusals: with no record an existing entity is index_not_ready after one entity read,
// an absent one gets the entity verb's refusal as it arrived; an unreadable record is refused
// internal naming its key, counted in Health, until a valid write heals it (spec, "No record", "An
// unreadable record"; design D6).
func TestLocalSearchRefusals(t *testing.T) {
	stack := startCommunityStack(t)
	stack.write(t, drone1)
	tick(t, stack.maintainer)
	// The view is caught up from here on: it answered.
	if _, err := awaitSearch(t, stack.client, fmt.Sprintf(`{"entity_id":%q,"query":"drone"}`, drone1)); err != nil {
		t.Fatal(err)
	}
	entityRequests := spyOnDownstream(t, stack.client, verbRow(t, graph.ResponderGraphIngest, "entity").Subject)

	t.Run("no record, the entity exists", func(t *testing.T) {
		const unpublished = "c360.test.query.unpublished.drone.001"
		stack.create(t, unpublished)
		errorsBefore := stack.component.Health().ErrorCount
		raw, err := search(t, stack.client, fmt.Sprintf(`{"entity_id":%q,"query":"drone"}`, unpublished))
		requireRefusal(t, raw, err, errs.ErrorTransient, "index_not_ready")
		if got := entityRequests(); len(got) != 1 {
			t.Errorf("entity requests = %q, want one", got)
		} else {
			requireSameJSON(t, []byte(got[0]), `{"id":"`+unpublished+`"}`)
		}
		if got := stack.component.Health().ErrorCount; got != errorsBefore {
			t.Errorf("ErrorCount = %d, want %d: a group with no record is not an error", got, errorsBefore)
		}
	})

	t.Run("no record, the entity absent", func(t *testing.T) {
		const absent = "c360.test.query.absent.drone.001"
		_, direct := stack.client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphIngest, "entity").Subject,
			[]byte(`{"id":"`+absent+`"}`), requestTimeout)
		want := refusalOf(t, direct)
		raw, err := search(t, stack.client, fmt.Sprintf(`{"entity_id":%q,"query":"drone"}`, absent))
		got := refusalOf(t, err)
		if got.Class != want.Class || got.Code != want.Code || err.Error() != direct.Error() {
			t.Errorf("refusal = %v %q %q (%s), want the entity verb's %v %q %q", got.Class, got.Code, err, raw, want.Class, want.Code, direct)
		}
		entityRequests()
	})

	t.Run("an unreadable record", func(t *testing.T) {
		const poisoned, key = "c360.test.query.poison.drone.001", "0.c360.test.query.poison"
		if _, err := stack.records.Put(t.Context(), key, []byte(`{"id":"c360.test.query.poison","level":0,"member_count":1}`)); err != nil {
			t.Fatal(err)
		}
		request := fmt.Sprintf(`{"entity_id":%q,"query":"drone"}`, poisoned)
		// Until the view applies the put, the key is absent and the entity too: each entity_not_found
		// is the event.
		got, err := probe.Await(t.Context(), func(ctx context.Context) (searchOutcome, error) {
			raw, err := stack.client.RequestClassified(ctx, localSearchSubject, []byte(request), requestTimeout)
			return searchOutcome{raw, err}, nil
		}, func(o searchOutcome) bool { return codeOf(o.err) != graph.ErrorCodeEntityNotFound })
		if err != nil {
			t.Fatal(err)
		}
		requireRefusal(t, got.raw, got.err, errs.ErrorTransient, "internal")
		if !strings.Contains(got.err.Error(), key) {
			t.Errorf("refusal %q does not name %s", got.err, key)
		}
		errorsBefore := stack.component.Health().ErrorCount
		raw, err := search(t, stack.client, request)
		requireRefusal(t, raw, err, errs.ErrorTransient, "internal")
		if got := stack.component.Health().ErrorCount; got != errorsBefore+1 {
			t.Errorf("ErrorCount = %d after one unreadable record, want %d", got, errorsBefore+1)
		}
		entityRequests()

		// The group's maintainer writes its record over the poison.
		stack.write(t, poisoned)
		tick(t, stack.maintainer)
		healed, err := probe.Await(t.Context(), func(ctx context.Context) (searchOutcome, error) {
			raw, err := stack.client.RequestClassified(ctx, localSearchSubject, []byte(request), requestTimeout)
			return searchOutcome{raw, err}, nil
		}, func(o searchOutcome) bool { return codeOf(o.err) != graph.ErrorCodeInternal })
		if err != nil {
			t.Fatal(err)
		}
		if _, data := searchSuccess(t, healed.raw, healed.err); data.CommunityID != "c360.test.query.poison" {
			t.Errorf("healed answer from %q, want c360.test.query.poison", data.CommunityID)
		}
	})
}

// TestLocalSearchRefusesBadRequests: with the view ready, each bad request is invalid_request naming
// its field, with no downstream request and no error counted (spec, "A bad request"; design D4).
func TestLocalSearchRefusesBadRequests(t *testing.T) {
	client := fixtureClient(t)
	withCommunityRecord(t, client)
	c := startComponent(t, client, `{}`)
	awaitCommunityView(t, client, drone1, 1)
	// Stubs answer the spies' markers; a bad request reaches neither.
	for _, name := range []string{"entity", "prefix"} {
		serve(t, client, verbRow(t, graph.ResponderGraphIngest, name).Subject, func(context.Context, []byte) ([]byte, error) {
			return nil, errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, errors.New("a marker"))
		})
	}
	entityRequests := spyOnDownstream(t, client, verbRow(t, graph.ResponderGraphIngest, "entity").Subject)
	prefixRequests := spyOnPrefixVerb(t, client)

	for _, tc := range []struct{ name, request, says string }{
		{"an unknown key", `{"entity_id":"` + drone1 + `","query":"drone","limit":5}`, "limit"},
		{"a five-part entity_id", `{"entity_id":"c360.test.query.system.drone","query":"drone"}`, "entity_id"},
		{"level 3", `{"entity_id":"` + drone1 + `","query":"drone","level":3}`, "level"},
		{"an empty query", `{"entity_id":"` + drone1 + `","query":"  "}`, "query"},
		{"seventeen terms", `{"entity_id":"` + drone1 + `","query":"a b c d e f g h i j k l m n o p q"}`, "query"},
		{"not JSON", `not json`, "invalid character"},
	} {
		raw, err := search(t, client, tc.request)
		requireRefusal(t, raw, err, errs.ErrorInvalid, "invalid_request")
		if !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%s: refusal %q does not name %q", tc.name, err, tc.says)
		}
	}
	if got := entityRequests(); len(got) != 0 {
		t.Errorf("entity requests = %q, want none", got)
	}
	if got := prefixRequests(); len(got) != 0 {
		t.Errorf("prefix requests = %q, want none", got)
	}
	if got := c.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0", got)
	}
}

// TestLocalSearchEntitiesPassUnchanged: a matching entity holding values graph-query does not read, an
// integer above 2^53 and a key it does not know, reaches the caller as the page sent it (spec,
// "Entities pass unchanged"). A stub sends the page: real graph-ingest decodes numbers itself.
func TestLocalSearchEntitiesPassUnchanged(t *testing.T) {
	client := fixtureClient(t)
	withCommunityRecord(t, client)
	startComponent(t, client, `{}`)
	awaitCommunityView(t, client, drone1, 1)
	const row = `{"id":"c360.test.query.system.drone.001","triples":[{"subject":"c360.test.query.system.drone.001","predicate":"test.reading.count","object":9007199254740993}],"unknown":{"kept":[1.50,2e3]}}`
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "prefix").Subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"entities":[` + row + `],"total":1}`), nil
	})

	raw, err := search(t, client, `{"entity_id":"`+drone1+`","query":"drone"}`)
	_, data := searchSuccess(t, raw, err)
	if len(data.Entities) != 1 || string(data.Entities[0]) != row {
		t.Errorf("entities = %s, want exactly [%s]", data.Entities, row)
	}
}

// TestLocalSearchFitsMaxPayload: on a broker whose max_payload a page fits but the reply would not,
// the lowest-ranked matches are left out until the reply fits, entities_truncated is set and the
// answer counts what was kept (spec, "A reply over the payload limit"; design D5).
func TestLocalSearchFitsMaxPayload(t *testing.T) {
	const limit = 8192
	// Own server: a small max_payload, which the shared server does not have.
	f := natsfixture.New(t, natsfixture.WithMaxPayload(limit))
	if err := f.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	client := natsfixture.Open(t, f.URL(), openClient)
	_, m := communityRecords(t, client)
	publishDrones(t, m, 1, drone1)
	startComponent(t, client, `{}`)
	awaitCommunityView(t, client, drone1, 1)

	// Rows until the page is just under the limit; every row matches, by its type.
	var rows []string
	page := func(rows []string) string {
		return `{"entities":[` + strings.Join(rows, ",") + `],"total":` + fmt.Sprint(len(rows)) + `}`
	}
	for i := 0; ; i++ {
		row := fmt.Sprintf(`{"id":"c360.test.query.system.drone.%03d","triples":[{"predicate":"dc.terms.title","object":%q}]}`, i, strings.Repeat("x", 150))
		if len(page(append(rows, row))) > limit-64 {
			break
		}
		rows = append(rows, row)
	}
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "prefix").Subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(page(rows)), nil
	})

	raw, err := search(t, client, `{"entity_id":"`+drone1+`","query":"drone"}`)
	_, data := searchSuccess(t, raw, err)
	if len(raw) > limit {
		t.Errorf("reply of %d bytes, over max_payload %d", len(raw), limit)
	}
	kept := len(data.Entities)
	if !data.EntitiesTruncated || kept == 0 || kept >= len(rows) {
		t.Fatalf("kept %d of %d matches, truncated %v; want some left out and the flag set", kept, len(rows), data.EntitiesTruncated)
	}
	for i, entity := range data.Entities {
		if string(entity) != rows[i] { // the rows' IDs ascend, and the scores tie
			t.Fatalf("entities[%d] = %s, want %s", i, entity, rows[i])
		}
	}
	want := fmt.Sprintf("Found %d matching entities among %d read from c360.test.query.system. Community of 1 entities including 1 drone. Key themes: drone.", kept, len(rows))
	if data.Answer != want {
		t.Errorf("answer = %q, want %q", data.Answer, want)
	}
}

// TestLocalSearchBeforeCommunityIndexExists: with no COMMUNITY_INDEX, Start succeeds, localSearch is
// refused index_not_ready, transient, the other verbs answer, and Health counts no error (spec,
// "Before the bucket exists"). Nothing in this test creates the bucket, so no wait can make it ready.
func TestLocalSearchBeforeCommunityIndexExists(t *testing.T) {
	client := fixtureClient(t)
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "entity").Subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"entity":{"id":"` + drone1 + `"},"kv_revision":5}`), nil
	})
	c := startComponent(t, client, `{}`)

	raw, err := search(t, client, `{"entity_id":"`+drone1+`","query":"drone"}`)
	requireRefusal(t, raw, err, errs.ErrorTransient, "index_not_ready")
	if _, err := client.RequestClassified(t.Context(), "graph.query.entity", []byte(`{"id":"`+drone1+`"}`), requestTimeout); err != nil {
		t.Errorf("entity beside a missing COMMUNITY_INDEX: %v", err)
	}
	if health := c.Health(); !health.Healthy || health.ErrorCount != 0 {
		t.Errorf("Health = %+v, want healthy with no error", health)
	}
}

// TestLocalSearchAfterCommunityIndexAppears: a COMMUNITY_INDEX created and published after Start is
// answered without a restart, within one reopen interval, with no GRAPH_STATUS key for
// graph-clustering and no COMMUNITY_SUMMARIES (spec, "The bucket appears", "Readiness comes from the
// view alone"; ADR-087 Decision 5).
func TestLocalSearchAfterCommunityIndexAppears(t *testing.T) {
	client := fixtureClient(t)
	startComponent(t, client, `{}`)
	request := `{"entity_id":"` + drone1 + `","query":"drone"}`
	raw, err := search(t, client, request)
	requireRefusal(t, raw, err, errs.ErrorTransient, "index_not_ready")

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
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "prefix").Subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"entities":[{"id":"` + drone1 + `","triples":[]}],"total":1}`), nil
	})
	raw, err = awaitSearch(t, client, request)
	if _, data := searchSuccess(t, raw, err); data.CommunityID != systemGroup || len(data.Entities) != 1 {
		t.Errorf("data = %+v, want drone1 from %s", data, systemGroup)
	}
}

// TestLocalSearchRefusesAfterWatchLoss: COMMUNITY_INDEX deleted under a live view is refused
// index_not_ready, never answered from the old projection, and answered again from the new bucket's
// records once it is recreated and published, twice over (spec, "A lost watch"; ADR-081).
func TestLocalSearchRefusesAfterWatchLoss(t *testing.T) {
	client := fixtureClient(t)
	_, m := communityRecords(t, client)
	publishDrones(t, m, 1, drone1)
	startComponent(t, client, `{}`)
	awaitCommunityView(t, client, drone1, 1)
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	request := fmt.Sprintf(`{"entity_id":%q,"query":"drone","min_revision":%d}`, drone1, farAhead)

	for round, revision := range []uint64{10, 20} {
		if err := js.DeleteKeyValue(t.Context(), graph.BucketCommunityIndex); err != nil {
			t.Fatalf("round %d: delete: %v", round, err)
		}
		// Each revision_not_reached naming the old record is the old view, until its loss is seen.
		if _, err := probe.Await(t.Context(), func(ctx context.Context) (searchOutcome, error) {
			raw, err := client.RequestClassified(ctx, localSearchSubject, []byte(request), requestTimeout)
			return searchOutcome{raw, err}, nil
		}, func(o searchOutcome) bool { return codeOf(o.err) == graph.ErrorCodeIndexNotReady }); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		raw, err := search(t, client, request)
		requireRefusal(t, raw, err, errs.ErrorTransient, "index_not_ready")

		_, m := communityRecords(t, client)
		publishDrones(t, m, revision, drone1)
		awaitCommunityView(t, client, drone1, revision)
	}
}

// TestLocalSearchDuringBlockedTick: with one group's next record written and another's not, as a tick
// held between two writes leaves the bucket, each group answers at once from its latest record (spec,
// "During a tick"; ruling 3). Only clustering's fake holds a real tick, so the next record is written
// straight to the bucket.
func TestLocalSearchDuringBlockedTick(t *testing.T) {
	client := fixtureClient(t)
	const elsewhere = "c360.test.query.other.drone.001"
	kv, m := communityRecords(t, client)
	publishDrones(t, m, 1, drone1, elsewhere)
	startComponent(t, client, `{}`)
	awaitCommunityView(t, client, drone1, 1)
	awaitCommunityView(t, client, elsewhere, 2)
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "prefix").Subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(`{"entities":[],"total":0}`), nil
	})

	next := `{"id":"c360.test.query.system","level":0,"member_count":7,"entity_revision":30,"type_counts":{"drone":7},"statistical_summary":"the next record."}`
	if _, err := kv.Put(t.Context(), systemGroupKey, []byte(next)); err != nil {
		t.Fatal(err)
	}
	// Each answer from the old record is the event, until the view has applied the put; none is a refusal.
	got, err := probe.Await(t.Context(), func(ctx context.Context) (searchOutcome, error) {
		raw, err := client.RequestClassified(ctx, localSearchSubject, []byte(`{"entity_id":"`+drone1+`","query":"drone"}`), requestTimeout)
		if err != nil {
			t.Errorf("drone1 during the tick refused: %v", err)
		}
		return searchOutcome{raw, err}, nil
	}, func(o searchOutcome) bool { return strings.Contains(string(o.raw), `"member_count":7`) })
	if err != nil {
		t.Fatal(err)
	}
	if envelope, _ := searchSuccess(t, got.raw, got.err); envelope.IndexedRevision != 30 {
		t.Errorf("drone1's indexed_revision = %d, want the next record's 30", envelope.IndexedRevision)
	}
	raw, err := search(t, client, `{"entity_id":"`+elsewhere+`","query":"drone"}`)
	envelope, data := searchSuccess(t, raw, err)
	if envelope.IndexedRevision != 2 || data.MemberCount != 1 || data.CommunityID != "c360.test.query.other" {
		t.Errorf("the other group answered %s at %d, want its old record at 2", envelope.Data, envelope.IndexedRevision)
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
		awaitCommunityView(t, client, drone1, 1)
		if c.communities.Load() == nil {
			t.Fatal("no view held while it answers")
		}
		if err := c.Stop(t.Context()); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		if c.communities.Load() != nil || c.supervised != nil {
			t.Error("a view or its supervisor is held after Stop returned")
		}
	})
	t.Run("waiting to reopen", func(t *testing.T) {
		client := fixtureClient(t)
		c := newComponent(t, client)
		if err := c.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := c.Stop(t.Context()); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		if c.communities.Load() != nil || c.supervised != nil {
			t.Error("a view or its supervisor is held after Stop returned")
		}
	})
}
