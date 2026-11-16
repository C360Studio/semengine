//go:build integration

package graphquery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// fallbackQuestion is a question of several words with no hints, so global search reads no
// reference and lists no hint: the word search is the only request before a fallback.
const fallbackQuestion = `{"query":"zzz qqq","limit":7}`

// semanticData is the embedding search's reply data in the stubs below: written out with odd
// spacing and an unknown field, so a reply that re-encoded it, or built it from SemanticData,
// would differ from the bytes the embedding search sent.
const semanticData = `{"results":[{"entity_id":"c360.test.query.system.drone.001","similarity":0.5}],` +
	`"embedder_type":"fixed","unreadable_records":0,"failed_records":0,"unknown_field":[1.5]}`

// fallbackReply is a searchGraph answer's data as the wire spells it, read by the test's own
// type: text, semantic and fallback by their keys, so it reads replies with and without them.
type fallbackReply struct {
	Text     *graph.TextSearchData    `json:"text"`
	Semantic json.RawMessage          `json:"semantic"`
	Fallback *graph.GlobalSearchHint  `json:"fallback"`
	Hints    []graph.GlobalSearchHint `json:"hints"`
	keys     map[string]json.RawMessage
}

// decodeFallback decodes a searchGraph success: the envelope strictly, then its data's keys and
// fields. A key the reply leaves out is absent from keys; one it sends as null is present.
func decodeFallback(t *testing.T, raw []byte) (wireEnvelope, fallbackReply) {
	t.Helper()
	envelope := decodeEnvelope(t, raw)
	var reply fallbackReply
	if err := json.Unmarshal(envelope.Data, &reply); err != nil {
		t.Fatalf("decode data %s: %v", envelope.Data, err)
	}
	if err := json.Unmarshal(envelope.Data, &reply.keys); err != nil {
		t.Fatalf("decode data keys %s: %v", envelope.Data, err)
	}
	return envelope, reply
}

// stubSemantic stands in for graph-embedding's search verb, answering every request with answer,
// and returns the requests it received. It stores each before it answers, so before graph-query's
// reply.
func stubSemantic(t *testing.T, client *natsclient.Client, answer func() ([]byte, error)) func() [][]byte {
	t.Helper()
	var received atomic.Pointer[[][]byte]
	received.Store(&[][]byte{})
	serve(t, client, verbRow(t, graph.ResponderGraphEmbedding, "search").Subject, func(_ context.Context, data []byte) ([]byte, error) {
		got := append(append([][]byte(nil), *received.Load()...), data)
		received.Store(&got)
		return answer()
	})
	return func() [][]byte { return *received.Load() }
}

// semanticAnswer is graph-embedding's search success, as it sends one, holding semanticData.
func semanticAnswer() ([]byte, error) {
	return []byte(`{"data":` + semanticData + `,"indexed_revision":0,"producer":"graph-embedding","timestamp":"2026-11-16T00:00:00Z"}`), nil
}

// askSearchGraph sends request to searchGraph.
func askSearchGraph(t *testing.T, client *natsclient.Client, request string) ([]byte, error) {
	t.Helper()
	return client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphQuery, "searchGraph").Subject, []byte(request), requestTimeout)
}

// requireOneSemanticRequest holds that the embedding search was asked exactly once, with the
// question and the request's limit, and nothing else: no min_revision, which it refuses above 0.
func requireOneSemanticRequest(t *testing.T, received [][]byte) {
	t.Helper()
	if len(received) != 1 {
		t.Fatalf("the embedding search received %q, want one request", received)
	}
	requireSameJSON(t, received[0], `{"query":"zzz qqq","limit":7}`)
}

// TestSearchGraphTierZeroAsksNoEmbedding: with embedding_enabled false, searchGraph answers a
// question the word index matches nothing for with the empty text alone, no semantic and no
// fallback, and sends the embedding search nothing, though it is served and would answer.
func TestSearchGraphTierZeroAsksNoEmbedding(t *testing.T) {
	client := fixtureClient(t)
	emptyWordSearch(t, client)
	received := stubSemantic(t, client, semanticAnswer)
	startComponent(t, client, `{"embedding_enabled": false}`)

	raw, err := askSearchGraph(t, client, fallbackQuestion)
	if err != nil {
		t.Fatalf("searchGraph: %v", err)
	}
	envelope, reply := decodeFallback(t, raw)
	requireSameJSON(t, envelope.Data, `{"text":`+emptyText+`,"hints":[]}`)
	if envelope.IndexedRevision != 9 {
		t.Errorf("indexed_revision = %d, want the word search's 9", envelope.IndexedRevision)
	}
	if reply.Fallback != nil || reply.Semantic != nil {
		t.Errorf("data = %s, want no fallback and no semantic", envelope.Data)
	}
	// The stub records before it answers, and graph-query would have had its answer before it replied.
	if got := received(); len(got) != 0 {
		t.Errorf("the embedding search received %q, want nothing at tier 0", got)
	}
}

// TestSearchGraphFallsBackWhenNoMatches: at tier 1, a question the word index matches nothing
// for is asked of the embedding search once, and the reply holds the empty text, semantic with
// the embedding reply's data as its bytes came, and fallback honoured with detail no_matches, under
// the word search's revision. globalSearch, asked the same, answers the text alone with no
// semantic request. A question the word index matches answers without a fallback.
func TestSearchGraphFallsBackWhenNoMatches(t *testing.T) {
	client := fixtureClient(t)
	emptyWordSearch(t, client)
	received := stubSemantic(t, client, semanticAnswer)
	startComponent(t, client, tierOneConfig)

	raw, err := askSearchGraph(t, client, fallbackQuestion)
	if err != nil {
		t.Fatalf("searchGraph: %v", err)
	}
	envelope, reply := decodeFallback(t, raw)
	if reply.Text == nil {
		t.Fatalf("data = %s, want the empty text", envelope.Data)
	}
	text, err := json.Marshal(reply.Text)
	if err != nil {
		t.Fatal(err)
	}
	requireSameJSON(t, text, emptyText)
	// The marshal of the reply compacts a json.RawMessage, which these bytes do not need.
	var want bytes.Buffer
	if err := json.Compact(&want, []byte(semanticData)); err != nil {
		t.Fatal(err)
	}
	if string(reply.Semantic) != want.String() {
		t.Errorf("semantic = %s, want the embedding reply's data %s", reply.Semantic, want.String())
	}
	if wantHint := (graph.GlobalSearchHint{Hint: "semantic", Detail: "no_matches", Honoured: true}); reply.Fallback == nil || *reply.Fallback != wantHint {
		t.Errorf("fallback = %+v, want %+v", reply.Fallback, wantHint)
	}
	if envelope.IndexedRevision != 9 {
		t.Errorf("indexed_revision = %d, want the word search's 9", envelope.IndexedRevision)
	}
	requireOneSemanticRequest(t, received())

	global, err := client.RequestClassified(t.Context(), globalSearchSubject, []byte(fallbackQuestion), requestTimeout)
	if err != nil {
		t.Fatalf("globalSearch: %v", err)
	}
	requireSameJSON(t, decodeEnvelope(t, global).Data, `{"text":`+emptyText+`,"hints":[]}`)
	if got := received(); len(got) != 1 {
		t.Errorf("after globalSearch the embedding search received %d requests, want still one", len(got))
	}
}

// TestSearchGraphMatchesAskNoEmbedding: at tier 1, a question the word index matches answers the
// matches with no semantic request, no semantic and no fallback.
func TestSearchGraphMatchesAskNoEmbedding(t *testing.T) {
	client := fixtureClient(t)
	stubWordIndex(t, client, lexicalReply(t, 4, graph.LexicalSearchData{
		Results: []graph.LexicalHit{{EntityID: drone1, Score: 1}}, Matched: 1,
	}))
	stubBatch(t, client, `{"entities":[`+titled(drone1, "Alpha")+`],"missing":[]}`)
	received := stubSemantic(t, client, semanticAnswer)
	startComponent(t, client, tierOneConfig)

	raw, err := askSearchGraph(t, client, fallbackQuestion)
	if err != nil {
		t.Fatalf("searchGraph: %v", err)
	}
	envelope, reply := decodeFallback(t, raw)
	if reply.Text == nil || len(reply.Text.Matches) != 1 || reply.Text.Matches[0].ID != drone1 {
		t.Fatalf("data = %s, want the one match %s", envelope.Data, drone1)
	}
	if _, ok := reply.keys["fallback"]; ok {
		t.Errorf("data = %s, want no fallback", envelope.Data)
	}
	if _, ok := reply.keys["semantic"]; ok {
		t.Errorf("data = %s, want no semantic", envelope.Data)
	}
	if got := received(); len(got) != 0 {
		t.Errorf("the embedding search received %q, want nothing when the words match", got)
	}
}

// TestSearchGraphNamesAFailedFallback: when the word index matches nothing and the embedding search
// refuses, the reply is the word answer, its empty text, with fallback not honoured, carrying the
// refusal's code and message: never an unlabelled empty answer, and no semantic.
func TestSearchGraphNamesAFailedFallback(t *testing.T) {
	client := fixtureClient(t)
	emptyWordSearch(t, client)
	refusal := errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeEmbeddingUnavailable, errors.New("the embedder is down"))
	received := stubSemantic(t, client, func() ([]byte, error) { return nil, refusal })
	startComponent(t, client, tierOneConfig)

	raw, err := askSearchGraph(t, client, fallbackQuestion)
	if err != nil {
		t.Fatalf("searchGraph: %v; want the word answer with the failed fallback named", err)
	}
	envelope, reply := decodeFallback(t, raw)
	if reply.Text == nil {
		t.Fatalf("data = %s, want the empty text", envelope.Data)
	}
	if reply.Fallback == nil {
		t.Fatalf("data = %s, want a fallback naming the refusal", envelope.Data)
	}
	got := *reply.Fallback
	if got.Hint != "semantic" || got.Detail != "no_matches" || got.Honoured || got.Code != graph.ErrorCodeEmbeddingUnavailable ||
		!strings.Contains(got.Reason, "the embedder is down") {
		t.Errorf("fallback = %+v, want semantic, no_matches, not honoured, code %s, reason naming the refusal",
			got, graph.ErrorCodeEmbeddingUnavailable)
	}
	if _, ok := reply.keys["semantic"]; ok {
		t.Errorf("data = %s, want no semantic", envelope.Data)
	}
	requireOneSemanticRequest(t, received())
}

// TestSearchGraphIndexNotReadyLeavesTextOut: when the word search refuses index_not_ready and the
// embedding search answers, the reply sends text as null (the word index was not read), semantic,
// and fallback honoured with detail index_not_ready. When the embedding search refuses too, the
// index_not_ready refusal is returned as it arrived.
func TestSearchGraphIndexNotReadyLeavesTextOut(t *testing.T) {
	notReady := errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady, errors.New("the word index is replaying"))
	wordIndexNotReady := func(t *testing.T, client *natsclient.Client) {
		serve(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject, func(context.Context, []byte) ([]byte, error) {
			return nil, notReady
		})
	}

	t.Run("the embedding search answers", func(t *testing.T) {
		client := fixtureClient(t)
		wordIndexNotReady(t, client)
		received := stubSemantic(t, client, semanticAnswer)
		startComponent(t, client, tierOneConfig)

		raw, err := askSearchGraph(t, client, fallbackQuestion)
		if err != nil {
			t.Fatalf("searchGraph: %v; want the semantic answer", err)
		}
		envelope, reply := decodeFallback(t, raw)
		if text, ok := reply.keys["text"]; !ok || string(text) != "null" {
			t.Errorf("data = %s, want text sent as null", envelope.Data)
		}
		var want bytes.Buffer
		if err := json.Compact(&want, []byte(semanticData)); err != nil {
			t.Fatal(err)
		}
		if string(reply.Semantic) != want.String() {
			t.Errorf("semantic = %s, want %s", reply.Semantic, want.String())
		}
		if wantHint := (graph.GlobalSearchHint{Hint: "semantic", Detail: "index_not_ready", Honoured: true}); reply.Fallback == nil || *reply.Fallback != wantHint {
			t.Errorf("fallback = %+v, want %+v", reply.Fallback, wantHint)
		}
		if !reflect.DeepEqual(reply.Hints, []graph.GlobalSearchHint{}) {
			t.Errorf("hints = %+v, want none", reply.Hints)
		}
		if envelope.IndexedRevision != 0 {
			t.Errorf("indexed_revision = %d, want 0: no word index was read", envelope.IndexedRevision)
		}
		requireOneSemanticRequest(t, received())
	})

	t.Run("the embedding search refuses", func(t *testing.T) {
		client := fixtureClient(t)
		wordIndexNotReady(t, client)
		received := stubSemantic(t, client, func() ([]byte, error) {
			return nil, errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady, errors.New("the embedding index is replaying"))
		})
		startComponent(t, client, tierOneConfig)

		raw, err := askSearchGraph(t, client, fallbackQuestion)
		got := requireRefusal(t, raw, err, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
		if !strings.Contains(got.Error(), "the word index is replaying") {
			t.Errorf("refusal %q, want the word search's own", got.Error())
		}
		requireOneSemanticRequest(t, received())
	})
}

// TestSearchGraphFallbackTimeoutKeepsWordAnswer: an embedding search that never answers holds the
// fallback for query_timeout_ms at most, and the reply is the word answer with fallback not
// honoured, code internal, naming the subject. The responder receives and never replies, so the
// request is not refused for want of a responder.
func TestSearchGraphFallbackTimeoutKeepsWordAnswer(t *testing.T) {
	client := fixtureClient(t)
	emptyWordSearch(t, client)
	semantic := verbRow(t, graph.ResponderGraphEmbedding, "search").Subject
	received := make(chan []byte, 1)
	if _, err := client.Subscribe(t.Context(), semantic, func(_ context.Context, msg *nats.Msg) { received <- msg.Data }); err != nil {
		t.Fatalf("subscribe %s: %v", semantic, err)
	}
	// query_timeout_ms is the deadline under test, low so the test does not wait the default 15 s;
	// it is long enough for the word search stub on the same server.
	c := startComponent(t, client, `{"embedding_enabled": true, "query_timeout_ms": 1000}`)

	raw, err := askSearchGraph(t, client, fallbackQuestion)
	if err != nil {
		t.Fatalf("searchGraph: %v; want the word answer", err)
	}
	envelope, reply := decodeFallback(t, raw)
	if reply.Text == nil {
		t.Fatalf("data = %s, want the empty text", envelope.Data)
	}
	if reply.Fallback == nil {
		t.Fatalf("data = %s, want a fallback naming the timeout", envelope.Data)
	}
	got := *reply.Fallback
	if got.Hint != "semantic" || got.Detail != "no_matches" || got.Honoured || got.Code != graph.ErrorCodeInternal ||
		!strings.Contains(got.Reason, semantic) {
		t.Errorf("fallback = %+v, want semantic, no_matches, not honoured, code internal, reason naming %s", got, semantic)
	}
	// graph-query published the request before it replied; the receive waits for its delivery.
	requireOneSemanticRequest(t, [][]byte{<-received})
	// ask counts an unanswered request before it returns, so before the reply above.
	if health := c.Health(); health.ErrorCount != 1 {
		t.Errorf("Health = %+v, want one error: the fallback had no answer", health)
	}
}
