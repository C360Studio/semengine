//go:build integration

package graphembedding

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/pkg/errs"
)

// outageEmbedder is a product's own embedder whose provider the test takes down and brings back.
// While down it fails as a product's embedder reports an outage: a text naming "busy" with
// errs.ErrRateLimited, any other with an errs.WrapTransient error. A text naming "wide" always
// fails with a reply of the wrong width, which no retry can fix. While hold is set, each call
// reports its text on entered and waits until hold is closed, as a slow provider would. calls
// counts the calls per text.
type outageEmbedder struct {
	entered chan string

	mu    sync.Mutex
	down  bool
	hold  chan struct{}
	calls map[string]int
}

func newOutageEmbedder() *outageEmbedder {
	return &outageEmbedder{entered: make(chan string, 16), down: true, calls: map[string]int{}}
}

// set takes the provider down or brings it back, and holds each call on hold when it is not nil.
func (e *outageEmbedder) set(down bool, hold chan struct{}) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.down, e.hold = down, hold
}

func (e *outageEmbedder) callsFor(text string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls[text]
}

func (e *outageEmbedder) Generate(ctx context.Context, texts []string) ([][]float32, error) {
	e.mu.Lock()
	for _, text := range texts {
		e.calls[text]++
	}
	down, hold := e.down, e.hold
	e.mu.Unlock()
	if hold != nil {
		e.entered <- texts[0]
		select {
		case <-hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		switch {
		case strings.Contains(text, "wide"):
			return nil, fmt.Errorf("embedding dimension 3 does not match %d", wordDimensions)
		case down && strings.Contains(text, "busy"):
			return nil, fmt.Errorf("provider asked for fewer requests: %w", errs.ErrRateLimited)
		case down:
			return nil, errs.WrapTransient(errors.New("provider answered 503"), "outageEmbedder", "Generate", "embed")
		}
		vectors[i] = wordVector(text)
	}
	return vectors, nil
}

func (e *outageEmbedder) GenerateQuery(ctx context.Context, texts []string) ([][]float32, error) {
	return e.Generate(ctx, texts)
}

func (*outageEmbedder) Dimensions() int { return wordDimensions }
func (*outageEmbedder) Model() string   { return "outage-v1" }
func (*outageEmbedder) Close() error    { return nil }

// outageFactory is graph-embedding as a product registers it with e as its embedder.
func outageFactory(t *testing.T, e *outageEmbedder) component.Factory {
	t.Helper()
	return registeredFactory(t, func(r *component.Registry) error {
		return RegisterWithEmbedder(r, "outage", func() (embedding.Embedder, error) { return e, nil })
	})
}

// recordRevision is the EMBEDDING_INDEX revision of id's record, read through a reader of its
// own: every pending write hop 1 makes for id moves it.
func (h *harness) recordRevision(t *testing.T, id string) uint64 {
	t.Helper()
	reader, err := kvcatalog.OpenCatalogReader(t.Context(), h.client, graph.BucketEmbeddingIndex)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := reader.Get(t.Context(), id)
	if err != nil {
		t.Fatalf("record of %s: %v", id, err)
	}
	return entry.Revision()
}

// TestRepairRetriesProviderFailures: entities that fail while the provider is down, as
// provider_unavailable or rate_limited, get their vectors from repair passes once it answers again,
// with no restart and no new write to ENTITY_STATES. An entity whose reply had the wrong width is
// not retried: it stays failed and no pass writes a pending record for it.
//
// The repair lane also runs every 30 seconds on its own, so every check here holds whichever pass,
// the test's or the lane's, made a retry.
func TestRepairRetriesProviderFailures(t *testing.T) {
	e := newOutageEmbedder()
	h := startEmbeddingBy(t, fixtureClient(t), `{}`, nil, outageFactory(t, e))
	texts := []string{"down one", "down two", "down three", "busy four", "wide five"}
	revs := map[string]uint64{}
	for i, text := range texts {
		id := entityID(i + 1)
		revs[id] = h.put(t, id, entityValue(t, id, nil, "doc.meta.title", text))
	}
	h.awaitFailed(t, "while the provider is down", map[string]uint64{
		"provider_unavailable": 3, "rate_limited": 1, "dimension_mismatch": 1})
	wide := entityID(5)
	wideRecord := h.recordRevision(t, wide)

	e.set(false, nil)
	for {
		n, _ := h.c.failed.counts()
		if n == 1 {
			break
		}
		h.c.repair(t.Context())
		// The pass re-drove a failure that was not waiting, or one is already waiting for the
		// Worker; either way the Worker's outcome, now a stored vector, lowers the count.
		await(t, "a retry's outcome", func(context.Context) (int64, error) {
			got, _ := h.c.failed.counts()
			return got, nil
		}, func(got int64) bool { return got < n })
	}
	h.assertFailed(t, "after the retries", map[string]uint64{"dimension_mismatch": 1})
	for i := range 4 {
		id := entityID(i + 1)
		h.awaitGenerated(t, id, revs[id])
	}
	// Each pass returns after any pending write it makes, so this read follows every write the
	// test's passes made.
	if got := h.recordRevision(t, wide); got != wideRecord {
		t.Errorf("the wide entity's record moved from revision %d to %d; want it never re-driven", wideRecord, got)
	}
	if got := e.callsFor("wide five"); got != 1 {
		t.Errorf("the embedder was called %d times for the wide entity; want once", got)
	}
}

// TestRepairWaitsForRedrivenOutcome: once a pass re-drives a failed entity, later passes leave it
// alone until the Worker reports the outcome: they write no second pending record, so the Worker
// makes no second call for it, and the entity stays counted failed meanwhile. The provider holds
// the re-driven call until the test lets it go.
func TestRepairWaitsForRedrivenOutcome(t *testing.T) {
	e := newOutageEmbedder()
	h := startEmbeddingBy(t, fixtureClient(t), `{}`, nil, outageFactory(t, e))
	a := entityID(1)
	rev := h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "down one"))
	h.awaitFailed(t, "while the provider is down", map[string]uint64{"provider_unavailable": 1})

	hold := make(chan struct{})
	e.set(false, hold)
	// This pass re-drives the entity, or the lane's own pass already has; either way the Worker's
	// call arrives and is held.
	h.c.repair(t.Context())
	if text := <-e.entered; text != "down one" {
		t.Fatalf("the held call embeds %q; want down one", text)
	}
	// The re-drive's pending write came before the Worker's call, and the Worker writes nothing
	// while the call is held.
	queued := h.recordRevision(t, a)
	for range 3 {
		h.c.repair(t.Context())
	}
	// Each pass returns after any pending write it makes, so this read follows them.
	if got := h.recordRevision(t, a); got != queued {
		t.Fatalf("passes while the re-drive waits moved the record from revision %d to %d; want no second re-drive",
			queued, got)
	}
	h.assertFailed(t, "while the re-drive waits", map[string]uint64{"provider_unavailable": 1})

	close(hold)
	h.awaitGenerated(t, a, rev)
	// The Worker's callback records the outcome after the generated record is written.
	h.awaitFailed(t, "after the re-drive's vector", nil)
}
