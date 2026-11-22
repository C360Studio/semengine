//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// mergedSearchReply is a mergedSearch answer's data as the wire spells it, read by the test's own
// type, with every key kept so a key the reply leaves out is told from one it sends empty.
type mergedSearchReply struct {
	Results []struct {
		EntityID string  `json:"entity_id"`
		Score    float64 `json:"score"`
	} `json:"results"`
	Excluded         []string                `json:"excluded"`
	BodiesNotIndexed int                     `json:"bodies_not_indexed"`
	Merge            *graph.GlobalSearchHint `json:"merge"`
	keys             map[string]json.RawMessage
}

// ids is each result's entity ID, in the reply's order.
func (r mergedSearchReply) ids() []string {
	ids := make([]string, 0, len(r.Results))
	for _, result := range r.Results {
		ids = append(ids, result.EntityID)
	}
	return ids
}

// scores is each result's score, in the reply's order.
func (r mergedSearchReply) scores() []float64 {
	scores := make([]float64, 0, len(r.Results))
	for _, result := range r.Results {
		scores = append(scores, result.Score)
	}
	return scores
}

// requireMergedScores holds that the reply's scores are want, each within a rounding of the sum.
func requireMergedScores(t *testing.T, reply mergedSearchReply, want ...float64) {
	t.Helper()
	got := reply.scores()
	if len(got) != len(want) {
		t.Fatalf("scores = %v, want %v", got, want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-12 {
			t.Errorf("score %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// decodeMergedSearch decodes a mergedSearch success: the envelope strictly, then its data's fields
// and keys.
func decodeMergedSearch(t *testing.T, raw []byte) (wireEnvelope, mergedSearchReply) {
	t.Helper()
	envelope := decodeEnvelope(t, raw)
	var reply mergedSearchReply
	if err := json.Unmarshal(envelope.Data, &reply); err != nil {
		t.Fatalf("decode data %s: %v", envelope.Data, err)
	}
	if err := json.Unmarshal(envelope.Data, &reply.keys); err != nil {
		t.Fatalf("decode data keys %s: %v", envelope.Data, err)
	}
	return envelope, reply
}

// askMergedSearch sends request to mergedSearch.
func askMergedSearch(t *testing.T, client *natsclient.Client, request string) ([]byte, error) {
	t.Helper()
	return client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphQuery, "mergedSearch").Subject, []byte(request), requestTimeout)
}

// countRequests serves subject with a refusal and counts the requests it receives, each before it
// refuses, so before any reply graph-query sends after it.
func countRequests(t *testing.T, client *natsclient.Client, subject string) *atomic.Int32 {
	t.Helper()
	var received atomic.Int32
	serve(t, client, subject, func(context.Context, []byte) ([]byte, error) {
		received.Add(1)
		return nil, errors.New("no read is expected here")
	})
	return &received
}

// TestMergedSearchMatchesSearchGraphOrder: for the same question and limit, mergedSearch's result
// IDs are searchGraph's match IDs in order, searchGraph being the oracle: no hit is missing or
// damaged and every reply fits. Thirty word hits and thirty embedding hits, fifteen in both, asked
// at limits 10 and 100; at tier 1 the merge and at tier 0 the word list alone.
func TestMergedSearchMatchesSearchGraphOrder(t *testing.T) {
	var wordIDs, vectorIDs []string
	for i := 1; i <= 30; i++ {
		wordIDs = append(wordIDs, drone(i))
		vectorIDs = append(vectorIDs, drone(46-i)) // 45 down to 16
	}
	for _, tier := range []struct{ name, config string }{
		{"tier 1", tierOneConfig},
		{"tier 0", `{"embedding_enabled": false}`},
	} {
		t.Run(tier.name, func(t *testing.T) {
			client := fixtureClient(t)
			stubWords(t, client, wordHits(t, 30, wordIDs...))
			stubSemantic(t, client, func() ([]byte, error) { return semanticReply(semanticHits(vectorIDs...)), nil })
			stubBatchEcho(t, client)
			startComponent(t, client, tier.config)

			for _, limit := range []int{10, 100} {
				request := fmt.Sprintf(`{"query":"zzz qqq","limit":%d}`, limit)
				raw, err := askSearchGraph(t, client, request)
				if err != nil {
					t.Fatalf("searchGraph at %d: %v", limit, err)
				}
				_, oracle := decodeSearchGraph(t, raw)
				if oracle.Text.Missing != 0 {
					t.Fatalf("searchGraph at %d left %d hits missing; the oracle needs none", limit, oracle.Text.Missing)
				}
				var want []string
				for _, row := range oracle.ranked(t) {
					id, _, _ := strings.Cut(row, " ")
					want = append(want, id)
				}
				raw, err = askMergedSearch(t, client, request)
				if err != nil {
					t.Fatalf("mergedSearch at %d: %v", limit, err)
				}
				_, got := decodeMergedSearch(t, raw)
				if !slices.Equal(got.ids(), want) {
					t.Errorf("limit %d: mergedSearch = %q\nsearchGraph    %q", limit, got.ids(), want)
				}
				if got, wantScores := got.scores(), oracle.scores(t); !slices.Equal(got, wantScores) {
					t.Errorf("limit %d: mergedSearch scores = %v, searchGraph's %v", limit, got, wantScores)
				}
			}
		})
	}
}

// TestMergedSearchScopesBothSearches: the request's scope reaches the word search and the embedding
// search, each asked 100 deep, and its min_revision the word search alone, since the embedding
// search refuses one above 0.
func TestMergedSearchScopesBothSearches(t *testing.T) {
	client := fixtureClient(t)
	words := stubWords(t, client, wordHits(t, 1, drone(1)))
	semantic := stubSemantic(t, client, semanticAnswer)
	startComponent(t, client, tierOneConfig)

	if _, err := askMergedSearch(t, client, `{"query":"zzz qqq","limit":3,"scope":["acme.ops.docs"],"min_revision":5}`); err != nil {
		t.Fatalf("mergedSearch: %v", err)
	}
	// Each stub stored its request before it answered, and graph-query had both answers before it
	// replied.
	if got := words(); len(got) != 1 {
		t.Fatalf("the word search received %q, want one request", got)
	} else {
		requireSameJSON(t, got[0], `{"query":"zzz qqq","limit":100,"scope":["acme.ops.docs"],"min_revision":5}`)
	}
	if got := semantic(); len(got) != 1 {
		t.Fatalf("the embedding search received %q, want one request", got)
	} else {
		requireSameJSON(t, got[0], `{"query":"zzz qqq","limit":100,"scope":["acme.ops.docs"]}`)
	}
}

// TestMergedSearchDecodesStrictly: a request with an unknown key, bytes after it, a blank query, a
// limit outside 0 or 1 to 100, or a scope member that is not an entity-ID prefix is refused
// invalid_request before any request is sent, and counts no error.
func TestMergedSearchDecodesStrictly(t *testing.T) {
	client := fixtureClient(t)
	words := stubWords(t, client, wordHits(t, 1, drone(1)))
	semantic := stubSemantic(t, client, semanticAnswer)
	c := startComponent(t, client, tierOneConfig)

	for _, request := range []string{
		`{"query":"zzz qqq","level":1}`,
		`{"query":"zzz qqq"} {}`,
		`{"query":"  "}`,
		`{"query":"zzz qqq","limit":101}`,
		`{"query":"zzz qqq","limit":-1}`,
		`{"query":"zzz qqq","scope":["a..b"]}`,
		`{"query":"zzz qqq","scope":["","acme.ops","a.*"]}`,
	} {
		raw, err := askMergedSearch(t, client, request)
		requireRefusal(t, raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
	}
	// A request sent before a refusal would have reached its stub before graph-query replied.
	if got := words(); len(got) != 0 {
		t.Errorf("the word search received %q, want nothing", got)
	}
	if got := semantic(); len(got) != 0 {
		t.Errorf("the embedding search received %q, want nothing", got)
	}
	if health := c.Health(); health.ErrorCount != 0 {
		t.Errorf("Health = %+v, want no error: a bad request is the caller's", health)
	}
}

// TestMergedSearchReplyShape: at tier 1, with the word list [A] and the embedding list [B, A], the
// reply lists A then B with their merged scores, worked out by hand from 1 / (60 + place); carries
// the word search's excluded words, bodies_not_indexed and revision, and the larger damaged count;
// holds merge honoured and nothing else; and reads no entity.
func TestMergedSearchReplyShape(t *testing.T) {
	client := fixtureClient(t)
	a, b := drone(1), drone(2)
	wordReply, err := graph.NewQueryResponse(graph.ResponderGraphLexical, 9, graph.LexicalSearchData{
		Results: []graph.LexicalHit{{EntityID: a, Score: 4.5}}, Matched: 1, Excluded: []string{"valve"}, BodiesNotIndexed: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	wordReply.DamagedEntities = 1
	encodedWords, err := json.Marshal(wordReply)
	if err != nil {
		t.Fatal(err)
	}
	stubWords(t, client, encodedWords)
	stubSemantic(t, client, func() ([]byte, error) {
		return []byte(`{"data":` + semanticHits(b, a) + `,"indexed_revision":0,"damaged_entities":3,` +
			`"producer":"graph-embedding","timestamp":"2026-11-22T00:00:00Z"}`), nil
	})
	batch := countRequests(t, client, verbRow(t, graph.ResponderGraphIngest, "batch").Subject)
	entity := countRequests(t, client, verbRow(t, graph.ResponderGraphIngest, "entity").Subject)
	startComponent(t, client, tierOneConfig)

	raw, err := askMergedSearch(t, client, `{"query":"zzz qqq -valve"}`)
	if err != nil {
		t.Fatalf("mergedSearch: %v", err)
	}
	envelope, reply := decodeMergedSearch(t, raw)
	if got, want := reply.ids(), []string{a, b}; !slices.Equal(got, want) {
		t.Errorf("results = %q, want %q", got, want)
	}
	requireMergedScores(t, reply, 1.0/61+1.0/62, 1.0/61)
	if !slices.Equal(reply.Excluded, []string{"valve"}) || reply.BodiesNotIndexed != 2 {
		t.Errorf("excluded %q, bodies_not_indexed %d; want the word search's [valve], 2", reply.Excluded, reply.BodiesNotIndexed)
	}
	if reply.Merge == nil || *reply.Merge != mergedHint {
		t.Errorf("merge = %+v, want %+v", reply.Merge, mergedHint)
	}
	var keys []string
	for key := range reply.keys {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	if want := []string{"bodies_not_indexed", "excluded", "merge", "results"}; !slices.Equal(keys, want) {
		t.Errorf("data keys = %q, want %q", keys, want)
	}
	if envelope.IndexedRevision != 9 || envelope.DamagedEntities != 3 {
		t.Errorf("indexed_revision %d, damaged_entities %d; want the word search's 9 and the larger 3",
			envelope.IndexedRevision, envelope.DamagedEntities)
	}
	// A read graph-query sent would have reached its stub before graph-query replied.
	if n, m := batch.Load(), entity.Load(); n != 0 || m != 0 {
		t.Errorf("batch received %d requests and entity %d, want none: mergedSearch reads no entity", n, m)
	}
}

// TestMergedSearchTierZeroAsksOnlyWords: with embedding_enabled false, mergedSearch asks the word
// search once at the request's limit, answers its list with the word scores and no merge key, and
// sends the embedding search nothing, though it is served and would answer.
func TestMergedSearchTierZeroAsksOnlyWords(t *testing.T) {
	client := fixtureClient(t)
	words := stubWords(t, client, wordHits(t, 2, drone(1), drone(2)))
	semantic := stubSemantic(t, client, semanticAnswer)
	startComponent(t, client, `{"embedding_enabled": false}`)

	raw, err := askMergedSearch(t, client, `{"query":"zzz qqq","limit":7}`)
	if err != nil {
		t.Fatalf("mergedSearch: %v", err)
	}
	envelope, reply := decodeMergedSearch(t, raw)
	if got, want := reply.ids(), []string{drone(1), drone(2)}; !slices.Equal(got, want) {
		t.Errorf("results = %q, want %q", got, want)
	}
	requireMergedScores(t, reply, 2, 1)
	if _, ok := reply.keys["merge"]; ok {
		t.Errorf("data = %s, want no merge at tier 0", envelope.Data)
	}
	if envelope.IndexedRevision != 9 {
		t.Errorf("indexed_revision = %d, want the word search's 9", envelope.IndexedRevision)
	}
	if got := words(); len(got) != 1 {
		t.Fatalf("the word search received %q, want one request", got)
	} else {
		requireSameJSON(t, got[0], `{"query":"zzz qqq","limit":7}`)
	}
	// The stub records before it answers, and graph-query would have had its answer before it replied.
	if got := semantic(); len(got) != 0 {
		t.Errorf("the embedding search received %q, want nothing at tier 0", got)
	}
}

// startDeclaring starts graph-query at tier 1 with its Warns about a failed merge captured and its
// collectors on a registry of the test's own.
func startDeclaring(t *testing.T, client *natsclient.Client, message string) (*Component, *warnCapture, *metric.MetricsRegistry) {
	t.Helper()
	logs := &warnCapture{message: message}
	registry := metric.NewMetricsRegistry()
	built, err := CreateGraphQuery(json.RawMessage(tierOneConfig), component.Dependencies{
		NATSClient: client, Logger: slog.New(logs), MetricsRegistry: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	startLifecycle(t, "graph-query", built)
	return built.(*Component), logs, registry
}

// TestMergedSearchFailedMergeKeepsWords: when the embedding search refuses, the reply is the word
// list with the word scores, merge not honoured carrying the refusal's code and message, never a
// refusal. The degrade is declared once per reply: one Warn naming the verb, and one count on the
// merge-failed counter searchGraph adds to as well.
func TestMergedSearchFailedMergeKeepsWords(t *testing.T) {
	client := fixtureClient(t)
	stubWords(t, client, wordHits(t, 2, drone(1), drone(2)))
	stubSemantic(t, client, func() ([]byte, error) {
		return nil, errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeEmbeddingUnavailable, errors.New("the embedder is down"))
	})
	stubBatchEcho(t, client)
	c, logs, registry := startDeclaring(t, client, "mergedSearch answered the word list alone: the embedding search failed")

	raw, err := askMergedSearch(t, client, `{"query":"zzz qqq","limit":7}`)
	if err != nil {
		t.Fatalf("mergedSearch: %v; want the word answer with the failed merge named", err)
	}
	envelope, reply := decodeMergedSearch(t, raw)
	if got, want := reply.ids(), []string{drone(1), drone(2)}; !slices.Equal(got, want) {
		t.Errorf("results = %q, want %q", got, want)
	}
	requireMergedScores(t, reply, 2, 1)
	if reply.Merge == nil {
		t.Fatalf("data = %s, want a merge naming the refusal", envelope.Data)
	}
	if got := *reply.Merge; got.Hint != "semantic" || got.Detail != "merged" || got.Honoured ||
		got.Code != graph.ErrorCodeEmbeddingUnavailable || !strings.Contains(got.Reason, "the embedder is down") {
		t.Errorf("merge = %+v, want semantic, merged, not honoured, code %s, reason naming the refusal", got, graph.ErrorCodeEmbeddingUnavailable)
	}
	// graph-query logs, counts and adds to the counter before it replies.
	warns := logs.snapshot()
	if len(warns) != 1 {
		t.Fatalf("warnings = %+v, want one", warns)
	}
	if w := warns[0]; w.level != slog.LevelWarn || w.attrs["verb"] != "mergedSearch" ||
		w.attrs["code"] != graph.ErrorCodeEmbeddingUnavailable || !strings.Contains(w.attrs["error"], "the embedder is down") ||
		w.attrs["reason"] != "the reply names the failed merge, and the word list is the answer mergedSearch gives with no embedding search" {
		t.Errorf("warning = %+v, want Warn naming mergedSearch, the code, the failure and why answering is safe", w)
	}
	if count := counterValue(t, registry, mergeFailedFamily); count != 1 {
		t.Errorf("%s = %v after mergedSearch, want 1", mergeFailedFamily, count)
	}
	if health := c.Health(); health.ErrorCount != 0 {
		t.Errorf("Health = %+v, want no error: the refusal is the responder's answer", health)
	}

	if _, err := askSearchGraph(t, client, mergeQuestion); err != nil {
		t.Fatalf("searchGraph: %v", err)
	}
	if count := counterValue(t, registry, mergeFailedFamily); count != 2 {
		t.Errorf("%s = %v after searchGraph too, want 2: the two verbs share the counter", mergeFailedFamily, count)
	}
}

// TestMergedSearchEmbeddingWaitBound: with query_timeout_ms 15000, an embedding search that never
// answers holds the merge for 4 seconds at most, so a caller on natsclient's default timeout of 5
// seconds gets the word list, merge not honoured, code internal, naming the embedding subject. The
// responder receives and never replies, so the request is not refused for want of a responder. The
// wait is real: graph-query takes no clock, and a NATS request's timeout runs on real time.
func TestMergedSearchEmbeddingWaitBound(t *testing.T) {
	client := fixtureClient(t)
	stubWords(t, client, wordHits(t, 1, drone(1)))
	semantic := verbRow(t, graph.ResponderGraphEmbedding, "search").Subject
	received := make(chan []byte, 1)
	if _, err := client.Subscribe(t.Context(), semantic, func(_ context.Context, msg *nats.Msg) { received <- msg.Data }); err != nil {
		t.Fatalf("subscribe %s: %v", semantic, err)
	}
	startComponent(t, client, `{"embedding_enabled": true, "query_timeout_ms": 15000}`)

	// Timeout 0 is natsclient's default, the caller's budget the bound is chosen to fit inside.
	raw, err := client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphQuery, "mergedSearch").Subject,
		[]byte(`{"query":"zzz qqq"}`), 0)
	if err != nil {
		t.Fatalf("mergedSearch on the default timeout: %v; want the word answer", err)
	}
	envelope, reply := decodeMergedSearch(t, raw)
	if got, want := reply.ids(), []string{drone(1)}; !slices.Equal(got, want) {
		t.Errorf("results = %q, want %q", got, want)
	}
	if reply.Merge == nil {
		t.Fatalf("data = %s, want a merge naming the timeout", envelope.Data)
	}
	if got := *reply.Merge; got.Honoured || got.Code != graph.ErrorCodeInternal || !strings.Contains(got.Reason, semantic) {
		t.Errorf("merge = %+v, want not honoured, code internal, reason naming %s", got, semantic)
	}
	// graph-query published the request before it replied; the receive waits for its delivery.
	requireSameJSON(t, <-received, `{"query":"zzz qqq","limit":100}`)
}

// TestMergedSearchBrokenEmbeddingIDKeepsWords: an embedding hit whose ID is not a literal entity ID
// is the embedding index's broken reply: the answer is the word list with merge not honoured, code
// internal, one error counted and one count on the merge-failed counter, never a refusal.
func TestMergedSearchBrokenEmbeddingIDKeepsWords(t *testing.T) {
	client := fixtureClient(t)
	stubWords(t, client, wordHits(t, 1, drone(1)))
	stubSemantic(t, client, func() ([]byte, error) {
		return semanticReply(`{"results":[{"entity_id":"not-an-entity","similarity":0.9}],"unreadable_records":0,"failed_records":0}`), nil
	})
	c, _, registry := startDeclaring(t, client, "mergedSearch answered the word list alone: the embedding search failed")

	raw, err := askMergedSearch(t, client, mergeQuestion)
	if err != nil {
		t.Fatalf("mergedSearch: %v; want the word answer", err)
	}
	envelope, reply := decodeMergedSearch(t, raw)
	if got, want := reply.ids(), []string{drone(1)}; !slices.Equal(got, want) {
		t.Errorf("results = %q, want %q", got, want)
	}
	if reply.Merge == nil || reply.Merge.Honoured || reply.Merge.Code != graph.ErrorCodeInternal {
		t.Errorf("data = %s, want merge not honoured, code internal", envelope.Data)
	}
	// The broken reply is counted where it is read, before graph-query replies.
	if health := c.Health(); health.ErrorCount != 1 {
		t.Errorf("Health = %+v, want one error: the embedding reply was broken", health)
	}
	if count := counterValue(t, registry, mergeFailedFamily); count != 1 {
		t.Errorf("%s = %v, want 1", mergeFailedFamily, count)
	}
}

// TestMergedSearchBrokenWordIDRefused: a word hit whose ID is not a literal entity ID is the word
// index's broken reply, and mergedSearch is refused transient internal naming the word search's
// subject, with one error counted, as searchGraph refuses that reply.
func TestMergedSearchBrokenWordIDRefused(t *testing.T) {
	broken := wordHits(t, 1, "not-an-entity")
	lexical := verbRow(t, graph.ResponderGraphLexical, "search").Subject
	for _, tier := range []struct{ name, config string }{
		{"tier 1", tierOneConfig},
		{"tier 0", `{"embedding_enabled": false}`},
	} {
		t.Run(tier.name, func(t *testing.T) {
			client := fixtureClient(t)
			stubWords(t, client, broken)
			stubSemantic(t, client, semanticAnswer)
			c := startComponent(t, client, tier.config)

			for _, ask := range []func(*testing.T, *natsclient.Client, string) ([]byte, error){askSearchGraph, askMergedSearch} {
				raw, err := ask(t, client, mergeQuestion)
				got := requireRefusal(t, raw, err, errs.ErrorTransient, graph.ErrorCodeInternal)
				if !strings.Contains(got.Error(), lexical) {
					t.Errorf("refusal %q does not name %s", got.Error(), lexical)
				}
			}
			if health := c.Health(); health.ErrorCount != 2 {
				t.Errorf("Health = %+v, want one error for each verb's broken word reply", health)
			}
		})
	}
}

// TestMergedSearchOtherRefusalIsReturned: a word refusal other than index_not_ready is the reply as
// it arrived, though the embedding search was asked and never answers: the merge's request is
// cancelled, never awaited, and counts no error. The word stub refuses only once the embedding
// search has the request, so the request is in flight when the refusal arrives. That the handler
// waits for the cancelled request to end is TestStoppedSearchEndsWithoutItsReply's, the step both
// search verbs share.
func TestMergedSearchOtherRefusalIsReturned(t *testing.T) {
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

	raw, err := askMergedSearch(t, client, `{"query":"the of and"}`)
	got := requireRefusal(t, raw, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
	if !strings.Contains(got.Error(), "the query holds only stop words") {
		t.Errorf("refusal %q, want the word search's own", got.Error())
	}
	// The word stub refused only after the embedding stub counted its request.
	if n := received.Load(); n != 1 {
		t.Errorf("the embedding search received %d requests, want one", n)
	}
	if health := c.Health(); health.ErrorCount != 0 {
		t.Errorf("Health = %+v, want no error: a responder's refusal is an answer, and the cancelled request is no failure", health)
	}
}

// TestMergedSearchWordIndexNotReady: when the word search refuses index_not_ready and the embedding
// search answers, the results are the embedding list in its order, merge honoured with detail
// index_not_ready, under revision 0, since no word index was read. When the embedding search refuses
// too, the index_not_ready refusal is returned as it arrived.
func TestMergedSearchWordIndexNotReady(t *testing.T) {
	notReady := errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady, errors.New("the word index is replaying"))
	wordIndexNotReady := func(t *testing.T, client *natsclient.Client) {
		serve(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject, func(context.Context, []byte) ([]byte, error) {
			return nil, notReady
		})
	}

	t.Run("the embedding search answers", func(t *testing.T) {
		client := fixtureClient(t)
		wordIndexNotReady(t, client)
		stubSemantic(t, client, func() ([]byte, error) { return semanticReply(semanticHits(drone(2), drone(1))), nil })
		startComponent(t, client, tierOneConfig)

		raw, err := askMergedSearch(t, client, mergeQuestion)
		if err != nil {
			t.Fatalf("mergedSearch: %v; want the embedding list", err)
		}
		envelope, reply := decodeMergedSearch(t, raw)
		if got, want := reply.ids(), []string{drone(2), drone(1)}; !slices.Equal(got, want) {
			t.Errorf("results = %q, want %q", got, want)
		}
		requireMergedScores(t, reply, 1.0/61, 1.0/62)
		if want := (graph.GlobalSearchHint{Hint: "semantic", Detail: "index_not_ready", Honoured: true}); reply.Merge == nil || *reply.Merge != want {
			t.Errorf("merge = %+v, want %+v", reply.Merge, want)
		}
		if _, ok := reply.keys["excluded"]; ok {
			t.Errorf("data = %s, want no excluded: the word index was not read", envelope.Data)
		}
		if envelope.IndexedRevision != 0 {
			t.Errorf("indexed_revision = %d, want 0: no word index was read", envelope.IndexedRevision)
		}
	})

	t.Run("the embedding search refuses", func(t *testing.T) {
		client := fixtureClient(t)
		wordIndexNotReady(t, client)
		stubSemantic(t, client, func() ([]byte, error) {
			return nil, errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady, errors.New("the embedding index is replaying"))
		})
		startComponent(t, client, tierOneConfig)

		raw, err := askMergedSearch(t, client, mergeQuestion)
		got := requireRefusal(t, raw, err, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
		if !strings.Contains(got.Error(), "the word index is replaying") {
			t.Errorf("refusal %q, want the word search's own", got.Error())
		}
	})
}
