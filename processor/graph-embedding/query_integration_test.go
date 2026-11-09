//go:build integration

package graphembedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/pkg/errs"
)

// subjectOf is the subject the verb table in graph declares for graph-embedding's verb name.
func subjectOf(t *testing.T, name string) string {
	t.Helper()
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == graph.ResponderGraphEmbedding && verb.Name == name {
			return verb.Subject
		}
	}
	t.Fatalf("the verb table declares no graph-embedding verb %q", name)
	return ""
}

// ask sends body on verb's subject through the classified request path a caller uses.
func (h *harness) ask(t *testing.T, verb, body string) ([]byte, error) {
	t.Helper()
	return h.client.RequestClassified(t.Context(), subjectOf(t, verb), []byte(body), requestBound)
}

// asked is one request's outcome: the raw reply and the decoded envelope, or the refusal.
type asked[T any] struct {
	raw   []byte
	reply graph.QueryResponse[T]
	err   error
}

func isNotReady(err error) bool {
	var refusal *errs.ClassifiedError
	return errors.As(err, &refusal) && refusal.Code == graph.ErrorCodeIndexNotReady
}

// awaitAnswer sends body on verb until it is answered and done accepts the reply, and returns
// that reply. The vector cache applies EMBEDDING_INDEX after the Worker's write lands in the bucket
// (design D4), so a test reads what it wrote through the reply that ends this wait. A refusal
// index_not_ready (the cache has not applied its first snapshot) is asked again; any other
// refusal fails the test at once.
func awaitAnswer[T any](t *testing.T, h *harness, verb, body string, done func(graph.QueryResponse[T]) bool) asked[T] {
	t.Helper()
	got := await(t, fmt.Sprintf("%s %s answered", verb, body), func(ctx context.Context) (asked[T], error) {
		raw, err := h.client.RequestClassified(ctx, subjectOf(t, verb), []byte(body), requestBound)
		if err != nil {
			return asked[T]{err: err}, nil
		}
		out := asked[T]{raw: raw}
		out.err = json.Unmarshal(raw, &out.reply)
		return out, nil
	}, func(a asked[T]) bool {
		if a.err != nil {
			return !isNotReady(a.err)
		}
		return done(a.reply)
	})
	if got.err != nil {
		t.Fatalf("%s %s: %v", verb, body, got.err)
	}
	return got
}

// answerNow sends body on verb once and requires an answer: the cache has applied what the test
// awaited before, and a test reads nothing written since.
func answerNow[T any](t *testing.T, h *harness, verb, body string) asked[T] {
	t.Helper()
	raw, err := h.ask(t, verb, body)
	if err != nil {
		t.Fatalf("%s %s: %v", verb, body, err)
	}
	out := asked[T]{raw: raw}
	if err := json.Unmarshal(raw, &out.reply); err != nil {
		t.Fatalf("%s reply %s: %v", verb, raw, err)
	}
	return out
}

// requireRefused requires err to be a refusal of class with code ("" for none).
func requireRefused(t *testing.T, what string, raw []byte, err error, class errs.ErrorClass, code string) {
	t.Helper()
	var refusal *errs.ClassifiedError
	if !errors.As(err, &refusal) || refusal.Class != class || refusal.Code != code {
		t.Errorf("%s: reply %s err %v, want class %v code %q", what, raw, err, class, code)
	}
}

func ids(hits []graph.SimilarityHit) []string {
	out := make([]string, len(hits))
	for i, hit := range hits {
		out[i] = hit.EntityID
	}
	return out
}

func hitCount[T graph.SemanticData | graph.SimilarData](n int) func(graph.QueryResponse[T]) bool {
	return func(r graph.QueryResponse[T]) bool {
		switch data := any(r.Data).(type) {
		case graph.SemanticData:
			return len(data.Results) == n
		case graph.SimilarData:
			return len(data.Results) == n
		}
		return false
	}
}

// titled writes each id to ENTITY_STATES with title as its one text predicate, in the order given.
func (h *harness) titled(t *testing.T, title string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		h.put(t, id, entityValue(t, id, nil, "doc.meta.title", title))
	}
}

func numbered(prefix string, from, to int) []string {
	var out []string
	for n := from; n <= to; n++ {
		out = append(out, fmt.Sprintf("%s.%03d", prefix, n))
	}
	return out
}

// TestVerbsServeTableSubjects: graph-embedding serves search and similar on the subjects their
// verb table rows name, each reaching its own handler, and holds one subscription per row and no
// other (design D1; graph-embedding spec, "The embedding verbs are served from Start until
// Stop"). Each request is one only its verb's handler refuses as it does: search names the blank
// query, similar the missing entity_id.
func TestVerbsServeTableSubjects(t *testing.T) {
	h := startEmbedding(t, fixtureClient(t), `{}`, nil)
	names := map[string]string{"search": "query", "similar": "entity_id"}
	rows := 0
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder != graph.ResponderGraphEmbedding {
			continue
		}
		rows++
		named, ok := names[verb.Name]
		if !ok {
			t.Errorf("the table declares %s for graph-embedding and this test has no request for it", verb.Name)
			continue
		}
		raw, err := h.client.RequestClassified(t.Context(), verb.Subject, []byte(`{}`), requestBound)
		requireRefused(t, verb.Subject, raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		if err != nil && !bytes.Contains([]byte(err.Error()), []byte(named)) {
			t.Errorf("%s: refusal %q does not name %s", verb.Subject, err, named)
		}
	}
	// Start subscribed on this goroutine before it returned.
	if rows != len(names) || len(h.c.subscriptions) != rows {
		t.Errorf("table rows %d, subscriptions %d; want one subscription per row, %d rows", rows,
			len(h.c.subscriptions), len(names))
	}
}

// TestSearchAnswersEnveloped: an answer is a graph.QueryResponse from graph-embedding with
// indexed_revision 0, a min_revision of 0 is answered, and results is a JSON array, never null,
// on both verbs (design D1, D4). The index holds one generated record, so search's "zebra" and
// similar's lone source each have no hit.
func TestSearchAnswersEnveloped(t *testing.T) {
	h := startEmbedding(t, fixtureClient(t), `{}`, nil)
	empty := awaitAnswer(t, h, "search", `{"query":"widget","min_revision":0}`, hitCount[graph.SemanticData](0))
	a := entityID(1)
	h.awaitGenerated(t, a, h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "widget")))
	awaitAnswer(t, h, "search", `{"query":"widget"}`, hitCount[graph.SemanticData](1))
	similar := answerNow[graph.SimilarData](t, h, "similar", fmt.Sprintf(`{"entity_id":%q,"min_revision":0}`, a))

	for _, got := range []struct {
		raw      []byte
		producer string
		revision uint64
		zero     bool
	}{
		{empty.raw, empty.reply.Producer, empty.reply.IndexedRevision, empty.reply.Timestamp.IsZero()},
		{similar.raw, similar.reply.Producer, similar.reply.IndexedRevision, similar.reply.Timestamp.IsZero()},
	} {
		if got.producer != graph.ResponderGraphEmbedding || got.revision != 0 || got.zero {
			t.Errorf("reply %s: want producer graph-embedding, indexed_revision 0 and a timestamp", got.raw)
		}
		if !bytes.Contains(got.raw, []byte(`"results":[]`)) {
			t.Errorf("reply %s: results is not the empty array", got.raw)
		}
	}
	if data := empty.reply.Data; data.EmbedderType != "bm25" || data.UnreadableRecords != 0 || data.FailedRecords != 0 {
		t.Errorf("empty search data = %+v, want embedder_type bm25 and no record left out", data)
	}
}

// TestSearchLimitEdges: on both verbs a limit of 0 means 10, 1 and 100 are used as sent, and 101
// and -1 are refused invalid_request naming limit and the bound, never clamped (design D3). 101
// entities share one title, so every one is a hit and only the limit bounds the answer.
func TestSearchLimitEdges(t *testing.T) {
	h := startEmbedding(t, fixtureClient(t), `{}`, nil)
	entities := numbered("acme.ops.test.system.doc", 1, 101)
	h.titled(t, "widget", entities...)
	awaitAnswer(t, h, "search", `{"query":"widget","limit":100}`, hitCount[graph.SemanticData](100))

	for limit, want := range map[int]int{0: 10, 1: 1, 100: 100} {
		search := answerNow[graph.SemanticData](t, h, "search", fmt.Sprintf(`{"query":"widget","limit":%d}`, limit))
		if got := len(search.reply.Data.Results); got != want {
			t.Errorf("search limit %d: %d hits, want %d", limit, got, want)
		}
		similar := answerNow[graph.SimilarData](t, h, "similar", fmt.Sprintf(`{"entity_id":%q,"limit":%d}`, entities[0], limit))
		if got := len(similar.reply.Data.Results); got != want {
			t.Errorf("similar limit %d: %d hits, want %d", limit, got, want)
		}
	}
	for _, limit := range []int{101, -1} {
		raw, err := h.ask(t, "search", fmt.Sprintf(`{"query":"widget","limit":%d}`, limit))
		requireRefused(t, fmt.Sprintf("search limit %d", limit), raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		raw, err = h.ask(t, "similar", fmt.Sprintf(`{"entity_id":%q,"limit":%d}`, entities[0], limit))
		requireRefused(t, fmt.Sprintf("similar limit %d", limit), raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
	}
}

// TestSearchQueryEdges: a blank query is refused, and a hit always scores above 0: a query of
// stopwords only embeds to the zero vector, which scores every entity 0, and answers no hit; a
// query sharing no term with an entity does not return it (design D3). Two widget entities and
// three gadget entities are indexed, so a limit of 10 would hold every one of them.
func TestSearchQueryEdges(t *testing.T) {
	h := startEmbedding(t, fixtureClient(t), `{}`, nil)
	widgets := numbered("acme.ops.test.system.widget", 1, 2)
	h.titled(t, "widget", widgets...)
	h.titled(t, "gadget", numbered("acme.ops.test.system.gadget", 1, 3)...)
	// Every entity shares a term with this query, so the wait ends on all five in the cache
	// whatever a 0 score does.
	awaitAnswer(t, h, "search", `{"query":"widget gadget"}`, hitCount[graph.SemanticData](5))

	if got := ids(answerNow[graph.SemanticData](t, h, "search", `{"query":"widget"}`).reply.Data.Results); !slices.Equal(got, widgets) {
		t.Errorf("search widget = %v, want only %v", got, widgets)
	}
	for _, query := range []string{"the and of", "zebra"} {
		search := answerNow[graph.SemanticData](t, h, "search", fmt.Sprintf(`{"query":%q}`, query))
		if !bytes.Contains(search.raw, []byte(`"results":[]`)) {
			t.Errorf("search %q = %s, want no hit", query, search.raw)
		}
	}
	for _, query := range []string{"", "   "} {
		raw, err := h.ask(t, "search", fmt.Sprintf(`{"query":%q}`, query))
		requireRefused(t, fmt.Sprintf("search %q", query), raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
	}
}

// TestSearchScopeEdges: a malformed scope prefix is refused invalid_request and leaves the
// embedder's statistics as they were (the same search answers the same scores after it); an empty
// scope element matches every entity; a scope matching nothing answers no hit; a scope keeps only
// the entities under it (design D3).
func TestSearchScopeEdges(t *testing.T) {
	h := startEmbedding(t, fixtureClient(t), `{}`, nil)
	team := numbered("acme.ops.team.system.doc", 1, 2)
	other := numbered("acme.ops.other.system.doc", 1, 2)
	h.titled(t, "widget gizmo", team...)
	h.titled(t, "widget", other...)
	before := awaitAnswer(t, h, "search", `{"query":"widget gizmo"}`, hitCount[graph.SemanticData](4))

	raw, err := h.ask(t, "search", `{"query":"widget gizmo","scope":["acme.*"]}`)
	requireRefused(t, "malformed scope", raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
	if after := answerNow[graph.SemanticData](t, h, "search", `{"query":"widget gizmo"}`); !slices.Equal(after.reply.Data.Results, before.reply.Data.Results) {
		t.Errorf("after a refused scope the search answers %v, before it %v", after.reply.Data.Results, before.reply.Data.Results)
	}

	for scope, want := range map[string][]string{
		`[""]`:                              append(slices.Clone(team), other...),
		`["acme.ops.nothing"]`:              {},
		`["acme.ops.other"]`:                other,
		`["acme.ops.other", ""]`:            append(slices.Clone(team), other...),
		`["acme.ops.team.system"]`:          team,
		`["acme.ops.other.system.doc.002"]`: other[1:],
	} {
		search := answerNow[graph.SemanticData](t, h, "search", fmt.Sprintf(`{"query":"widget gizmo","scope":%s}`, scope))
		got := ids(search.reply.Data.Results)
		slices.Sort(got)
		wantSorted := slices.Sorted(slices.Values(want))
		if !slices.Equal(got, wantSorted) {
			t.Errorf("scope %s = %v, want %v", scope, got, wantSorted)
		}
	}
}

// TestSimilarEdges: similar refuses a malformed entity_id invalid_request; an entity with no
// record, a pending record, a failed record or a generated record with no vector class invalid,
// code embedding_unavailable; an undecodable source record class invalid with no code; and the
// entity named is never its own hit (design D3). The Worker's one lane is held, after the two
// widget entities are generated, so the third entity's record stays pending.
func TestSimilarEdges(t *testing.T) {
	store := newGatedStore()
	h := startEmbedding(t, fixtureClient(t), `{"workers": 1}`, store.stores(t))
	a, b := entityID(1), entityID(2)
	h.awaitGenerated(t, a, h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "widget")))
	h.awaitGenerated(t, b, h.put(t, b, entityValue(t, b, nil, "doc.meta.title", "widget")))
	holdWorker(t, h, store)
	pending := entityID(3)
	h.awaitPending(t, pending, h.put(t, pending, entityValue(t, pending, nil, "doc.meta.title", "widget")))

	index, err := kvcatalog.EnsureCatalogBucket(t.Context(), h.client, graph.BucketEmbeddingIndex)
	if err != nil {
		t.Fatal(err)
	}
	failed, vectorless, undecodable := entityID(4), entityID(5), entityID(6)
	for key, value := range map[string]string{
		failed:      fmt.Sprintf(`{"entity_id":%q,"content_hash":"h","status":"failed","error_msg":"x"}`, failed),
		vectorless:  fmt.Sprintf(`{"entity_id":%q,"content_hash":"h","status":"generated"}`, vectorless),
		undecodable: `not a record`,
	} {
		if _, err := index.Put(t.Context(), key, []byte(value)); err != nil {
			t.Fatalf("put %s: %v", key, err)
		}
	}

	// The undecodable record poisons its key in the cache; the reply counting it orders the
	// self-exclusion read after the cache applied every write above.
	self := awaitAnswer(t, h, "similar", fmt.Sprintf(`{"entity_id":%q}`, a),
		func(r graph.QueryResponse[graph.SimilarData]) bool { return r.Data.UnreadableRecords == 1 })
	if got := ids(self.reply.Data.Results); !slices.Equal(got, []string{b}) {
		t.Errorf("similar to %s = %v, want only %s", a, got, b)
	}
	// search counts the same poisoned record: the reply above orders this read after it.
	if search := answerNow[graph.SemanticData](t, h, "search", `{"query":"widget"}`); search.reply.Data.UnreadableRecords != 1 {
		t.Errorf("search with one undecodable record = %s, want unreadable_records 1", search.raw)
	}

	for _, tc := range []struct {
		id    string
		class errs.ErrorClass
		code  string
	}{
		{"acme.ops.test.system.doc", errs.ErrorInvalid, graph.ErrorCodeInvalidRequest},
		{"acme.ops.test.system.doc.*", errs.ErrorInvalid, graph.ErrorCodeInvalidRequest},
		{entityID(999), errs.ErrorInvalid, graph.ErrorCodeEmbeddingUnavailable},
		{pending, errs.ErrorInvalid, graph.ErrorCodeEmbeddingUnavailable},
		{failed, errs.ErrorInvalid, graph.ErrorCodeEmbeddingUnavailable},
		{vectorless, errs.ErrorInvalid, graph.ErrorCodeEmbeddingUnavailable},
		{undecodable, errs.ErrorInvalid, ""},
	} {
		raw, err := h.ask(t, "similar", fmt.Sprintf(`{"entity_id":%q}`, tc.id))
		requireRefused(t, "similar "+tc.id, raw, err, tc.class, tc.code)
	}
}

// TestTopKAtTie: when more entities tie on similarity than the limit admits, the hits kept are the
// smallest entity IDs among the tied, on every call, on both verbs (design D3; the pin's
// similar_determinism_integration_test.go:31, ported to the verbs). Twenty entities share one
// one-word title, so each vector is the same unit vector and every similarity is exactly 1; they
// are written in reverse order, so neither the bucket's order nor the view's is the answer.
func TestTopKAtTie(t *testing.T) {
	h := startEmbedding(t, fixtureClient(t), `{}`, nil)
	entities := numbered("c360.plat.docs.site.doc", 1, 20)
	reversed := slices.Clone(entities)
	slices.Reverse(reversed)
	h.titled(t, "widget", reversed...)
	awaitAnswer(t, h, "search", `{"query":"widget","limit":20}`, hitCount[graph.SemanticData](20))

	const k = 8
	for range 10 {
		search := answerNow[graph.SemanticData](t, h, "search", fmt.Sprintf(`{"query":"widget","limit":%d}`, k))
		if got := ids(search.reply.Data.Results); !slices.Equal(got, entities[:k]) {
			t.Fatalf("search top %d at a tie = %v, want %v", k, got, entities[:k])
		}
		similar := answerNow[graph.SimilarData](t, h, "similar", fmt.Sprintf(`{"entity_id":%q,"limit":%d}`, entities[19], k))
		if got := ids(similar.reply.Data.Results); !slices.Equal(got, entities[:k]) {
			t.Fatalf("similar top %d at a tie = %v, want %v", k, got, entities[:k])
		}
	}
}

// TestScopedSearch: a scope filters the candidates before they are ranked and limited, so a scoped
// search returns the best in-scope entities even when out-of-scope ones score higher (ADR-071;
// design D3; the pin's scope_integration_test.go:25, ported to the verb). The code entities match
// the query exactly; the docs entities share one of its terms only, so they score lower, and a
// limit of 2 cut before the scope would leave nothing in scope.
func TestScopedSearch(t *testing.T) {
	h := startEmbedding(t, fixtureClient(t), `{}`, nil)
	code := []string{
		"c360.semspec.python.pkg.fn.test_raises",
		"c360.semspec.python.pkg.fn.request",
		"c360.semspec.python.pkg.fn.session",
		"c360.semspec.golang.pkg.fn.Handle",
	}
	docs := []string{
		"c360.semspec.source.chunk.segment.exceptions_0",
		"c360.semspec.source.doc.page.exceptions",
	}
	h.titled(t, "exceptions", code...)
	h.titled(t, "exceptions handling", docs...)
	unscoped := awaitAnswer(t, h, "search", `{"query":"exceptions"}`, hitCount[graph.SemanticData](6))
	if got := ids(unscoped.reply.Data.Results[:len(code)]); !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(code))) {
		t.Fatalf("unscoped search ranks %v first, want the code entities: the docs must score lower", got)
	}

	scoped := answerNow[graph.SemanticData](t, h, "search",
		`{"query":"exceptions","limit":2,"scope":["c360.semspec.source.doc","c360.semspec.source.chunk"]}`)
	if got := ids(scoped.reply.Data.Results); !slices.Equal(slices.Sorted(slices.Values(got)), docs) {
		t.Errorf("scoped search = %v, want only the docs %v", got, docs)
	}
}
