//go:build integration

package graphlexical

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/storage/storeregistry"
)

// requestBound bounds one request: long enough for any healthy reply, so the bound never decides
// a test.
const requestBound = time.Hour

// searchSubject is the subject the verb table declares for graph-lexical's search.
func searchSubject(t *testing.T) string {
	t.Helper()
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == graph.ResponderGraphLexical && verb.Name == "search" {
			return verb.Subject
		}
	}
	t.Fatal("graph.QueryVerbs declares no search verb for graph-lexical")
	return ""
}

// ask sends body on the search subject once and returns the reply or the refusal.
func ask(t *testing.T, client *natsclient.Client, body string) ([]byte, error) {
	t.Helper()
	return client.RequestClassified(t.Context(), searchSubject(t), []byte(body), requestBound)
}

// answer sends body once and requires an answer, decoded.
func answer(t *testing.T, client *natsclient.Client, body string) graph.QueryResponse[graph.LexicalSearchData] {
	t.Helper()
	raw, err := ask(t, client, body)
	if err != nil {
		t.Fatalf("search %s: %v", body, err)
	}
	var reply graph.QueryResponse[graph.LexicalSearchData]
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("search %s: reply %s: %v", body, raw, err)
	}
	return reply
}

// refusal requires err to be a refusal of class with code, and returns it.
func refusal(t *testing.T, what string, raw []byte, err error, class errs.ErrorClass, code string) *errs.ClassifiedError {
	t.Helper()
	var refused *errs.ClassifiedError
	if !errors.As(err, &refused) || refused.Class != class || refused.Code != code {
		t.Fatalf("%s: reply %s err %v, want class %v code %q", what, raw, err, class, code)
	}
	return refused
}

func resultIDs(data graph.LexicalSearchData) []string {
	ids := make([]string, 0, len(data.Results))
	for _, hit := range data.Results {
		ids = append(ids, hit.EntityID)
	}
	return ids
}

// TestGraphLexicalServesExactlyTheDeclaredVerbs: graph-lexical holds one request subscription per
// row the verb table declares for it, and each row's subject reaches its handler: a request no
// handler but search's refuses as it does, invalid_request naming the empty query.
func TestGraphLexicalServesExactlyTheDeclaredVerbs(t *testing.T) {
	client := fixtureClient(t)
	ensureEntityStates(t, client)
	c := startLexical(t, client, metric.NewMetricsRegistry(), nil)

	rows := 0
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder != graph.ResponderGraphLexical {
			continue
		}
		rows++
		if verb.Name != "search" || !verb.Enveloped {
			t.Errorf("the table declares %+v for graph-lexical; want only the enveloped search", verb)
			continue
		}
		raw, err := client.RequestClassified(t.Context(), verb.Subject, []byte(`{}`), requestBound)
		refused := refusal(t, verb.Subject, raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		if !strings.Contains(refused.Error(), "nothing to search for") {
			t.Errorf("%s: refusal %q does not name the empty query", verb.Subject, refused)
		}
	}
	// Start subscribed on this goroutine before it returned.
	if rows != 1 || len(c.subscriptions) != rows {
		t.Errorf("table rows %d, subscriptions %d; want one subscription per row, 1 row", rows, len(c.subscriptions))
	}
}

// TestSearchWithheldUntilBuilt: an entity's offloaded body is on a store whose read waits until
// the test releases it, so the initial build cannot complete before then. A request sent first is
// refused transient index_not_ready, also when its min_revision is above every revision, while a
// malformed one is refused invalid_request: the request is checked before readiness, and
// readiness before min_revision. Once the read is released the same search is answered, the body
// in the index.
func TestSearchWithheldUntilBuilt(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	const pump = "c360.lex.test.site.pump.gated"
	put(t, states, pump, text{name: "gated pump", offloaded: true})

	// The store ignores its context and answers only on release, so the build cannot complete
	// through the read's deadline while the requests below run.
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseRead := func() { releaseOnce.Do(func() { close(release) }) }
	store := newFakeStore(func(context.Context, string) (io.ReadCloser, error) {
		<-release
		return io.NopCloser(strings.NewReader("impeller housing")), nil
	})
	stores := storeregistry.New()
	if err := stores.Register("objectstore", store); err != nil {
		t.Fatalf("register: %v", err)
	}
	startLexical(t, client, metric.NewMetricsRegistry(), stores)
	// Registered after startLexical, so it runs before the component stops: a failed assertion
	// below still frees the blocked read and lets Stop join it.
	t.Cleanup(releaseRead)

	// The read waits for release, which follows these requests.
	raw, err := ask(t, client, `{"query":"pump"}`)
	refusal(t, "during the build", raw, err, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
	raw, err = ask(t, client, `{"query":"pump","min_revision":1000000}`)
	refusal(t, "during the build, behind", raw, err, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
	raw, err = ask(t, client, `{"query":"pump","limit":101}`)
	refusal(t, "during the build, malformed", raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
	releaseRead()

	got := await(t, "the search answered once the build is done",
		func(context.Context) (error, error) {
			_, err := ask(t, client, `{"query":"impeller"}`)
			return err, nil
		}, func(err error) bool {
			var refused *errs.ClassifiedError
			return !errors.As(err, &refused) || refused.Code != graph.ErrorCodeIndexNotReady
		})
	if got != nil {
		t.Fatalf("search after the build: %v", got)
	}
	reply := answer(t, client, `{"query":"impeller"}`)
	if ids := resultIDs(reply.Data); !slices.Equal(ids, []string{pump}) {
		t.Fatalf("search for a body word after the build found %v; want [%s]", ids, pump)
	}
	if reply.Producer != graph.ResponderGraphLexical || reply.Timestamp.IsZero() || reply.Data.BodiesNotIndexed != 0 {
		t.Errorf("reply = %+v; want producer graph-lexical, a timestamp and no body left out", reply)
	}
}

// TestSearchMinRevisionBoundary: with the follower at revision R, a min_revision of R is answered
// with indexed_revision at or above R, and R+1 is refused transient revision_not_reached with
// detail indexed_revision R, never waited for.
func TestSearchMinRevisionBoundary(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	c := startLexical(t, client, metric.NewMetricsRegistry(), nil)
	put(t, states, "c360.lex.test.site.pump.a", text{name: "north pump"})
	r := put(t, states, "c360.lex.test.site.pump.b", text{name: "south pump"})
	awaitApplied(t, c, r)

	reply := answer(t, client, fmt.Sprintf(`{"query":"pump","min_revision":%d}`, r))
	if reply.IndexedRevision < r || len(reply.Data.Results) != 2 {
		t.Errorf("min_revision %d: indexed_revision %d, %d hits; want at or above %d, 2 hits",
			r, reply.IndexedRevision, len(reply.Data.Results), r)
	}
	raw, err := ask(t, client, fmt.Sprintf(`{"query":"pump","min_revision":%d}`, r+1))
	refused := refusal(t, "min_revision R+1", raw, err, errs.ErrorTransient, graph.ErrorCodeRevisionNotReached)
	if reached := refused.Detail["indexed_revision"]; reached != float64(r) {
		t.Errorf("detail indexed_revision = %v; want %d", reached, r)
	}
}

// TestSearchScopeLimitMatched: the best matches for "pump" lie outside the scope. A search scoped
// to north with limit 1 answers the best match inside it, and matched counts every match inside
// it, not the hits and not the matches outside. bodies_not_indexed counts the whole index: the
// one entity whose body has no store is outside the scope and no hit, and is counted.
func TestSearchScopeLimitMatched(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	const (
		southBest   = "c360.lex.test.south.pump.a"
		southSecond = "c360.lex.test.south.pump.b"
		northBest   = "c360.lex.test.north.pump.a"
		northWorse  = "c360.lex.test.north.pump.b"
		northOther  = "c360.lex.test.north.valve.a"
		eastBodied  = "c360.lex.test.east.tank.a"
	)
	c := startLexical(t, client, metric.NewMetricsRegistry(), nil)
	// Every document but the last two holds "pump"; one with more of it, or with fewer other words,
	// scores higher (BM25: term frequency up, length down).
	put(t, states, southBest, text{name: "pump", inline: "pump pump"})
	put(t, states, southSecond, text{name: "pump", inline: "pump"})
	put(t, states, northBest, text{name: "pump", inline: "spare"})
	put(t, states, northWorse, text{name: "pump", inline: "spare unit kept in the far north shed"})
	put(t, states, northOther, text{name: "valve", inline: "spare"})
	last := put(t, states, eastBodied, text{name: "tank", offloaded: true, instance: "nowhere"})
	awaitApplied(t, c, last)

	global := answer(t, client, `{"query":"pump","limit":1}`).Data
	if ids := resultIDs(global); !slices.Equal(ids, []string{southBest}) {
		t.Fatalf("unscoped limit 1 = %v; want [%s]: the best match lies outside the scope", ids, southBest)
	}
	if global.Matched != 4 {
		t.Errorf("unscoped matched = %d; want 4, every match", global.Matched)
	}

	scoped := answer(t, client, `{"query":"pump","limit":1,"scope":["c360.lex.test.north"]}`).Data
	if ids := resultIDs(scoped); !slices.Equal(ids, []string{northBest}) {
		t.Errorf("scoped limit 1 found %v; want [%s], the best inside the scope", ids, northBest)
	}
	if scoped.Matched != 2 {
		t.Errorf("scoped matched = %d; want 2, every match inside the scope", scoped.Matched)
	}
	if scoped.BodiesNotIndexed != 1 || global.BodiesNotIndexed != 1 {
		t.Errorf("bodies_not_indexed = %d scoped, %d unscoped; want 1 in both, the whole index's",
			scoped.BodiesNotIndexed, global.BodiesNotIndexed)
	}

	// Two prefixes, one of them a whole literal ID: their union, ordered by score.
	union := answer(t, client, `{"query":"pump","scope":["c360.lex.test.north","c360.lex.test.south.pump.b"]}`).Data
	if ids := resultIDs(union); !slices.Equal(ids, []string{southSecond, northBest, northWorse}) || union.Matched != 3 {
		t.Errorf("two-prefix scope = %v matched %d; want [%s %s %s] matched 3", ids, union.Matched,
			southSecond, northBest, northWorse)
	}
}

// TestSearchLimitRange: 101 entities match equally, so the hits are the first IDs in order. A
// limit of 0 answers 10, 1 and 100 answer as many, and 101 and -1 are refused invalid_request,
// never clamped. matched is 101 in every answer.
func TestSearchLimitRange(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	c := startLexical(t, client, metric.NewMetricsRegistry(), nil)
	var ids []string
	var last uint64
	for n := range 101 {
		id := fmt.Sprintf("c360.lex.test.site.pump.%03d", n)
		ids = append(ids, id)
		last = put(t, states, id, text{name: "pump"})
	}
	awaitApplied(t, c, last)

	for _, tc := range []struct{ limit, want int }{{0, 10}, {1, 1}, {100, 100}} {
		data := answer(t, client, fmt.Sprintf(`{"query":"pump","limit":%d}`, tc.limit)).Data
		if got := resultIDs(data); !slices.Equal(got, ids[:tc.want]) || data.Matched != 101 {
			t.Errorf("limit %d: %d hits matched %d; want the first %d IDs, matched 101", tc.limit, len(got),
				data.Matched, tc.want)
		}
	}
	for _, limit := range []int{101, -1} {
		raw, err := ask(t, client, fmt.Sprintf(`{"query":"pump","limit":%d}`, limit))
		refused := refusal(t, fmt.Sprintf("limit %d", limit), raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		if !strings.Contains(refused.Error(), "limit") {
			t.Errorf("limit %d: refusal %q does not name the limit", limit, refused)
		}
	}
}

// TestSearchRefusesParseErrors: a query lexical.ParseQuery refuses is refused invalid_request, the
// message naming the rule: reserved syntax for pump* and name:pump, nothing to search for, too many
// distinct words.
func TestSearchRefusesParseErrors(t *testing.T) {
	client := fixtureClient(t)
	ensureEntityStates(t, client)
	startLexical(t, client, metric.NewMetricsRegistry(), nil)

	words := make([]string, graph.MaxSearchTerms+1)
	for i := range words {
		words[i] = fmt.Sprintf("word%c", 'a'+i)
	}
	for _, tc := range []struct{ query, rule string }{
		{"pump*", "reserved"},
		{"name:pump", "reserved"},
		{"", "nothing to search for"},
		{"!!! ...", "nothing to search for"},
		{strings.Join(words, " "), "too many distinct words"},
	} {
		body, err := json.Marshal(graph.LexicalSearchRequest{Query: tc.query})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		raw, err := ask(t, client, string(body))
		refused := refusal(t, fmt.Sprintf("query %q", tc.query), raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		if !strings.Contains(refused.Error(), tc.rule) {
			t.Errorf("query %q: refusal %q does not name %q", tc.query, refused, tc.rule)
		}
	}
}

// TestSearchRefusesMalformedRequests: a request is decoded strictly and its scope prefixes are
// checked by the shared entity-ID prefix validator, each refusal invalid_request naming what is
// wrong.
func TestSearchRefusesMalformedRequests(t *testing.T) {
	client := fixtureClient(t)
	ensureEntityStates(t, client)
	startLexical(t, client, metric.NewMetricsRegistry(), nil)

	for _, tc := range []struct{ body, names string }{
		{`{"query":"pump","workers":2}`, "workers"},
		{`{"query":"pump"} {}`, "data after the request object"},
		{`{"query":"pump","scope":["c360..lex"]}`, "scope[0]"},
		{`{"query":"pump","scope":["c360.lex.test.site.pump.a.b"]}`, "scope[0]"},
		{`{"query":"pump","scope":["", "c360.lex*"]}`, "scope[1]"},
		{`{"query":"pump","min_revision":-1}`, "min_revision"},
	} {
		raw, err := ask(t, client, tc.body)
		refused := refusal(t, tc.body, raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		if !strings.Contains(refused.Error(), tc.names) {
			t.Errorf("%s: refusal %q does not name %q", tc.body, refused, tc.names)
		}
	}
}
