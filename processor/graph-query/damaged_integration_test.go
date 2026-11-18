//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// countedReply is an index verb's success as graph-index, graph-lexical or graph-embedding sends it
// while it holds damaged entities: data, the revision, and damaged_entities, left off at 0 as the
// producers leave it off.
func countedReply(producer, data string, revision uint64, damaged int) string {
	count := ""
	if damaged > 0 {
		count = fmt.Sprintf(`"damaged_entities":%d,`, damaged)
	}
	return fmt.Sprintf(`{"data":%s,"indexed_revision":%d,%s"producer":%q,"timestamp":"2026-11-18T00:00:00Z"}`,
		data, revision, count, producer)
}

// replyWith stands in for a verb, answering every request with reply.
func replyWith(t *testing.T, client *natsclient.Client, responder, verb, reply string) {
	t.Helper()
	serve(t, client, verbRow(t, responder, verb).Subject, func(context.Context, []byte) ([]byte, error) {
		return []byte(reply), nil
	})
}

// damagedCount is the damaged_entities a graph-query reply carries, and whether the key is there at
// all: a reply that left nothing out carries none.
func damagedCount(t *testing.T, raw []byte, err error) (int, bool) {
	t.Helper()
	if err != nil {
		t.Fatalf("refused: %v, want an answer", err)
	}
	decodeEnvelope(t, raw)
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	value, ok := keys["damaged_entities"]
	if !ok {
		return 0, false
	}
	var count int
	if err := json.Unmarshal(value, &count); err != nil {
		t.Fatalf("damaged_entities %s: %v", value, err)
	}
	return count, true
}

// requireDamaged holds that a reply carries damaged_entities want, or no such key when want is 0.
func requireDamaged(t *testing.T, raw []byte, err error, want int) {
	t.Helper()
	got, present := damagedCount(t, raw, err)
	switch {
	case want == 0 && present:
		t.Errorf("damaged_entities = %d, want the key absent: %s", got, raw)
	case want > 0 && got != want:
		t.Errorf("damaged_entities = %d (present %v), want %d: %s", got, present, want, raw)
	}
}

// emptyEdges and emptyWords are index reply data that list nothing: the count is the subject here,
// not the rows.
const (
	emptyEdges = `{"relationships":[]}`
	emptyWords = `{"results":[],"matched":0}`
)

// TestComposedReplyCarriesLargestDamagedCount: each reply graph-query builds from index replies
// carries the largest damaged_entities among them, whichever read carried it, and none when every
// read carried none. The index replies are stubbed: no component reports damage yet, and a real
// index cannot be made to answer each read with a chosen count. A verb that reads one index reply
// carries that reply's count.
func TestComposedReplyCarriesLargestDamagedCount(t *testing.T) {
	ask := func(t *testing.T, client *natsclient.Client, verb, request string) ([]byte, error) {
		t.Helper()
		return client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphQuery, verb).Subject, []byte(request), requestTimeout)
	}

	for _, direction := range []string{"outgoing", "incoming"} {
		t.Run("relationships "+direction, func(t *testing.T) {
			client := fixtureClient(t)
			replyWith(t, client, graph.ResponderGraphIndex, direction, countedReply("graph-index", emptyEdges, 4, 3))
			startComponent(t, client, `{}`)
			raw, err := ask(t, client, "relationships", fmt.Sprintf(`{"entity_id":%q,"direction":%q}`, drone1, direction))
			requireDamaged(t, raw, err, 3)
		})
	}

	t.Run("entityByAlias", func(t *testing.T) {
		client := fixtureClient(t)
		replyWith(t, client, graph.ResponderGraphIndex, "alias", countedReply("graph-index", `{"canonical_id":"`+drone1+`"}`, 4, 3))
		replyWith(t, client, graph.ResponderGraphIngest, "entity", entityReply(drone1))
		startComponent(t, client, `{}`)
		raw, err := ask(t, client, "entityByAlias", `{"alias":"held-1"}`)
		requireDamaged(t, raw, err, 3)
	})

	t.Run("summary", func(t *testing.T) {
		client := fixtureClient(t)
		replyWith(t, client, graph.ResponderGraphIndex, "predicateList", countedReply("graph-index", `{"predicates":[]}`, 4, 3))
		replyWith(t, client, graph.ResponderGraphIngest, "prefix", `{"entities":null,"total":0}`)
		startComponent(t, client, `{}`)
		raw, err := ask(t, client, "summary", `{}`)
		requireDamaged(t, raw, err, 3)
	})

	// Two hops, so two index replies; the larger count is first, then second, in the walk.
	for _, counts := range [][2]int{{2, 3}, {3, 2}, {0, 0}} {
		t.Run(fmt.Sprintf("pathSearch hops at %d and %d", counts[0], counts[1]), func(t *testing.T) {
			client := fixtureClient(t)
			serveEntity(t, client)
			serveHop(t, client, "outgoing", map[string]stubAnswer{
				drone1: {reply: countedReply("graph-index", `{"relationships":[`+outgoingRow(drone2)+`]}`, 4, counts[0])},
				drone2: {reply: countedReply("graph-index", emptyEdges, 4, counts[1])},
			})
			startComponent(t, client, `{}`)
			raw, err := ask(t, client, "pathSearch", fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":2}`, drone1))
			requireDamaged(t, raw, err, max(counts[0], counts[1]))
		})
	}

	t.Run("localSearch", func(t *testing.T) {
		client := fixtureClient(t)
		replyWith(t, client, graph.ResponderGraphLexical, "search", countedReply("graph-lexical", emptyWords, 4, 3))
		startComponent(t, client, `{}`)
		raw, err := ask(t, client, "localSearch", fmt.Sprintf(`{"entity_id":%q,"query":"drone"}`, drone1))
		requireDamaged(t, raw, err, 3)
	})

	// A one-word question is an entity hint, read through the alias verb: the word search and the
	// alias read are both index replies the answer is built from.
	for _, counts := range [][2]int{{2, 3}, {3, 2}, {0, 0}} {
		t.Run(fmt.Sprintf("globalSearch words at %d, alias at %d", counts[0], counts[1]), func(t *testing.T) {
			client := fixtureClient(t)
			replyWith(t, client, graph.ResponderGraphLexical, "search", countedReply("graph-lexical", emptyWords, 4, counts[0]))
			replyWith(t, client, graph.ResponderGraphIndex, "alias", countedReply("graph-index", `{"canonical_id":"`+drone1+`"}`, 4, counts[1]))
			replyWith(t, client, graph.ResponderGraphIngest, "entity", entityReply(drone1))
			startComponent(t, client, `{}`)
			raw, err := ask(t, client, "globalSearch", `{"query":"held-1"}`)
			requireDamaged(t, raw, err, max(counts[0], counts[1]))
		})
	}

	// At tier 1 searchGraph merges the word search and the embedding search.
	for _, counts := range [][2]int{{2, 3}, {3, 2}, {0, 0}} {
		t.Run(fmt.Sprintf("searchGraph words at %d, embedding at %d", counts[0], counts[1]), func(t *testing.T) {
			client := fixtureClient(t)
			replyWith(t, client, graph.ResponderGraphLexical, "search", countedReply("graph-lexical", emptyWords, 4, counts[0]))
			replyWith(t, client, graph.ResponderGraphEmbedding, "search", countedReply("graph-embedding", semanticHits(), 0, counts[1]))
			startComponent(t, client, tierOneConfig)
			raw, err := ask(t, client, "searchGraph", mergeQuestion)
			requireDamaged(t, raw, err, max(counts[0], counts[1]))
		})
	}

	// With the word index not ready, searchGraph answers from the embedding search alone, so its
	// count is the one read.
	t.Run("searchGraph from the embedding search alone", func(t *testing.T) {
		client := fixtureClient(t)
		serve(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject, func(context.Context, []byte) ([]byte, error) {
			return nil, errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady, errors.New("replaying"))
		})
		replyWith(t, client, graph.ResponderGraphEmbedding, "search", countedReply("graph-embedding", semanticHits(), 0, 3))
		startComponent(t, client, tierOneConfig)
		raw, err := ask(t, client, "searchGraph", mergeQuestion)
		requireDamaged(t, raw, err, 3)
	})
}

// TestSearchTruncatedWhileDamaged: a search reply built from a reply that carried damaged_entities
// above 0 says truncated, though every hit was read back and nothing else was left out: an index that
// left a damaged entity out may have left out a match. The same reply with no count says not
// truncated, which shows the count is what sets it. The replies are stubbed, as above.
func TestSearchTruncatedWhileDamaged(t *testing.T) {
	ask := func(t *testing.T, client *natsclient.Client, verb, request string) ([]byte, error) {
		t.Helper()
		return client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphQuery, verb).Subject, []byte(request), requestTimeout)
	}
	// One hit, matched 1, which the batch reads back: nothing but the count can make it truncated.
	oneHit := `{"results":[{"entity_id":"` + drone1 + `","score":1}],"matched":1}`
	// requireText holds a reply's damaged_entities and its text's truncated.
	requireText := func(t *testing.T, raw []byte, err error, damaged int, truncated bool, read func([]byte) bool) {
		t.Helper()
		requireDamaged(t, raw, err, damaged)
		if got := read(decodeEnvelope(t, raw).Data); got != truncated {
			t.Errorf("truncated = %v, want %v: %s", got, truncated, raw)
		}
	}
	localTruncated := func(data []byte) bool {
		var reply graph.LocalSearchData
		if err := json.Unmarshal(data, &reply); err != nil {
			t.Fatalf("decode %s: %v", data, err)
		}
		return reply.Truncated
	}
	textTruncated := func(data []byte) bool {
		var reply graph.GlobalSearchData
		if err := json.Unmarshal(data, &reply); err != nil || reply.Text == nil {
			t.Fatalf("decode %s: %v, want a text answer", data, err)
		}
		return reply.Text.Truncated
	}

	for _, damaged := range []int{1, 0} {
		t.Run(fmt.Sprintf("localSearch, the word index at %d", damaged), func(t *testing.T) {
			client := fixtureClient(t)
			replyWith(t, client, graph.ResponderGraphLexical, "search", countedReply("graph-lexical", oneHit, 4, damaged))
			stubBatchEcho(t, client)
			startComponent(t, client, `{}`)
			raw, err := ask(t, client, "localSearch", fmt.Sprintf(`{"entity_id":%q,"query":"drone"}`, drone1))
			requireText(t, raw, err, damaged, damaged > 0, localTruncated)
		})
		t.Run(fmt.Sprintf("globalSearch, the word index at %d", damaged), func(t *testing.T) {
			client := fixtureClient(t)
			replyWith(t, client, graph.ResponderGraphLexical, "search", countedReply("graph-lexical", oneHit, 4, damaged))
			stubBatchEcho(t, client)
			startComponent(t, client, `{}`)
			raw, err := ask(t, client, "globalSearch", mergeQuestion)
			requireText(t, raw, err, damaged, damaged > 0, textTruncated)
		})
	}

	// The count comes from the alias read of an entity hint, not the word search: the reply was built
	// from it too.
	t.Run("globalSearch, the alias read at 3", func(t *testing.T) {
		client := fixtureClient(t)
		replyWith(t, client, graph.ResponderGraphLexical, "search", countedReply("graph-lexical", oneHit, 4, 0))
		replyWith(t, client, graph.ResponderGraphIndex, "alias", countedReply("graph-index", `{"canonical_id":"`+drone1+`"}`, 4, 3))
		replyWith(t, client, graph.ResponderGraphIngest, "entity", entityReply(drone1))
		stubBatchEcho(t, client)
		startComponent(t, client, `{}`)
		raw, err := ask(t, client, "globalSearch", `{"query":"held-1"}`)
		requireText(t, raw, err, 3, true, textTruncated)
	})

	// At tier 1 both searches read back their one hit, the same entity; the embedding reply carries
	// the count.
	for _, damaged := range []int{2, 0} {
		t.Run(fmt.Sprintf("searchGraph, the embedding index at %d", damaged), func(t *testing.T) {
			client := fixtureClient(t)
			replyWith(t, client, graph.ResponderGraphLexical, "search", countedReply("graph-lexical", oneHit, 4, 0))
			replyWith(t, client, graph.ResponderGraphEmbedding, "search", countedReply("graph-embedding", semanticHits(drone1), 0, damaged))
			stubBatchEcho(t, client)
			startComponent(t, client, tierOneConfig)
			raw, err := ask(t, client, "searchGraph", mergeQuestion)
			requireText(t, raw, err, damaged, damaged > 0, textTruncated)
		})
	}
}
