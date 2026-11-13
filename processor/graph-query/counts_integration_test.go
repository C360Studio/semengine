//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

const (
	hierarchyStatsSubject = "graph.query.hierarchyStats"
	countedGroup          = "c360.test.query.system"
)

// seedPagedGroup writes five entities under countedGroup, in four child prefixes. Each holds a fifth
// of max_payload: four fit a reply and five do not, so real graph-ingest ends its first page after
// gamma.001 and its second holds zeta.001.
func seedPagedGroup(t *testing.T, stack graphStack) {
	t.Helper()
	maxPayload, err := stack.client.MaxPayload()
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("x", int(maxPayload)/5)
	for _, child := range []string{"gamma.001", "beta.002", "zeta.001", "alpha.001", "beta.001"} {
		stack.create(t, countedGroup+"."+child, [2]string{"test.entity.attribute", body})
	}
}

// requireSameJSON compares a reply with a literal, whatever the key order.
func requireSameJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decode reply %q: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decode literal %q: %v", want, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("reply  %s\nwant   %s", got, want)
	}
}

// cutCursor returns the reply with next_cursor's opaque value as true, for the literal, and that value.
func cutCursor(t *testing.T, raw []byte) ([]byte, string) {
	t.Helper()
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatalf("decode reply %q: %v", raw, err)
	}
	var cursor string
	if value, ok := keys["next_cursor"]; ok {
		if err := json.Unmarshal(value, &cursor); err != nil {
			t.Fatalf("decode next_cursor in %q: %v", raw, err)
		}
		keys["next_cursor"] = json.RawMessage(`true`)
	}
	cut, err := json.Marshal(keys)
	if err != nil {
		t.Fatal(err)
	}
	return cut, cursor
}

// TestHierarchyStatsDeclaresAPartialPage: real graph-ingest, a group larger than one page. The reply
// says so, is bare, orders children by count then prefix, and hands on the page's own cursor, which a
// direct read of the prefix verb shows (spec, "A group larger than a page", "Keys on the wire").
func TestHierarchyStatsDeclaresAPartialPage(t *testing.T) {
	stack := startStack(t)
	seedPagedGroup(t, stack)

	raw, err := stack.client.RequestClassified(t.Context(), hierarchyStatsSubject, []byte(`{"prefix":"`+countedGroup+`"}`), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	cut, cursor := cutCursor(t, raw)
	requireSameJSON(t, cut, `{"prefix":"c360.test.query.system","total_entities":5,"entities_scanned":4,"truncated":true,"next_cursor":true,"children":[`+
		`{"prefix":"c360.test.query.system.beta","count":2},`+
		`{"prefix":"c360.test.query.system.alpha","count":1},`+
		`{"prefix":"c360.test.query.system.gamma","count":1}]}`)

	direct, err := stack.client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphIngest, "prefix").Subject,
		[]byte(`{"prefix":"`+countedGroup+`"}`), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(direct, &page); err != nil {
		t.Fatal(err)
	}
	if cursor == "" || cursor != page.NextCursor {
		t.Errorf("next_cursor = %q, the prefix verb's is %q", cursor, page.NextCursor)
	}

	// No prefix counts the whole graph, each child being a first segment.
	raw, err = stack.client.RequestClassified(t.Context(), hierarchyStatsSubject, []byte(`{}`), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	cut, _ = cutCursor(t, raw)
	requireSameJSON(t, cut, `{"prefix":"","total_entities":5,"entities_scanned":4,"truncated":true,"next_cursor":true,"children":[{"prefix":"c360","count":4}]}`)
}

// TestCountsAddUpAcrossPages: a caller following next_cursor to the end reads every seeded entity once,
// and its rows and child counts sum to total_entities, against the seeded literals. The last page has
// no next_cursor and truncated is false. Two literal pages, so a cursor that never ended fails the test
// rather than holding it.
func TestCountsAddUpAcrossPages(t *testing.T) {
	stack := startStack(t)
	seedPagedGroup(t, stack)

	pages := []string{
		`{"prefix":"c360.test.query.system","total_entities":5,"entities_scanned":4,"truncated":true,"next_cursor":true,"children":[` +
			`{"prefix":"c360.test.query.system.beta","count":2},` +
			`{"prefix":"c360.test.query.system.alpha","count":1},` +
			`{"prefix":"c360.test.query.system.gamma","count":1}]}`,
		`{"prefix":"c360.test.query.system","total_entities":5,"entities_scanned":1,"truncated":false,"children":[` +
			`{"prefix":"c360.test.query.system.zeta","count":1}]}`,
	}
	scanned, counted := 0, 0
	request := graph.HierarchyStatsRequest{Prefix: countedGroup}
	for i, want := range pages {
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := stack.client.RequestClassified(t.Context(), hierarchyStatsSubject, data, requestTimeout)
		if err != nil {
			t.Fatalf("page %d: %v", i, err)
		}
		cut, cursor := cutCursor(t, raw)
		requireSameJSON(t, cut, want)
		var reply graph.HierarchyStatsData
		if err := json.Unmarshal(raw, &reply); err != nil {
			t.Fatal(err)
		}
		scanned += reply.EntitiesScanned
		for _, child := range reply.Children {
			counted += child.Count
		}
		request.Cursor = cursor
	}
	if request.Cursor != "" {
		t.Errorf("the last page left next_cursor %q", request.Cursor)
	}
	if scanned != 5 || counted != 5 {
		t.Errorf("rows read = %d, child counts sum to %d, want both 5, the number seeded", scanned, counted)
	}
}

// TestCountsOfAnEmptyGroup: real graph-ingest sends entities null for a prefix holding nothing, which is
// an answer of zero with an empty list, not a broken page. Both with no entity at all and with one
// outside the prefix.
func TestCountsOfAnEmptyGroup(t *testing.T) {
	stack := startStack(t)
	for _, tc := range []struct{ name, request, want string }{
		{"the whole graph, empty", `{}`,
			`{"prefix":"","total_entities":0,"entities_scanned":0,"truncated":false,"children":[]}`},
		{"a prefix holding nothing", `{"prefix":"c360.test.empty"}`,
			`{"prefix":"c360.test.empty","total_entities":0,"entities_scanned":0,"truncated":false,"children":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := stack.client.RequestClassified(t.Context(), hierarchyStatsSubject, []byte(tc.request), requestTimeout)
			if err != nil {
				t.Fatal(err)
			}
			requireSameJSON(t, raw, tc.want)
		})
	}
	stack.create(t, drone1)
	raw, err := stack.client.RequestClassified(t.Context(), hierarchyStatsSubject, []byte(`{"prefix":"c360.test.empty"}`), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	requireSameJSON(t, raw, `{"prefix":"c360.test.empty","total_entities":0,"entities_scanned":0,"truncated":false,"children":[]}`)
}

// spyOnPrefixVerb is spyOnDownstream on the prefix verb.
func spyOnPrefixVerb(t *testing.T, client *natsclient.Client) func() []string {
	t.Helper()
	return spyOnDownstream(t, client, verbRow(t, graph.ResponderGraphIngest, "prefix").Subject)
}

// spyOnDownstream returns a check giving the bodies subject has received since the last one. It sends a
// marker on the connection every component here shares and reads up to it, as spyOnEntityVerb does,
// under the same order and the same limit on what is promised across versions. Whatever answers the
// marker is not read.
func spyOnDownstream(t *testing.T, client *natsclient.Client, subject string) func() []string {
	t.Helper()
	seen := make(chan string, 16)
	if _, err := client.Subscribe(t.Context(), subject, func(_ context.Context, msg *nats.Msg) { seen <- string(msg.Data) }); err != nil {
		t.Fatalf("subscribe %s: %v", subject, err)
	}
	return func() []string {
		t.Helper()
		const marker = `{"prefix":"spy-marker"}`
		_, _ = client.RequestClassified(t.Context(), subject, []byte(marker), requestTimeout)
		var bodies []string
		for got := <-seen; got != marker; got = <-seen {
			bodies = append(bodies, got)
		}
		return bodies
	}
}

// TestCountsReadOnePage: counting beside real graph-ingest, a request is one prefix request and
// nothing else, at the maximum limit, with the request's cursor sent unchanged and its min_revision
// not sent at all (spec, "Counting the requests").
func TestCountsReadOnePage(t *testing.T) {
	stack := startStack(t)
	stack.create(t, drone1)
	stack.create(t, drone2)
	stack.create(t, drone3)

	prefixSubject := verbRow(t, graph.ResponderGraphIngest, "prefix").Subject
	raw, err := stack.client.RequestClassified(t.Context(), prefixSubject, []byte(`{"prefix":"c360","limit":1}`), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	_, cursor := cutCursor(t, raw)
	if cursor == "" {
		t.Fatal("a page of one of three entities names no cursor")
	}

	prefixRequests := spyOnPrefixVerb(t, stack.client)
	others := spyOnWalk(t, stack.client)
	for _, tc := range []struct{ name, request, want string }{
		{"a first page", `{"prefix":"c360","min_revision":7}`, `{"prefix":"c360","limit":1000}`},
		{"a later page", `{"prefix":"c360","cursor":"` + cursor + `"}`, `{"prefix":"c360","limit":1000,"cursor":"` + cursor + `"}`},
		{"the whole graph", `{}`, `{"prefix":"","limit":1000}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := stack.client.RequestClassified(t.Context(), hierarchyStatsSubject, []byte(tc.request), requestTimeout); err != nil {
				t.Fatal(err)
			}
			got := prefixRequests()
			if len(got) != 1 {
				t.Fatalf("prefix requests = %q, want one", got)
			}
			requireSameJSON(t, []byte(got[0]), tc.want)
			if counts := others.count(t); len(counts) != 0 {
				t.Errorf("other requests = %v, want none", counts)
			}
		})
	}
}

// brokenPages are prefix replies no real graph-ingest sends, each with what its refusal says.
var brokenPages = []struct{ name, reply, says string }{
	{"an empty object", `{}`, "names no entities"},
	{"not JSON", `not json`, "invalid character"},
	{"no entities key", `{"total":1}`, "names no entities"},
	{"no total key", `{"entities":null}`, "names no total"},
	{"entities not a list", `{"entities":"none","total":1}`, "cannot unmarshal"},
	{"a row whose ID does not parse", `{"entities":[{"id":"c360.test.short"}],"total":1}`, "entity_id[0]"},
	{"a row with no ID", `{"entities":[{}],"total":1}`, "is not under prefix"},
	{"a row outside the prefix", `{"entities":[{"id":"c360.testing.query.system.drone.001"}],"total":1}`, "is not under prefix"},
}

// TestCountsRefuseABrokenPage: a stub sends what no real graph-ingest does. Each is one transient
// internal refusal and one error. A good page on the same stub answers, so the refusals are the
// replies' doing.
func TestCountsRefuseABrokenPage(t *testing.T) {
	client := fixtureClient(t)
	c := startComponent(t, client, `{}`)
	var reply atomic.Pointer[string]
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "prefix").Subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(*reply.Load()), nil
	})
	request := []byte(`{"prefix":"c360.test"}`)

	errorsSoFar := 0
	for _, tc := range brokenPages {
		t.Run(tc.name, func(t *testing.T) {
			reply.Store(&tc.reply)
			got, err := client.RequestClassified(t.Context(), hierarchyStatsSubject, request, requestTimeout)
			refusal := requireRefusal(t, got, err, errs.ErrorTransient, "internal")
			if !strings.Contains(refusal.Error(), tc.says) {
				t.Errorf("refusal = %v, want it to say %q", refusal, tc.says)
			}
			errorsSoFar++
			if health := c.Health(); health.ErrorCount != errorsSoFar {
				t.Errorf("ErrorCount = %d, want %d", health.ErrorCount, errorsSoFar)
			}
		})
	}

	good := `{"entities":[{"id":"` + drone1 + `"}],"total":1}`
	reply.Store(&good)
	got, err := client.RequestClassified(t.Context(), hierarchyStatsSubject, request, requestTimeout)
	if err != nil {
		t.Fatalf("a good page: %v", err)
	}
	requireSameJSON(t, got, `{"prefix":"c360.test","total_entities":1,"entities_scanned":1,"truncated":false,"children":[{"prefix":"c360.test.query","count":1}]}`)
}

// TestHierarchyStatsOrdersChildren: by count descending, then prefix, written in another order.
// Twenty ties, as with six a missing tie-break passed two runs in three when measured. Go promises
// no map order, so this detection is a measure, not a proof.
func TestHierarchyStatsOrdersChildren(t *testing.T) {
	stack := startStack(t)
	stack.create(t, countedGroup+".z.2")
	stack.create(t, countedGroup+".z.1")
	for _, child := range strings.Split("t s r q p o n m l k j i h g f e d c b a", " ") {
		stack.create(t, countedGroup+"."+child+".1")
	}
	raw, err := stack.client.RequestClassified(t.Context(), hierarchyStatsSubject, []byte(`{"prefix":"`+countedGroup+`"}`), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	requireSameJSON(t, raw, `{"prefix":"c360.test.query.system","total_entities":22,"entities_scanned":22,"truncated":false,"children":[`+
		`{"prefix":"c360.test.query.system.z","count":2},`+
		`{"prefix":"c360.test.query.system.a","count":1},`+
		`{"prefix":"c360.test.query.system.b","count":1},`+
		`{"prefix":"c360.test.query.system.c","count":1},`+
		`{"prefix":"c360.test.query.system.d","count":1},`+
		`{"prefix":"c360.test.query.system.e","count":1},`+
		`{"prefix":"c360.test.query.system.f","count":1},`+
		`{"prefix":"c360.test.query.system.g","count":1},`+
		`{"prefix":"c360.test.query.system.h","count":1},`+
		`{"prefix":"c360.test.query.system.i","count":1},`+
		`{"prefix":"c360.test.query.system.j","count":1},`+
		`{"prefix":"c360.test.query.system.k","count":1},`+
		`{"prefix":"c360.test.query.system.l","count":1},`+
		`{"prefix":"c360.test.query.system.m","count":1},`+
		`{"prefix":"c360.test.query.system.n","count":1},`+
		`{"prefix":"c360.test.query.system.o","count":1},`+
		`{"prefix":"c360.test.query.system.p","count":1},`+
		`{"prefix":"c360.test.query.system.q","count":1},`+
		`{"prefix":"c360.test.query.system.r","count":1},`+
		`{"prefix":"c360.test.query.system.s","count":1},`+
		`{"prefix":"c360.test.query.system.t","count":1}]}`)
}

// TestHierarchyStatsIgnoresMinRevision: the verb reads no index, so it has nothing to refuse and no
// indexed_revision to answer with; a revision past any the graph holds answers bare, with the same
// counts (spec, "An answer that read only the authority").
func TestHierarchyStatsIgnoresMinRevision(t *testing.T) {
	stack := startStack(t)
	stack.create(t, drone1)

	raw, err := stack.client.RequestClassified(t.Context(), hierarchyStatsSubject,
		fmt.Appendf(nil, `{"prefix":"c360.test.query.system.drone","min_revision":%d}`, uint64(1<<63)), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	requireSameJSON(t, raw, `{"prefix":"c360.test.query.system.drone","total_entities":1,"entities_scanned":1,"truncated":false,"children":[{"prefix":"c360.test.query.system.drone.001","count":1}]}`)
}

// TestHierarchyStatsOfAnExactID: a six-part prefix naming an entity real graph-ingest holds counts it
// and names no child (spec, "A prefix that is an entity's ID").
func TestHierarchyStatsOfAnExactID(t *testing.T) {
	stack := startStack(t)
	stack.create(t, drone1)
	stack.create(t, drone2)

	raw, err := stack.client.RequestClassified(t.Context(), hierarchyStatsSubject, []byte(`{"prefix":"`+drone1+`"}`), requestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	requireSameJSON(t, raw, `{"prefix":"c360.test.query.system.drone.001","total_entities":1,"entities_scanned":1,"truncated":false,"children":[]}`)
}

// TestHierarchyStatsRefusesABadRequest: with nothing behind it, a request the verb refuses is its own
// refusal and not the no-responders one, so no read went out. The prefix keeps the validator's code. A
// cursor is graph-ingest's to judge: real graph-ingest refuses a bad one, and that reaches the caller.
func TestHierarchyStatsRefusesABadRequest(t *testing.T) {
	client := fixtureClient(t)
	c := startComponent(t, client, `{}`)
	for _, tc := range []struct{ name, request, code string }{
		{"not JSON", `not json`, "invalid_request"},
		{"no body", ``, "invalid_request"},
		{"an unknown key", `{"prefix":"c360","limit":1}`, "invalid_request"},
		{"bytes after the request", `{"prefix":"c360"} {}`, "invalid_request"},
		{"a prefix of the wrong type", `{"prefix":1}`, "invalid_request"},
		{"a prefix with an empty segment", `{"prefix":"c360..test"}`, "entity_id_prefix_invalid"},
		{"a prefix of seven parts", `{"prefix":"a.b.c.d.e.f.g"}`, "entity_id_prefix_invalid"},
		{"a prefix with a trailing dot", `{"prefix":"c360."}`, "entity_id_prefix_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := client.RequestClassified(t.Context(), hierarchyStatsSubject, []byte(tc.request), requestTimeout)
			requireRefusal(t, got, err, errs.ErrorInvalid, tc.code)
		})
	}
	if health := c.Health(); health.ErrorCount != 0 {
		t.Errorf("ErrorCount = %d, want 0: a caller's bad request is not graph-query's error", health.ErrorCount)
	}
}

// TestHierarchyStatsPassesABadCursorsRefusal: the cursor is opaque here, so only real graph-ingest can
// refuse one, and its refusal reaches the caller as it arrived.
func TestHierarchyStatsPassesABadCursorsRefusal(t *testing.T) {
	stack := startStack(t)
	got, err := stack.client.RequestClassified(t.Context(), hierarchyStatsSubject, []byte(`{"cursor":"!not-a-cursor!"}`), requestTimeout)
	refusal := requireRefusal(t, got, err, errs.ErrorInvalid, "invalid_request")
	if !strings.Contains(refusal.Error(), "invalid cursor") {
		t.Errorf("refusal = %v, want graph-ingest's naming the cursor", refusal)
	}
}
