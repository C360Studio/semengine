//go:build integration

package graphembedding

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/embedding/httpembedder"
)

// truncatedMetric is the name an operator scrapes for entities embedded from only the opening of
// their text.
const truncatedMetric = "semengine_graph_embedding_truncated_total"

// truncatedCount reads truncatedMetric's series for by from the metrics registry graph-embedding
// was given, as a scrape would; a missing family or series fails the test.
func truncatedCount(t *testing.T, h *harness, by string) float64 {
	t.Helper()
	families, err := h.metrics.PrometheusRegistry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() != truncatedMetric {
			continue
		}
		for _, m := range family.GetMetric() {
			for _, label := range m.GetLabel() {
				if label.GetName() == "by" && label.GetValue() == by {
					return m.GetCounter().GetValue()
				}
			}
		}
		t.Fatalf("%s has no series by=%q", truncatedMetric, by)
	}
	t.Fatalf("%s is not registered", truncatedMetric)
	return 0
}

// windowRunes is the fake provider's input window: like semembed, it embeds a longer text from its
// opening and marks that text's reply entry truncated.
const windowRunes = 20

// cuttingProvider is a semembed-shaped embeddings endpoint that marks each text longer than
// windowRunes truncated, and counts the texts it marked in marked. Every vector is {1, 0, 0, 0}:
// the test reads only the marks.
func cuttingProvider(t *testing.T) (srv *httptest.Server, marked *atomic.Int64) {
	t.Helper()
	marked = &atomic.Int64{}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if r.URL.Path != "/v1/embeddings" || json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		type entry struct {
			Object    string    `json:"object"`
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
			Truncated bool      `json:"truncated,omitempty"`
		}
		data := make([]entry, len(req.Input))
		for i, text := range req.Input {
			data[i] = entry{Object: "embedding", Embedding: []float32{1, 0, 0, 0}, Index: i,
				Truncated: len([]rune(text)) > windowRunes}
			if data[i].Truncated {
				marked.Add(1)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "model": req.Model, "data": data})
	}))
	t.Cleanup(srv.Close)
	return srv, marked
}

// semembedFactory is graph-embedding registered with an HTTP embedder on provider.
func semembedFactory(t *testing.T, provider *httptest.Server) component.Factory {
	t.Helper()
	return registeredFactory(t, func(registry *component.Registry) error {
		return RegisterWithEmbedder(registry, "semembed", func() (embedding.Embedder, error) {
			return httpembedder.New(httpembedder.Config{BaseURL: provider.URL + "/v1", Model: "m"})
		})
	})
}

// TestProviderCutCounted: graph-embedding on the HTTP embedder, against a provider that marks a
// text it cut, counts the long entity under by="provider" and nothing under by="text_cap": the
// text is under graph-embedding's own cap, and the short entity is not marked.
func TestProviderCutCounted(t *testing.T) {
	client := fixtureClient(t)
	provider, _ := cuttingProvider(t)
	h := startEmbeddingBy(t, client, `{}`, nil, semembedFactory(t, provider))

	long, short := entityID(1), entityID(2)
	longRev := h.put(t, long, entityValue(t, long, nil, "doc.meta.title", strings.Repeat("word ", 10)))
	shortRev := h.put(t, short, entityValue(t, short, nil, "doc.meta.title", "short title"))
	h.awaitGenerated(t, long, longRev)
	h.awaitGenerated(t, short, shortRev)
	// The Worker counts a cut after it stores the vector and before it reports the entity done,
	// which moves the follower's floor awaited here; shortRev is the later write, so the floor
	// past it covers both entities.
	h.awaitIndexed(t, "both entities done", shortRev)

	if n := truncatedCount(t, h, "provider"); n != 1 {
		t.Errorf(`truncated_total{by="provider"} = %v, want 1: one entity's text was marked`, n)
	}
	if n := truncatedCount(t, h, "text_cap"); n != 0 {
		t.Errorf(`truncated_total{by="text_cap"} = %v, want 0: both texts fit max_text_len`, n)
	}
}

// TestTextCapCutCounted: an entity longer than max_text_len is counted under by="text_cap", one
// within it is not, and the word-count embedder, which marks nothing, leaves by="provider" at 0.
func TestTextCapCutCounted(t *testing.T) {
	client := fixtureClient(t)
	h := startEmbedding(t, client, `{"max_text_len":20}`, nil)

	long, short := entityID(1), entityID(2)
	longRev := h.put(t, long, entityValue(t, long, nil, "doc.meta.title", strings.Repeat("word ", 10)))
	shortRev := h.put(t, short, entityValue(t, short, nil, "doc.meta.title", "short title"))
	h.awaitGenerated(t, long, longRev)
	h.awaitGenerated(t, short, shortRev)
	// The Worker counts a cut after it stores the vector and before it reports the entity done,
	// which moves the follower's floor awaited here; shortRev is the later write, so the floor
	// past it covers both entities.
	h.awaitIndexed(t, "both entities done", shortRev)

	if n := truncatedCount(t, h, "text_cap"); n != 1 {
		t.Errorf(`truncated_total{by="text_cap"} = %v, want 1: one entity's text is past max_text_len`, n)
	}
	if n := truncatedCount(t, h, "provider"); n != 0 {
		t.Errorf(`truncated_total{by="provider"} = %v, want 0: the embedder marks nothing`, n)
	}
}

// TestCapCutCountedOnceWhenProviderAlsoCuts: an entity whose text graph-embedding cut at
// max_text_len, and which the provider then marked truncated as well, is one entity embedded from
// its opening, counted once under by="text_cap" and not under by="provider", so the two series
// add up to the entities cut. The cap, 30, leaves the text longer than the provider's window, 20,
// and the provider's own count shows it did mark that text.
func TestCapCutCountedOnceWhenProviderAlsoCuts(t *testing.T) {
	client := fixtureClient(t)
	provider, marked := cuttingProvider(t)
	h := startEmbeddingBy(t, client, `{"max_text_len":30}`, nil, semembedFactory(t, provider))

	long, short := entityID(1), entityID(2)
	longRev := h.put(t, long, entityValue(t, long, nil, "doc.meta.title", strings.Repeat("word ", 10)))
	shortRev := h.put(t, short, entityValue(t, short, nil, "doc.meta.title", "short title"))
	h.awaitGenerated(t, long, longRev)
	h.awaitGenerated(t, short, shortRev)
	// The provider marks a text before it replies. The Worker counts a cut after it stores the
	// vector and before it reports the entity done, which moves the follower's floor awaited
	// here; shortRev is the later write, so the floor past it covers both entities.
	h.awaitIndexed(t, "both entities done", shortRev)

	if n := marked.Load(); n != 1 {
		t.Fatalf("the provider marked %d texts, want 1: the capped text must still be over its window", n)
	}
	if n := truncatedCount(t, h, "text_cap"); n != 1 {
		t.Errorf(`truncated_total{by="text_cap"} = %v, want 1: one entity's text is past max_text_len`, n)
	}
	if n := truncatedCount(t, h, "provider"); n != 0 {
		t.Errorf(`truncated_total{by="provider"} = %v, want 0: the provider's cut is of a text the cap already cut`, n)
	}
}
