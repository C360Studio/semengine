//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/embedding/httpembedder"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/vocabulary"
)

// fakeProviderModel is the model the fake provider serves and the client asks for.
const fakeProviderModel = "fake-embed-v1"

// newFakeProvider is an embedding service shaped like semembed's OpenAI-style API: POST
// /v1/embeddings answers one four-wide vector per input, naming the model it was asked for. The
// test closes it; the cleanup's second Close does nothing.
func newFakeProvider(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Input) == 0 {
			http.Error(w, `{"error":{"message":"bad request","type":"invalid_request_error"}}`, http.StatusBadRequest)
			return
		}
		type object struct {
			Object    string    `json:"object"`
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		reply := struct {
			Object string   `json:"object"`
			Data   []object `json:"data"`
			Model  string   `json:"model"`
		}{Object: "list", Model: request.Model}
		for i := range request.Input {
			reply.Data = append(reply.Data, object{Object: "embedding", Embedding: []float32{1, 0, 0, 0}, Index: i})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// embeddingStatus reads graph-embedding's GRAPH_STATUS entry as a remote reader does.
func embeddingStatus(ctx context.Context, client *natsclient.Client) (graph.IndexStatusResponse, error) {
	reader, err := kvcatalog.OpenCatalogReader(ctx, client, graph.BucketGraphStatus)
	if err != nil {
		return graph.IndexStatusResponse{}, err
	}
	entry, err := reader.Get(ctx, readiness.KeyGraphEmbedding)
	if err != nil {
		return graph.IndexStatusResponse{}, err
	}
	var status graph.IndexStatusResponse
	return status, json.Unmarshal(entry.Value(), &status)
}

// TestLosingTheProviderKeepsWordSearch: graph-ingest, graph-index, graph-lexical, graph-embedding
// with the HTTP client against a fake provider, and graph-query at tier 1. Entity A is written and
// both indexes are seen to hold it; the provider is closed and entity B written. Then the word
// searches find B, the semantic verb refuses rather than answering empty, searchGraph answers B by
// its words with no fallback and names its failed fallback for a question nothing matches, and
// graph-embedding's readiness reads degraded with B's provider failure counted.
func TestLosingTheProviderKeepsWordSearch(t *testing.T) {
	stack := startIngestAndIndex(t)
	startWordIndex(t, stack.client)
	provider := newFakeProvider(t)
	startGraphEmbeddingWith(t, stack.client, "fake-provider", func() (embedding.Embedder, error) {
		return httpembedder.New(httpembedder.Config{BaseURL: provider.URL + "/v1", Model: fakeProviderModel})
	})
	startComponent(t, stack.client, tierOneConfig)
	ask := func(verb, request string) ([]byte, error) {
		return stack.client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphQuery, verb).Subject, []byte(request), requestTimeout)
	}

	a := stack.create(t, drone1, [2]string{vocabulary.DCTermsTitle, "Alpha widget"})
	stack.awaitIndexed(t, a)
	awaitWordIndexed(t, stack.client, a)
	// The embedding index holds A once a semantic search answers it; until then the search refuses
	// index_not_ready or answers without it, and the answer naming A is the event.
	held, err := probe.Await(t.Context(), func(context.Context) (searchOutcome, error) {
		raw, err := ask("semantic", `{"query":"alpha widget"}`)
		return searchOutcome{raw, err}, nil
	}, func(o searchOutcome) bool {
		if o.err != nil {
			return false
		}
		var reply graph.QueryResponse[graph.SemanticData]
		return json.Unmarshal(o.raw, &reply) == nil &&
			slices.ContainsFunc(reply.Data.Results, func(hit graph.SimilarityHit) bool { return hit.EntityID == drone1 })
	})
	if err != nil || held.err != nil {
		t.Fatalf("await A in the embedding index: %v, %v", err, held.err)
	}

	provider.Close()
	b := stack.create(t, drone2, [2]string{vocabulary.DCTermsTitle, "Bravo gadget"})
	stack.awaitIndexed(t, b)
	awaitWordIndexed(t, stack.client, b)
	// graph-embedding marks B failed and publishes degraded with its reason; the published entry
	// is the event. Before graph-embedding first publishes, the key is absent.
	status, err := probe.Await(t.Context(), func(ctx context.Context) (graph.IndexStatusResponse, error) {
		status, err := embeddingStatus(ctx, stack.client)
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return status, nil
		}
		return status, err
	}, func(s graph.IndexStatusResponse) bool { return s.State == graph.IndexStateDegraded })
	if err != nil {
		t.Fatalf("await graph-embedding degraded: %v", err)
	}
	if want := map[string]uint64{"connection_refused": 1}; !maps.Equal(status.FailedReasons, want) {
		t.Errorf("failed_reasons = %v, want %v", status.FailedReasons, want)
	}

	raw, err := ask("globalSearch", `{"query":"bravo gadget"}`)
	if err != nil {
		t.Fatalf("globalSearch for B: %v", err)
	}
	if _, data := decodeGlobal(t, raw); !slices.Equal(matchIDs(data.Text.Matches), []string{drone2}) {
		t.Errorf("globalSearch for B matched %v, want %s", matchIDs(data.Text.Matches), drone2)
	}

	if raw, err := ask("semantic", `{"query":"bravo gadget"}`); err == nil {
		t.Errorf("semantic without its provider answered %s, want a refusal", raw)
	} else {
		refusalOf(t, err)
	}

	raw, err = ask("searchGraph", `{"query":"bravo gadget"}`)
	if err != nil {
		t.Fatalf("searchGraph for B: %v", err)
	}
	envelope, reply := decodeFallback(t, raw)
	if reply.Text == nil || !slices.Equal(matchIDs(reply.Text.Matches), []string{drone2}) {
		t.Errorf("searchGraph for B = %s, want the match %s", envelope.Data, drone2)
	}
	if _, ok := reply.keys["fallback"]; ok {
		t.Errorf("searchGraph for B = %s, want no fallback", envelope.Data)
	}

	raw, err = ask("searchGraph", `{"query":"zzz qqq"}`)
	if err != nil {
		t.Fatalf("searchGraph for nothing: %v; want the empty text with the failed fallback named", err)
	}
	envelope, reply = decodeFallback(t, raw)
	if reply.Text == nil || len(reply.Text.Matches) != 0 {
		t.Errorf("searchGraph for nothing = %s, want the empty text", envelope.Data)
	}
	if reply.Fallback == nil || reply.Fallback.Honoured || reply.Fallback.Code == "" {
		t.Errorf("searchGraph for nothing: fallback = %+v, want not honoured with a code", reply.Fallback)
	}
}
