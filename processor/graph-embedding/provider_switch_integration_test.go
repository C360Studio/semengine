//go:build integration

package graphembedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/embedding/httpembedder"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/errs"
)

// switchEmbedder is a deterministic provider that maps every text to one vector. When lazy, it
// reports width 0 until its first Generate, as a remote embedder that learns its width from its
// first response does.
type switchEmbedder struct {
	model   string
	vector  []float32
	lazy    bool
	learned atomic.Bool
}

func (e *switchEmbedder) Generate(_ context.Context, texts []string) ([][]float32, error) {
	e.learned.Store(true)
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = slices.Clone(e.vector)
	}
	return out, nil
}

func (e *switchEmbedder) GenerateQuery(ctx context.Context, texts []string) ([][]float32, error) {
	return e.Generate(ctx, texts)
}

func (e *switchEmbedder) Model() string { return e.model }

func (e *switchEmbedder) Dimensions() int {
	if e.lazy && !e.learned.Load() {
		return 0
	}
	return len(e.vector)
}

func (*switchEmbedder) Close() error { return nil }

// provider is one registration: the type it is registered under and how to build its embedder.
type provider struct {
	embedderType string
	model        string
	vector       []float32
	lazy         bool
}

func (p provider) embedder() *switchEmbedder {
	return &switchEmbedder{model: p.model, vector: p.vector, lazy: p.lazy}
}

// TestProviderSwitchRestart: a product restarts against the same NATS state under another
// provider. A provider that differs from the first in registered type, model, or width (the
// last learned only from its first response) regenerates every entity in ENTITY_STATES before
// readiness, and search finds an entity by its own text. A vector the old provider left for an
// entity nothing regenerates (one with no ENTITY_STATES entry, standing for an entity whose
// regeneration has not run yet) is neither scored by search nor used or returned by similar. The
// same provider again keeps the stored vector.
func TestProviderSwitchRestart(t *testing.T) {
	a := provider{embedderType: "model-A", model: "model-A", vector: []float32{1, 0}}
	for _, tc := range []struct {
		name          string
		first, second provider
	}{
		{"same-provider", a, a},
		{"changed-model", a, provider{embedderType: "model-B", model: "model-B", vector: []float32{0, 1}}},
		{"changed-type", provider{embedderType: "provider-A", model: "same-model", vector: []float32{1, 0}},
			provider{embedderType: "provider-B", model: "same-model", vector: []float32{0, 1}}},
		{"unknown-width", provider{embedderType: "provider", model: "same-model", vector: []float32{1, 0}},
			provider{embedderType: "provider", model: "same-model", vector: []float32{0, 1, 0}, lazy: true}},
	} {
		t.Run(tc.name, func(t *testing.T) { providerRestart(t, tc.first, tc.second) })
	}
}

func providerRestart(t *testing.T, before, after provider) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	start := func(p provider) *harness {
		factory := registeredFactory(t, func(r *component.Registry) error {
			return RegisterWithEmbedder(r, p.embedderType, func() (embedding.Embedder, error) { return p.embedder(), nil })
		})
		built, err := factory(json.RawMessage(`{}`), component.Dependencies{NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry()})
		if err != nil {
			t.Fatal(err)
		}
		c := built.(*Component)
		if err = c.Initialize(); err != nil {
			t.Fatal(err)
		}
		if err = c.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { stopComponent(t, c) })
		return &harness{c: c, client: client, states: states, f: c.follower.Load()}
	}

	first := start(before)
	id := entityID(1)
	rev := first.put(t, id, entityValue(t, id, nil, "doc.meta.title", "widget"))
	stored := first.awaitGenerated(t, id, rev)
	stopComponent(t, first.c)

	// stale is a vector the first provider left for an entity with no ENTITY_STATES entry, so
	// nothing regenerates it: the old half of a mixed old/new state. Its vector is the one the
	// second provider produces, so a search or similar that scored it regardless of identity
	// would return it.
	stale := entityID(2)
	index, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEmbeddingIndex)
	if err != nil {
		t.Fatal(err)
	}
	staleRecord, err := json.Marshal(embedding.Record{
		EntityID: stale, Vector: slices.Clone(after.vector), EmbedderType: stored.EmbedderType,
		Model: stored.Model, Dimensions: stored.Dimensions, MaxTextLen: stored.MaxTextLen,
		Status: embedding.StatusGenerated, SourceRevision: 1, ContentHash: "stale",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := index.Put(t.Context(), stale, staleRecord); err != nil {
		t.Fatal(err)
	}

	second := start(after)
	// Readiness orders the read after the Worker's terminal write for the replayed revision.
	second.awaitIndexed(t, "replayed after the restart", rev)
	got, err := second.record(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != embedding.StatusGenerated || got.EmbedderType != after.embedderType ||
		got.Model != after.model || got.Dimensions != len(after.vector) || !slices.Equal(got.Vector, after.vector) {
		t.Fatalf("record after the restart = %+v; want generated by %s/%s with vector %v",
			got, after.embedderType, after.model, after.vector)
	}
	same := before.embedderType == after.embedderType && before.model == after.model && slices.Equal(before.vector, after.vector)
	if same && !got.GeneratedAt.Equal(stored.GeneratedAt) {
		t.Fatalf("same-provider restart regenerated the record: generated at %v, then %v", stored.GeneratedAt, got.GeneratedAt)
	}

	hits := func(r graph.QueryResponse[graph.SemanticData]) []string {
		var ids []string
		for _, hit := range r.Data.Results {
			ids = append(ids, hit.EntityID)
		}
		return ids
	}
	// The cache applies the regenerated record after readiness, so the wait ends on the first
	// reply that holds the entity, or on one that holds the stale vector, which must never show.
	// Under the same provider the stale record is that provider's own vector, the same one the
	// entity holds, so it is a legitimate hit.
	reply := awaitAnswer(t, second, "search", `{"query":"widget"}`, func(r graph.QueryResponse[graph.SemanticData]) bool {
		return slices.Contains(hits(r), id) || slices.Contains(hits(r), stale)
	})
	want := []string{id}
	if same {
		want = []string{id, stale}
	}
	if ids := hits(reply.reply); !slices.Equal(ids, want) || reply.reply.Data.EmbedderType != after.embedderType {
		t.Fatalf("search after the restart: type %s hits %v; want type %s hits %v",
			reply.reply.Data.EmbedderType, ids, after.embedderType, want)
	}
	if same {
		return
	}

	raw, err := second.ask(t, "similar", `{"entity_id":"`+stale+`"}`)
	requireRefused(t, "similar from the stale vector", raw, err, errs.ErrorInvalid, graph.ErrorCodeEmbeddingUnavailable)
	similar := awaitAnswer(t, second, "similar", `{"entity_id":"`+id+`"}`, func(graph.QueryResponse[graph.SimilarData]) bool { return true })
	if len(similar.reply.Data.Results) != 0 {
		t.Fatalf("similar to %s = %+v; want no hits (the only other vector is the old provider's)", id, similar.reply.Data.Results)
	}
}

// TestDeclaredWidthRestartKeepsVectors: graph-embedding on the HTTP embedder with its width set in
// the embedder's config restarts against the same NATS state and sends the provider no request for
// an entity whose stored vector is unchanged. Each start builds a new embedder, so the restarted one
// has had no reply to learn a width from: only the declared width lets it vouch for the stored
// vector before the provider answers.
func TestDeclaredWidthRestartKeepsVectors(t *testing.T) {
	client := fixtureClient(t)
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if r.URL.Path != "/v1/embeddings" || json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		data := make([]map[string]any, len(req.Input))
		for i := range req.Input {
			data[i] = map[string]any{"object": "embedding", "embedding": []float32{1, 0}, "index": i}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "model": req.Model, "data": data})
	}))
	t.Cleanup(srv.Close)
	factory := registeredFactory(t, func(r *component.Registry) error {
		return RegisterWithEmbedder(r, "semembed", func() (embedding.Embedder, error) {
			return httpembedder.New(httpembedder.Config{BaseURL: srv.URL + "/v1", Model: "m", Dimensions: 2})
		})
	})

	first := startEmbeddingBy(t, client, `{}`, nil, factory)
	id := entityID(1)
	rev := first.put(t, id, entityValue(t, id, nil, "doc.meta.title", "widget"))
	stored := first.awaitGenerated(t, id, rev)
	stopComponent(t, first.c)
	before := requests.Load()

	second := startEmbeddingBy(t, client, `{}`, nil, factory)
	// Readiness orders the reads after the Worker's terminal write for the replayed revision, which
	// comes after any request it would have sent for the entity.
	second.awaitIndexed(t, "replayed after the restart", rev)
	if n := requests.Load() - before; n != 0 {
		t.Fatalf("provider saw %d requests after the restart, want none: the stored vector is unchanged", n)
	}
	got, err := second.record(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != embedding.StatusGenerated || !got.GeneratedAt.Equal(stored.GeneratedAt) {
		t.Fatalf("record after the restart = %+v; want the stored one, generated at %v", got, stored.GeneratedAt)
	}
}
