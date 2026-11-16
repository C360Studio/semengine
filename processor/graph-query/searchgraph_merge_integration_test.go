//go:build integration

package graphquery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// mergeQuestion is a question of several words with no hints, so global search reads no reference
// and lists no hint: the word and embedding searches are the only requests before the batch read.
const mergeQuestion = `{"query":"zzz qqq","limit":7}`

// semanticData is the embedding search's reply data in the stubs below: written out with odd
// spacing and an unknown field, so a reply that re-encoded it, or built it from SemanticData,
// would differ from the bytes the embedding search sent.
const semanticData = `{"results":[{"entity_id":"c360.test.query.system.drone.001","similarity":0.5}],` +
	`"embedder_type":"fixed","unreadable_records":0,"failed_records":0,"unknown_field":[1.5]}`

// searchGraphReply is a searchGraph answer's data as the wire spells it, read by the test's own
// type: text, semantic and merge by their keys, so it reads replies with and without them. Each
// match is kept as its JSON object, so a rank the reply leaves out is told from a rank of 0.
type searchGraphReply struct {
	Text *struct {
		Matches   []map[string]json.RawMessage `json:"matches"`
		Matched   int                          `json:"matched"`
		Missing   int                          `json:"missing"`
		Truncated bool                         `json:"truncated"`
		Answer    string                       `json:"answer"`
	} `json:"text"`
	Semantic json.RawMessage          `json:"semantic"`
	Merge    *graph.GlobalSearchHint  `json:"merge"`
	Hints    []graph.GlobalSearchHint `json:"hints"`
	keys     map[string]json.RawMessage
}

// decodeSearchGraph decodes a searchGraph success: the envelope strictly, then its data's keys and
// fields. A key the reply leaves out is absent from keys; one it sends as null is present.
func decodeSearchGraph(t *testing.T, raw []byte) (wireEnvelope, searchGraphReply) {
	t.Helper()
	envelope := decodeEnvelope(t, raw)
	var reply searchGraphReply
	if err := json.Unmarshal(envelope.Data, &reply); err != nil {
		t.Fatalf("decode data %s: %v", envelope.Data, err)
	}
	if err := json.Unmarshal(envelope.Data, &reply.keys); err != nil {
		t.Fatalf("decode data keys %s: %v", envelope.Data, err)
	}
	return envelope, reply
}

// ranked is each match as "<id> w<word_rank> v<vector_rank>", with "-" for a rank the match leaves
// out, in the reply's order.
func (r searchGraphReply) ranked(t *testing.T) []string {
	t.Helper()
	if r.Text == nil {
		t.Fatalf("reply has no text: %s", r.keys)
	}
	rows := make([]string, 0, len(r.Text.Matches))
	for _, match := range r.Text.Matches {
		var id string
		if err := json.Unmarshal(match["id"], &id); err != nil {
			t.Fatalf("match id %s: %v", match["id"], err)
		}
		rank := func(key string) string {
			raw, ok := match[key]
			if !ok {
				return "-"
			}
			return string(raw)
		}
		rows = append(rows, fmt.Sprintf("%s w%s v%s", id, rank("word_rank"), rank("vector_rank")))
	}
	return rows
}

// scores is each match's score, in the reply's order.
func (r searchGraphReply) scores(t *testing.T) []float64 {
	t.Helper()
	scores := make([]float64, 0, len(r.Text.Matches))
	for _, match := range r.Text.Matches {
		var score float64
		if err := json.Unmarshal(match["score"], &score); err != nil {
			t.Fatalf("match score %s: %v", match["score"], err)
		}
		scores = append(scores, score)
	}
	return scores
}

// requireScores holds that the reply's scores are want, each within a rounding of the sum.
func requireScores(t *testing.T, reply searchGraphReply, want ...float64) {
	t.Helper()
	got := reply.scores(t)
	if len(got) != len(want) {
		t.Fatalf("scores = %v, want %v", got, want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-12 {
			t.Errorf("score %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// drone is the test's nth drone ID, all in one level-0 group.
func drone(n int) string { return fmt.Sprintf("c360.test.query.system.drone.%03d", n) }

// short is a drone ID's last part, as ranked rows and the tests' literals name it.
func short(id string) string { return id[strings.LastIndex(id, ".")+1:] }

// shortRows is rows with each ID cut to its last part.
func shortRows(rows []string) []string {
	cut := make([]string, len(rows))
	for i, row := range rows {
		id, rest, _ := strings.Cut(row, " ")
		cut[i] = short(id) + " " + rest
	}
	return cut
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
	return semanticReply(semanticData), nil
}

// semanticReply is graph-embedding's search success holding data.
func semanticReply(data string) []byte {
	return []byte(`{"data":` + data + `,"indexed_revision":0,"producer":"graph-embedding","timestamp":"2026-11-16T00:00:00Z"}`)
}

// semanticHits is the embedding search's reply data listing ids, best first.
func semanticHits(ids ...string) string {
	results := make([]string, 0, len(ids))
	for i, id := range ids {
		results = append(results, fmt.Sprintf(`{"entity_id":%q,"similarity":%g}`, id, 0.99-float64(i)/1000))
	}
	return `{"results":[` + strings.Join(results, ",") + `],"embedder_type":"fixed","unreadable_records":0,"failed_records":0}`
}

// wordHits is the word index's reply at revision 9 listing ids, best first, of matched matches.
func wordHits(t *testing.T, matched int, ids ...string) []byte {
	t.Helper()
	hits := make([]graph.LexicalHit, 0, len(ids))
	for i, id := range ids {
		hits = append(hits, graph.LexicalHit{EntityID: id, Score: float64(len(ids) - i)})
	}
	return lexicalReply(t, 9, graph.LexicalSearchData{Results: hits, Matched: matched})
}

// stubWords stands in for graph-lexical's word search, answering every request with reply, and
// returns the requests it received, each stored before it answers.
func stubWords(t *testing.T, client *natsclient.Client, reply []byte) func() [][]byte {
	t.Helper()
	var received atomic.Pointer[[][]byte]
	received.Store(&[][]byte{})
	serve(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject, func(_ context.Context, data []byte) ([]byte, error) {
		got := append(append([][]byte(nil), *received.Load()...), data)
		received.Store(&got)
		return reply, nil
	})
	return func() [][]byte { return *received.Load() }
}

// stubBatchEcho stands in for graph-ingest's batch verb, answering every ID it is asked for with an
// entity titled by its ID.
func stubBatchEcho(t *testing.T, client *natsclient.Client) {
	t.Helper()
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "batch").Subject, func(_ context.Context, data []byte) ([]byte, error) {
		var request graph.EntityBatchRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		rows := make([]string, 0, len(request.IDs))
		for _, id := range request.IDs {
			rows = append(rows, titled(id, id))
		}
		return []byte(`{"entities":[` + strings.Join(rows, ",") + `],"missing":[]}`), nil
	})
}

// askSearchGraph sends request to searchGraph.
func askSearchGraph(t *testing.T, client *natsclient.Client, request string) ([]byte, error) {
	t.Helper()
	return client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphQuery, "searchGraph").Subject, []byte(request), requestTimeout)
}

// requireDeepRequests holds that a search was asked exactly once, with the question at the deepest
// limit, whatever the request's limit, and nothing else: no min_revision, which the embedding search
// refuses above 0 and the request did not send.
func requireDeepRequests(t *testing.T, name string, received [][]byte) {
	t.Helper()
	if len(received) != 1 {
		t.Fatalf("the %s search received %q, want one request", name, received)
	}
	requireSameJSON(t, received[0], `{"query":"zzz qqq","limit":100}`)
}

// mergedHint is the merge step honoured.
var mergedHint = graph.GlobalSearchHint{Hint: "semantic", Detail: "merged", Honoured: true}

// compacted is data as a reply's marshal writes a json.RawMessage, compacted.
func compacted(t *testing.T, data string) string {
	t.Helper()
	var want bytes.Buffer
	if err := json.Compact(&want, []byte(data)); err != nil {
		t.Fatal(err)
	}
	return want.String()
}

// TestSearchGraphMergesWordsAndMeaning: at tier 1, with the word list [A, B] and the embedding
// list [C, A], searchGraph asks both searches 100 deep, and answers A, C, B, each once, with each
// list's place as its rank and 1 / (60 + place) summed as its score, worked out by hand below;
// semantic as its bytes came, merge honoured, matched the word index's count, the answer naming the
// one match found by meaning alone, under the word search's revision.
func TestSearchGraphMergesWordsAndMeaning(t *testing.T) {
	client := fixtureClient(t)
	a, b, c := drone(1), drone(2), drone(3)
	words := stubWords(t, client, wordHits(t, 2, a, b))
	data := semanticHits(c, a)
	semantic := stubSemantic(t, client, func() ([]byte, error) { return semanticReply(data), nil })
	stubBatchEcho(t, client)
	startComponent(t, client, tierOneConfig)

	raw, err := askSearchGraph(t, client, mergeQuestion)
	if err != nil {
		t.Fatalf("searchGraph: %v", err)
	}
	envelope, reply := decodeSearchGraph(t, raw)
	if got, want := shortRows(reply.ranked(t)), []string{"001 w1 v2", "003 w- v1", "002 w2 v-"}; !slices.Equal(got, want) {
		t.Errorf("matches = %q, want %q", got, want)
	}
	requireScores(t, reply, 1.0/61+1.0/62, 1.0/61, 1.0/62)
	if reply.Merge == nil || *reply.Merge != mergedHint {
		t.Errorf("merge = %+v, want %+v", reply.Merge, mergedHint)
	}
	if got, want := string(reply.Semantic), compacted(t, data); got != want {
		t.Errorf("semantic = %s, want the embedding reply's data %s", got, want)
	}
	if reply.Text.Matched != 2 || reply.Text.Truncated {
		t.Errorf("matched %d, truncated %v; want the word index's 2, false", reply.Text.Matched, reply.Text.Truncated)
	}
	if want := "Found 2 matching entities in 1 groups, and 1 more by meaning."; reply.Text.Answer != want {
		t.Errorf("answer = %q, want %q", reply.Text.Answer, want)
	}
	if envelope.IndexedRevision != 9 {
		t.Errorf("indexed_revision = %d, want the word search's 9", envelope.IndexedRevision)
	}
	if _, ok := reply.keys["fallback"]; ok {
		t.Errorf("data = %s, want no fallback key", envelope.Data)
	}
	requireDeepRequests(t, "word", words())
	requireDeepRequests(t, "embedding", semantic())
}

// TestSearchGraphLimitIsAPrefix: the same question asked with limit 10 and limit 100, no hit
// missing and both replies fitting, answers the same first ten matches in the same order, with the
// same ranks and scores. Thirty word hits and thirty embedding hits, fifteen in both, so the merge
// at 100 holds all forty-five.
func TestSearchGraphLimitIsAPrefix(t *testing.T) {
	client := fixtureClient(t)
	var wordIDs, vectorIDs []string
	for i := 1; i <= 30; i++ {
		wordIDs = append(wordIDs, drone(i))
		vectorIDs = append(vectorIDs, drone(46-i)) // 45 down to 16
	}
	stubWords(t, client, wordHits(t, 30, wordIDs...))
	stubSemantic(t, client, func() ([]byte, error) { return semanticReply(semanticHits(vectorIDs...)), nil })
	stubBatchEcho(t, client)
	startComponent(t, client, tierOneConfig)

	ask := func(limit int) searchGraphReply {
		t.Helper()
		raw, err := askSearchGraph(t, client, fmt.Sprintf(`{"query":"zzz qqq","limit":%d}`, limit))
		if err != nil {
			t.Fatalf("searchGraph at %d: %v", limit, err)
		}
		_, reply := decodeSearchGraph(t, raw)
		return reply
	}
	ten, hundred := ask(10), ask(100)
	if len(hundred.Text.Matches) != 45 || hundred.Text.Truncated {
		t.Fatalf("limit 100: %d matches, truncated %v; want all 45, false", len(hundred.Text.Matches), hundred.Text.Truncated)
	}
	if len(ten.Text.Matches) != 10 || !ten.Text.Truncated {
		t.Fatalf("limit 10: %d matches, truncated %v; want 10, true", len(ten.Text.Matches), ten.Text.Truncated)
	}
	if got, want := ten.ranked(t), hundred.ranked(t)[:10]; !slices.Equal(got, want) {
		t.Errorf("limit 10 = %q\nlimit 100's first ten = %q", got, want)
	}
	if got, want := ten.scores(t), hundred.scores(t)[:10]; !slices.Equal(got, want) {
		t.Errorf("limit 10 scores = %v, limit 100's first ten = %v", got, want)
	}
}

// TestSearchGraphMeaningAloneWhenWordsMatchNothing: at tier 1, a question the word index matches
// nothing for answers the embedding list read back, each match with vector_rank only, merge
// honoured, under the word search's revision. globalSearch, asked the same, answers the empty text
// alone and sends the embedding search nothing.
func TestSearchGraphMeaningAloneWhenWordsMatchNothing(t *testing.T) {
	client := fixtureClient(t)
	emptyWordSearch(t, client)
	received := stubSemantic(t, client, semanticAnswer)
	stubBatchEcho(t, client)
	startComponent(t, client, tierOneConfig)

	raw, err := askSearchGraph(t, client, mergeQuestion)
	if err != nil {
		t.Fatalf("searchGraph: %v", err)
	}
	envelope, reply := decodeSearchGraph(t, raw)
	if got, want := shortRows(reply.ranked(t)), []string{"001 w- v1"}; !slices.Equal(got, want) {
		t.Errorf("matches = %q, want %q", got, want)
	}
	requireScores(t, reply, 1.0/61)
	if reply.Merge == nil || *reply.Merge != mergedHint {
		t.Errorf("merge = %+v, want %+v", reply.Merge, mergedHint)
	}
	if got, want := string(reply.Semantic), compacted(t, semanticData); got != want {
		t.Errorf("semantic = %s, want %s", got, want)
	}
	if want := "Found 0 matching entities in 1 groups, and 1 more by meaning."; reply.Text.Answer != want {
		t.Errorf("answer = %q, want %q", reply.Text.Answer, want)
	}
	if envelope.IndexedRevision != 9 {
		t.Errorf("indexed_revision = %d, want the word search's 9", envelope.IndexedRevision)
	}
	requireDeepRequests(t, "embedding", received())

	global, err := client.RequestClassified(t.Context(), globalSearchSubject, []byte(mergeQuestion), requestTimeout)
	if err != nil {
		t.Fatalf("globalSearch: %v", err)
	}
	requireSameJSON(t, decodeEnvelope(t, global).Data, `{"text":`+emptyText+`,"hints":[]}`)
	if got := received(); len(got) != 1 {
		t.Errorf("after globalSearch the embedding search received %d requests, want still one", len(got))
	}
}

// TestSearchGraphTierZeroAsksNoEmbedding: with embedding_enabled false, searchGraph answers a
// question the word index matches nothing for with the empty text alone, no semantic and no merge,
// and sends the embedding search nothing, though it is served and would answer.
func TestSearchGraphTierZeroAsksNoEmbedding(t *testing.T) {
	client := fixtureClient(t)
	emptyWordSearch(t, client)
	received := stubSemantic(t, client, semanticAnswer)
	startComponent(t, client, `{"embedding_enabled": false}`)

	raw, err := askSearchGraph(t, client, mergeQuestion)
	if err != nil {
		t.Fatalf("searchGraph: %v", err)
	}
	envelope, reply := decodeSearchGraph(t, raw)
	requireSameJSON(t, envelope.Data, `{"text":`+emptyText+`,"hints":[]}`)
	if envelope.IndexedRevision != 9 {
		t.Errorf("indexed_revision = %d, want the word search's 9", envelope.IndexedRevision)
	}
	if reply.Merge != nil || reply.Semantic != nil {
		t.Errorf("data = %s, want no merge and no semantic", envelope.Data)
	}
	// The stub records before it answers, and graph-query would have had its answer before it replied.
	if got := received(); len(got) != 0 {
		t.Errorf("the embedding search received %q, want nothing at tier 0", got)
	}
}

// TestSearchGraphNamesAFailedMerge: when the embedding search refuses, the reply is the word list
// in its own order, each match with word_rank only and the word index's score, with merge not
// honoured, carrying the refusal's code and message, and no semantic: never a refusal. truncated
// counts the word list read: two hits at limit 1 is cut, at limit 7 whole.
func TestSearchGraphNamesAFailedMerge(t *testing.T) {
	client := fixtureClient(t)
	stubWords(t, client, wordHits(t, 2, drone(1), drone(2)))
	refusal := errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeEmbeddingUnavailable, errors.New("the embedder is down"))
	stubSemantic(t, client, func() ([]byte, error) { return nil, refusal })
	stubBatchEcho(t, client)
	startComponent(t, client, tierOneConfig)

	for _, tc := range []struct {
		limit     int
		rows      []string
		scores    []float64
		truncated bool
		answer    string
	}{
		{1, []string{"001 w1 v-"}, []float64{2}, true, "Found 2 matching entities in 1 groups. Showing the first 1."},
		{7, []string{"001 w1 v-", "002 w2 v-"}, []float64{2, 1}, false, "Found 2 matching entities in 1 groups."},
	} {
		raw, err := askSearchGraph(t, client, fmt.Sprintf(`{"query":"zzz qqq","limit":%d}`, tc.limit))
		if err != nil {
			t.Fatalf("searchGraph at %d: %v; want the word answer with the failed merge named", tc.limit, err)
		}
		envelope, reply := decodeSearchGraph(t, raw)
		if got := shortRows(reply.ranked(t)); !slices.Equal(got, tc.rows) {
			t.Errorf("limit %d: matches = %q, want %q", tc.limit, got, tc.rows)
		}
		requireScores(t, reply, tc.scores...)
		if reply.Text.Truncated != tc.truncated || reply.Text.Answer != tc.answer {
			t.Errorf("limit %d: truncated %v, answer %q; want %v, %q", tc.limit, reply.Text.Truncated, reply.Text.Answer, tc.truncated, tc.answer)
		}
		if reply.Merge == nil {
			t.Fatalf("data = %s, want a merge naming the refusal", envelope.Data)
		}
		got := *reply.Merge
		if got.Hint != "semantic" || got.Detail != "merged" || got.Honoured || got.Code != graph.ErrorCodeEmbeddingUnavailable ||
			!strings.Contains(got.Reason, "the embedder is down") {
			t.Errorf("merge = %+v, want semantic, merged, not honoured, code %s, reason naming the refusal",
				got, graph.ErrorCodeEmbeddingUnavailable)
		}
		if _, ok := reply.keys["semantic"]; ok {
			t.Errorf("data = %s, want no semantic", envelope.Data)
		}
	}
}

// TestSearchGraphMergeTimeoutKeepsWordAnswer: an embedding search that never answers holds the
// merge for query_timeout_ms at most, and the reply is the word answer with merge not honoured,
// code internal, naming the subject. The responder receives and never replies, so the request is not
// refused for want of a responder.
func TestSearchGraphMergeTimeoutKeepsWordAnswer(t *testing.T) {
	client := fixtureClient(t)
	stubWords(t, client, wordHits(t, 1, drone(1)))
	stubBatchEcho(t, client)
	semantic := verbRow(t, graph.ResponderGraphEmbedding, "search").Subject
	received := make(chan []byte, 1)
	if _, err := client.Subscribe(t.Context(), semantic, func(_ context.Context, msg *nats.Msg) { received <- msg.Data }); err != nil {
		t.Fatalf("subscribe %s: %v", semantic, err)
	}
	// query_timeout_ms is the deadline under test, low so the test does not wait the default 15 s;
	// it is long enough for the word search and batch stubs on the same server.
	c := startComponent(t, client, `{"embedding_enabled": true, "query_timeout_ms": 1000}`)

	raw, err := askSearchGraph(t, client, mergeQuestion)
	if err != nil {
		t.Fatalf("searchGraph: %v; want the word answer", err)
	}
	envelope, reply := decodeSearchGraph(t, raw)
	if got, want := shortRows(reply.ranked(t)), []string{"001 w1 v-"}; !slices.Equal(got, want) {
		t.Errorf("matches = %q, want %q", got, want)
	}
	if reply.Merge == nil {
		t.Fatalf("data = %s, want a merge naming the timeout", envelope.Data)
	}
	got := *reply.Merge
	if got.Hint != "semantic" || got.Detail != "merged" || got.Honoured || got.Code != graph.ErrorCodeInternal ||
		!strings.Contains(got.Reason, semantic) {
		t.Errorf("merge = %+v, want semantic, merged, not honoured, code internal, reason naming %s", got, semantic)
	}
	// graph-query published the request before it replied; the receive waits for its delivery.
	requireDeepRequests(t, "embedding", [][]byte{<-received})
	// The merge reads the unanswered request's error before the reply above, and counts it there.
	if health := c.Health(); health.ErrorCount != 1 {
		t.Errorf("Health = %+v, want one error: the embedding search had no answer", health)
	}
}

// TestSearchGraphIndexNotReadyLeavesTextOut: when the word search refuses index_not_ready and the
// embedding search answers, the reply sends text as null (the word index was not read), semantic,
// and merge honoured with detail index_not_ready. When the embedding search refuses too, the
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

		raw, err := askSearchGraph(t, client, mergeQuestion)
		if err != nil {
			t.Fatalf("searchGraph: %v; want the semantic answer", err)
		}
		envelope, reply := decodeSearchGraph(t, raw)
		if text, ok := reply.keys["text"]; !ok || string(text) != "null" {
			t.Errorf("data = %s, want text sent as null", envelope.Data)
		}
		if got, want := string(reply.Semantic), compacted(t, semanticData); got != want {
			t.Errorf("semantic = %s, want %s", got, want)
		}
		if wantHint := (graph.GlobalSearchHint{Hint: "semantic", Detail: "index_not_ready", Honoured: true}); reply.Merge == nil || *reply.Merge != wantHint {
			t.Errorf("merge = %+v, want %+v", reply.Merge, wantHint)
		}
		if !reflect.DeepEqual(reply.Hints, []graph.GlobalSearchHint{}) {
			t.Errorf("hints = %+v, want none", reply.Hints)
		}
		if envelope.IndexedRevision != 0 {
			t.Errorf("indexed_revision = %d, want 0: no word index was read", envelope.IndexedRevision)
		}
		requireDeepRequests(t, "embedding", received())
	})

	t.Run("the embedding search refuses", func(t *testing.T) {
		client := fixtureClient(t)
		wordIndexNotReady(t, client)
		received := stubSemantic(t, client, func() ([]byte, error) {
			return nil, errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady, errors.New("the embedding index is replaying"))
		})
		startComponent(t, client, tierOneConfig)

		raw, err := askSearchGraph(t, client, mergeQuestion)
		got := requireRefusal(t, raw, err, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
		if !strings.Contains(got.Error(), "the word index is replaying") {
			t.Errorf("refusal %q, want the word search's own", got.Error())
		}
		requireDeepRequests(t, "embedding", received())
	})
}

// seenOnce is a request a stub has received: closed by the first, once.
type seenOnce struct {
	once sync.Once
	seen chan struct{}
}

func newSeenOnce() *seenOnce { return &seenOnce{seen: make(chan struct{})} }

func (s *seenOnce) mark() { s.once.Do(func() { close(s.seen) }) }

// TestSearchGraphOtherRefusalIsReturned: a word refusal other than index_not_ready is the reply as
// it arrived, though the embedding search was asked and never answers: the merge's request is
// cancelled, never awaited, and counts no error. The word stub refuses only once the embedding
// search has the request, so the request is in flight when the refusal arrives; the embedding stub
// holds it until the test ends. That the call waits for the cancelled request to end, and leaves
// nothing running, is TestStoppedSearchEndsWithoutItsReply's.
func TestSearchGraphOtherRefusalIsReturned(t *testing.T) {
	client := fixtureClient(t)
	asked := newSeenOnce()
	serve(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject, func(ctx context.Context, _ []byte) ([]byte, error) {
		select {
		case <-asked.seen:
			return nil, errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, errors.New("the query holds only stop words"))
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	release := make(chan struct{})
	var received atomic.Int32
	serve(t, client, verbRow(t, graph.ResponderGraphEmbedding, "search").Subject, func(ctx context.Context, _ []byte) ([]byte, error) {
		received.Add(1)
		asked.mark()
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, errors.New("released")
	})
	c := startComponent(t, client, tierOneConfig)
	t.Cleanup(func() { close(release) })

	raw, err := askSearchGraph(t, client, `{"query":"the of and"}`)
	got := requireRefusal(t, raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
	if !strings.Contains(got.Error(), "the query holds only stop words") {
		t.Errorf("refusal %q, want the word search's own", got.Error())
	}
	// The word stub refused only after the embedding stub counted its request.
	if n := received.Load(); n != 1 {
		t.Errorf("the embedding search received %d requests, want one", n)
	}
	// Counted errors are graph-query's own, and the refusal is returned only once the embedding
	// request has ended.
	if health := c.Health(); health.ErrorCount != 0 {
		t.Errorf("Health = %+v, want no error: a responder's refusal is an answer, and the cancelled request is no failure", health)
	}
}

// TestSearchGraphAsksBothAtOnce: each search answers only after the other has been asked, and
// searchGraph answers the merge, so it sent both before waiting on either reply.
func TestSearchGraphAsksBothAtOnce(t *testing.T) {
	client := fixtureClient(t)
	wordAsked, semanticAsked := newSeenOnce(), newSeenOnce()
	wordReply := wordHits(t, 1, drone(1))
	serve(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject, func(ctx context.Context, _ []byte) ([]byte, error) {
		wordAsked.mark()
		select {
		case <-semanticAsked.seen:
			return wordReply, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	serve(t, client, verbRow(t, graph.ResponderGraphEmbedding, "search").Subject, func(ctx context.Context, _ []byte) ([]byte, error) {
		semanticAsked.mark()
		select {
		case <-wordAsked.seen:
			return semanticReply(semanticHits(drone(2))), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	stubBatchEcho(t, client)
	startComponent(t, client, tierOneConfig)

	raw, err := askSearchGraph(t, client, mergeQuestion)
	if err != nil {
		t.Fatalf("searchGraph: %v; want the merge of both answers", err)
	}
	_, reply := decodeSearchGraph(t, raw)
	if got, want := shortRows(reply.ranked(t)), []string{"001 w1 v-", "002 w- v1"}; !slices.Equal(got, want) {
		t.Errorf("matches = %q, want %q", got, want)
	}
	if reply.Merge == nil || *reply.Merge != mergedHint {
		t.Errorf("merge = %+v, want %+v", reply.Merge, mergedHint)
	}
}

// TestSearchGraphMergedTruncation: at tier 1 the word search returns 3 hits and the embedding list
// holds 12 entities, 2 of them among those 3, at limit 10: 13 entities found and 10 sent, so
// matched is 3 and truncated true. The order, worked out by hand from 1 / (60 + place): w2 (word 2,
// vector 1), w1 (word 1, vector 3), e1 (vector 2), w3 (word 3), then e2 to e7 (vector 4 to 9); seven
// of the ten have no word_rank.
func TestSearchGraphMergedTruncation(t *testing.T) {
	client := fixtureClient(t)
	w1, w2, w3 := drone(1), drone(2), drone(3)
	vectors := []string{w2, drone(101), w1}
	for i := 102; i <= 110; i++ {
		vectors = append(vectors, drone(i))
	}
	stubWords(t, client, wordHits(t, 3, w1, w2, w3))
	stubSemantic(t, client, func() ([]byte, error) { return semanticReply(semanticHits(vectors...)), nil })
	stubBatchEcho(t, client)
	startComponent(t, client, tierOneConfig)

	raw, err := askSearchGraph(t, client, `{"query":"zzz qqq","limit":10}`)
	if err != nil {
		t.Fatalf("searchGraph: %v", err)
	}
	_, reply := decodeSearchGraph(t, raw)
	want := []string{
		"002 w2 v1", "001 w1 v3", "101 w- v2", "003 w3 v-",
		"102 w- v4", "103 w- v5", "104 w- v6", "105 w- v7", "106 w- v8", "107 w- v9",
	}
	if got := shortRows(reply.ranked(t)); !slices.Equal(got, want) {
		t.Errorf("matches = %q\nwant      %q", got, want)
	}
	if reply.Text.Matched != 3 || !reply.Text.Truncated {
		t.Errorf("matched %d, truncated %v; want 3, true", reply.Text.Matched, reply.Text.Truncated)
	}
	if want := "Found 3 matching entities in 1 groups, and 7 more by meaning. Showing the first 10."; reply.Text.Answer != want {
		t.Errorf("answer = %q, want %q", reply.Text.Answer, want)
	}
}

// TestSearchGraphBrokenEmbeddingReplyKeepsWordAnswer: an embedding reply graph-query cannot use, a
// hit whose ID names no entity group or data that is no search reply, is the embedding index's
// broken reply: the answer is the word list with merge not honoured, code internal, and one error
// counted, never a refusal.
func TestSearchGraphBrokenEmbeddingReplyKeepsWordAnswer(t *testing.T) {
	for _, tc := range []struct{ name, data string }{
		{"a hit naming no group", `{"results":[{"entity_id":"not-an-entity","similarity":0.9}],"unreadable_records":0,"failed_records":0}`},
		{"data that is no search reply", `[1,2]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fixtureClient(t)
			stubWords(t, client, wordHits(t, 1, drone(1)))
			stubSemantic(t, client, func() ([]byte, error) { return semanticReply(tc.data), nil })
			stubBatchEcho(t, client)
			c := startComponent(t, client, tierOneConfig)

			raw, err := askSearchGraph(t, client, mergeQuestion)
			if err != nil {
				t.Fatalf("searchGraph: %v; want the word answer", err)
			}
			envelope, reply := decodeSearchGraph(t, raw)
			if got, want := shortRows(reply.ranked(t)), []string{"001 w1 v-"}; !slices.Equal(got, want) {
				t.Errorf("matches = %q, want %q", got, want)
			}
			if reply.Merge == nil || reply.Merge.Honoured || reply.Merge.Code != graph.ErrorCodeInternal {
				t.Errorf("merge = %+v, want not honoured, code internal", reply.Merge)
			}
			if _, ok := reply.keys["semantic"]; ok {
				t.Errorf("data = %s, want no semantic", envelope.Data)
			}
			// The broken reply is counted where it is read, before graph-query replies.
			if health := c.Health(); health.ErrorCount != 1 {
				t.Errorf("Health = %+v, want one error: the embedding reply was broken", health)
			}
		})
	}
}
