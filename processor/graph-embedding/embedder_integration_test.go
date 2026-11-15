//go:build integration

package graphembedding

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/natsclient"
)

// errCloseFailed is a test embedder's first Close failure.
var errCloseFailed = errors.New("embedder close failed")

// dropEntityStates deletes ENTITY_STATES, so a Start waiting one attempt for it fails in the
// follower's Open, after the embedder is built.
func dropEntityStates(t *testing.T, client *natsclient.Client) {
	t.Helper()
	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	if err := js.DeleteKeyValue(t.Context(), graph.BucketEntityStates); err != nil &&
		!errors.Is(err, jetstream.ErrBucketNotFound) {
		t.Fatalf("delete %s: %v", graph.BucketEntityStates, err)
	}
}

// requireBuiltOnce returns the one embedder source built, failing the test on any other count.
func requireBuiltOnce(t *testing.T, source *embedderSource) *wordEmbedder {
	t.Helper()
	built := source.builds()
	if len(built) != 1 {
		t.Fatalf("built %d embedders, want one", len(built))
	}
	return built[0]
}

// TestEmbedderClosedOnStop: the embedder lives from Start to Stop (graph-embedding spec, "The
// embedder's lifetime"). A component built and never started builds none; one started and
// stopped builds one and closes it once, a second Stop adding no close; a Start that fails after
// the build closes it; and when that close fails, Start reports both errors and keeps the embedder,
// which the next Stop closes.
func TestEmbedderClosedOnStop(t *testing.T) {
	const failFast = `{"startup_attempts": 1, "startup_interval_ms": 1}`

	t.Run("built and never started", func(t *testing.T) {
		source := &embedderSource{}
		h := newEmbeddingFrom(t, fixtureClient(t), `{}`, nil, source)
		stopComponent(t, h.c)
		if built := source.builds(); len(built) != 0 {
			t.Fatalf("a component never started built %d embedders, want none", len(built))
		}
	})

	t.Run("started and stopped", func(t *testing.T) {
		client := fixtureClient(t)
		ensureEntityStates(t, client)
		source := &embedderSource{}
		h := newEmbeddingFrom(t, client, `{}`, nil, source)
		if err := h.c.Start(t.Context()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		e := requireBuiltOnce(t, source)
		if n := e.closeCount(); n != 0 {
			t.Fatalf("a running component's embedder closed %d times, want none", n)
		}
		stopComponent(t, h.c)
		stopComponent(t, h.c)
		if n := e.closeCount(); n != 1 {
			t.Fatalf("embedder closed %d times over two Stops, want once", n)
		}
	})

	t.Run("Start fails after the build", func(t *testing.T) {
		client := fixtureClient(t)
		dropEntityStates(t, client)
		source := &embedderSource{}
		h := newEmbeddingFrom(t, client, failFast, nil, source)
		if err := h.c.Start(t.Context()); err == nil {
			t.Fatal("Start without ENTITY_STATES returned nil")
		}
		if n := requireBuiltOnce(t, source).closeCount(); n != 1 {
			t.Fatalf("a failed Start closed its embedder %d times, want once", n)
		}
		if h.c.embedder != nil {
			t.Fatal("a failed Start whose close succeeded kept the embedder")
		}
	})

	t.Run("its close fails too", func(t *testing.T) {
		client := fixtureClient(t)
		dropEntityStates(t, client)
		source := &embedderSource{closeErr: errCloseFailed}
		h := newEmbeddingFrom(t, client, failFast, nil, source)
		err := h.c.Start(t.Context())
		if !errors.Is(err, errCloseFailed) || !strings.Contains(err.Error(), "follower") {
			t.Fatalf("Start = %v, want both the follower's failure and the close failure", err)
		}
		e := requireBuiltOnce(t, source)
		if h.c.embedder != e {
			t.Fatal("a failed Start whose close failed did not keep the embedder for Stop")
		}
		stopComponent(t, h.c)
		if n := e.closeCount(); n != 2 || h.c.embedder != nil {
			t.Fatalf("after Stop the embedder closed %d times and is kept %v, want closed again and released",
				n, h.c.embedder != nil)
		}
	})
}

// TestProductEmbedderIsUsed: a product's embedder does the embedding (graph-embedding spec, "A
// product's embedder"). The entity's one title is its whole text, so its generated record holds the
// test embedder's vector for that title, under the dedup key whose type part is the registered
// name, and the dedup bucket holds that vector under the key; search embeds its query through the
// embedder's query side and replies that name as embedder_type.
func TestProductEmbedderIsUsed(t *testing.T) {
	h := startEmbedding(t, fixtureClient(t), `{}`, nil)
	e := requireBuiltOnce(t, h.source)
	const text = "Blue Widget"
	id := entityID(1)
	record := h.awaitGenerated(t, id, h.put(t, id, entityValue(t, id, nil, "doc.meta.title", text)))

	if !slices.Equal(record.Vector, wordVector(text)) {
		t.Fatalf("record vector for %q is not the product embedder's", text)
	}
	key := embedding.DedupKey(embedding.EmbedderIdentity{
		Type: testEmbedderType, Model: e.Model(), Dimensions: e.Dimensions(), MaxTextLen: h.c.config.MaxTextLen,
	}, text)
	if record.ContentHash != key {
		t.Fatalf("record dedup key %q, want the %s key %q", record.ContentHash, testEmbedderType, key)
	}
	dedup, err := h.c.storage.GetByContentHash(t.Context(), key)
	if err != nil || dedup == nil || !slices.Equal(dedup.Vector, record.Vector) {
		t.Fatalf("dedup record under the %s key = %+v, %v; want the entity's vector", testEmbedderType, dedup, err)
	}

	search := awaitAnswer(t, h, "search", `{"query":"widget"}`, hitCount[graph.SemanticData](1))
	if got := search.reply.Data.EmbedderType; got != testEmbedderType {
		t.Errorf("search embedder_type = %q, want %q", got, testEmbedderType)
	}
	if !slices.Contains(e.queryTexts(), "widget") {
		t.Errorf("query texts embedded = %q, want the search's query among them", e.queryTexts())
	}
}
