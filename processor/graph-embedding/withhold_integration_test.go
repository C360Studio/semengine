//go:build integration

package graphembedding

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/storage"
	"github.com/c360studio/semengine/storage/storeregistry"
)

// brokenInstance is the storage instance of a store whose every read fails, so the Worker fails
// the entity content_error (text extraction failed), the Worker failure tier 0 can produce.
const brokenInstance = "broken"

type brokenStore struct{ storage.StreamableStore }

func (brokenStore) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("the disk is unreadable")
}

// bothStores is a registry holding the gated store under bodyInstance and a broken one under
// brokenInstance.
func bothStores(t *testing.T, gated *gatedStore) *storeregistry.Registry {
	t.Helper()
	stores := gated.stores(t)
	if err := stores.Register(brokenInstance, brokenStore{}); err != nil {
		t.Fatal(err)
	}
	return stores
}

func brokenRef(key string) map[string]string {
	return map[string]string{"storage_instance": brokenInstance, "key": key}
}

// startAfter ensures ENTITY_STATES, runs before with the write handle set, so what it writes is
// in the follower's first build, and then builds graph-embedding from raw on deps' logger and
// starts it; the test's cleanup stops it. A nil logger logs to the harness's buffer.
func startAfter(t *testing.T, client *natsclient.Client, stores *storeregistry.Registry, logger func(io.Writer) slog.Handler,
	before func(h *harness)) *harness {
	t.Helper()
	h := &harness{client: client, states: ensureEntityStates(t, client), logs: &logBuffer{}, metrics: metric.NewMetricsRegistry()}
	before(h)
	if logger == nil {
		logger = func(w io.Writer) slog.Handler {
			return slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
		}
	}
	built, err := CreateGraphEmbedding(json.RawMessage(`{}`), component.Dependencies{
		NATSClient: client, MetricsRegistry: h.metrics, StoreRegistry: stores, Logger: slog.New(logger(h.logs)),
	})
	if err != nil {
		t.Fatalf("CreateGraphEmbedding: %v", err)
	}
	h.c = built.(*Component)
	if err := h.c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := h.c.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopComponent(t, h.c) })
	h.f = h.c.follower.Load()
	return h
}

// refusedNow sends body on verb once and requires the refusal class and code.
func refusedNow(t *testing.T, h *harness, what, verb, body string, class errs.ErrorClass, code string) {
	t.Helper()
	raw, err := h.ask(t, verb, body)
	requireRefused(t, what+": "+verb, raw, err, class, code)
}

// awaitFailed waits until the failed map holds exactly want.
func (h *harness) awaitFailed(t *testing.T, what string, want map[string]uint64) {
	t.Helper()
	await(t, what, func(context.Context) (map[string]uint64, error) {
		_, reasons := h.c.failed.counts()
		return reasons, nil
	}, func(reasons map[string]uint64) bool { return maps.Equal(reasons, want) })
}

// TestQueriesWithheldUntilReady: both verbs ask the follower's QueryReady before they answer
// (graph-embedding spec, "The embedding verbs are withheld until they can answer").
// Each refusal is read while the state that causes it is held by the test, so it is not timing.
func TestQueriesWithheldUntilReady(t *testing.T) {
	t.Run("before the first build", func(t *testing.T) {
		store := newGatedStore()
		held := entityID(1)
		h := startAfter(t, fixtureClient(t), store.stores(t), nil, func(h *harness) {
			h.put(t, held, entityValue(t, held, bodyRef("held"), "doc.meta.title", "gadget"))
		})
		// The Worker holds the entity's revision inside its body read, so the floor stays below the
		// target the snapshot fixed: the first build is incomplete until the test releases it.
		if key := <-store.opened; key != "held" {
			t.Fatalf("the Worker opened %q; want held", key)
		}
		refusedNow(t, h, "before the first build", "search", `{"query":"gadget"}`, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
		refusedNow(t, h, "before the first build", "similar", `{"entity_id":"`+held+`"}`, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)

		store.release("held")
		got := awaitAnswer(t, h, "search", `{"query":"gadget"}`, hitCount[graph.SemanticData](1))
		if ids := ids(got.reply.Data.Results); !slices.Equal(ids, []string{held}) {
			t.Errorf("search once the build completed = %v; want [%s]", ids, held)
		}
	})

	t.Run("a hop-1 failure", func(t *testing.T) {
		client := fixtureClient(t)
		h := startEmbedding(t, client, `{}`, nil)
		a, b := entityID(1), entityID(2)
		h.titled(t, "gadget", a, b)
		awaitAnswer(t, h, "search", `{"query":"gadget"}`, hitCount[graph.SemanticData](2))

		editStream(t, client, graph.BucketEntityStates, directGets(false))
		// Hop 1 marks the read failure before it drains the revision, so the floor reaching it
		// orders the requests after the mark.
		h.awaitIndexed(t, "the unread revision drained", h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "gadget again")))
		h.assertFailed(t, "after the failed read", map[string]uint64{"entity_read_failed": 1})
		refusedNow(t, h, "a held entity_read_failed", "search", `{"query":"gadget"}`, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
		refusedNow(t, h, "a held entity_read_failed", "similar", `{"entity_id":"`+b+`"}`, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)

		editStream(t, client, graph.BucketEntityStates, directGets(true))
		h.c.repair(t.Context()) // the read answers: the failure clears on this goroutine
		h.assertFailed(t, "after the repair", nil)
		got := answerNow[graph.SemanticData](t, h, "search", `{"query":"gadget"}`)
		if got.reply.Data.FailedRecords != 0 {
			t.Errorf("search after the repair: failed_records %d; want 0", got.reply.Data.FailedRecords)
		}
	})

	t.Run("after a reset", func(t *testing.T) {
		h := startEmbedding(t, fixtureClient(t), `{}`, nil)
		a := entityID(1)
		h.titled(t, "gadget", a)
		awaitAnswer(t, h, "search", `{"query":"gadget"}`, hitCount[graph.SemanticData](1))
		h.put(t, entityID(3), []byte("not an entity state"))
		// The follower latches the reset before Health can name it.
		await(t, "the reset latched", func(context.Context) (component.HealthStatus, error) { return h.c.Health(), nil },
			func(health component.HealthStatus) bool {
				return strings.Contains(health.LastError, "graph state reset required")
			})
		refusedNow(t, h, "after a reset", "search", `{"query":"gadget"}`, errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired)
		refusedNow(t, h, "after a reset", "similar", `{"entity_id":"`+a+`"}`, errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired)
	})
}

// TestWorkerFailureAnswersAndCounts: a Worker failure does not withhold an answer
// (graph-embedding spec, "A document whose text cannot be extracted"): both verbs
// answer from the other vectors with failed_records 1, while Health and the published status keep
// reading the failure, degraded. A failure that arrives before the first build is refused with the
// build, and once the build completes the verbs answer past it with no other change.
func TestWorkerFailureAnswersAndCounts(t *testing.T) {
	t.Run("after the first build", func(t *testing.T) {
		client := fixtureClient(t)
		h := startEmbedding(t, client, `{}`, bothStores(t, newGatedStore()))
		good, bad := entityID(1), entityID(2)
		h.titled(t, "gadget", good)
		awaitAnswer(t, h, "search", `{"query":"gadget"}`, hitCount[graph.SemanticData](1))
		h.put(t, bad, entityValue(t, bad, brokenRef("bad"), "doc.meta.title", "gadget"))
		h.awaitFailed(t, "the Worker failed the unreadable body", map[string]uint64{"content_error": 1})

		// The cache holds good's vector (the answer above) and never one for bad, so each verb is
		// asked once: a refusal here fails the test rather than being asked again.
		search := answerNow[graph.SemanticData](t, h, "search", `{"query":"gadget"}`)
		if ids := ids(search.reply.Data.Results); !slices.Equal(ids, []string{good}) || search.reply.Data.FailedRecords != 1 {
			t.Errorf("search with a content_error = %s; want only %s with failed_records 1", search.raw, good)
		}
		similar := answerNow[graph.SimilarData](t, h, "similar", `{"entity_id":"`+good+`"}`)
		if len(similar.reply.Data.Results) != 0 || similar.reply.Data.FailedRecords != 1 {
			t.Errorf("similar with a content_error = %s; want no hit with failed_records 1", similar.raw)
		}
		if health := h.c.Health(); health.Healthy || !strings.Contains(health.LastError, "1 unresolved failures: content_error 1") {
			t.Errorf("Health with a content_error = %+v; want degraded naming the unresolved write", health)
		}
		published := await(t, "the published status counts the failure",
			func(ctx context.Context) (status, error) { return publishedStatus(ctx, client) },
			func(s status) bool { return s.envelope.FailedCount == 1 })
		if published.envelope.State != graph.IndexStateDegraded || published.envelope.Ready {
			t.Errorf("published status with a content_error = %+v; want degraded, not ready", published.envelope)
		}
	})

	t.Run("before the first build", func(t *testing.T) {
		store := newGatedStore()
		held, bad := entityID(1), entityID(2)
		h := startAfter(t, fixtureClient(t), bothStores(t, store), nil, func(h *harness) {
			h.put(t, held, entityValue(t, held, bodyRef("held"), "doc.meta.title", "gadget"))
			h.put(t, bad, entityValue(t, bad, brokenRef("bad"), "doc.meta.title", "gadget"))
		})
		if key := <-store.opened; key != "held" {
			t.Fatalf("the Worker opened %q; want held", key)
		}
		h.awaitFailed(t, "the Worker failed the unreadable body", map[string]uint64{"content_error": 1})
		refusedNow(t, h, "a content_error before the first build", "search", `{"query":"gadget"}`,
			errs.ErrorTransient, graph.ErrorCodeIndexNotReady)

		store.release("held")
		got := awaitAnswer(t, h, "search", `{"query":"gadget"}`, hitCount[graph.SemanticData](1))
		if ids := ids(got.reply.Data.Results); !slices.Equal(ids, []string{held}) || got.reply.Data.FailedRecords != 1 {
			t.Errorf("search once the build completed = %s; want only %s with failed_records 1", got.raw, held)
		}
	})
}

// TestHealthNamesLostCacheWatch: when EMBEDDING_INDEX is deleted under a running component, the
// vector cache's Run returns while the run context is live: Health names the loss until the
// process restarts and both verbs refuse index_not_ready (graph-embedding spec, "A lost cache
// watch is a stuck state"). Nothing rebinds the watch: with the bucket recreated and
// a record written to it, the loss is still named and the verbs still refuse. The loss latches
// whether or not the first build is complete.
func TestHealthNamesLostCacheWatch(t *testing.T) {
	loseIndex := func(t *testing.T, h *harness) {
		t.Helper()
		js, err := h.client.JetStream()
		if err != nil {
			t.Fatal(err)
		}
		if err := js.DeleteKeyValue(t.Context(), graph.BucketEmbeddingIndex); err != nil {
			t.Fatalf("delete %s: %v", graph.BucketEmbeddingIndex, err)
		}
		// How soon nats.go closes a deleted bucket's watch is its timing; go test -timeout is the
		// deadline. The component latches the loss before it logs the line, and the log buffer's
		// lock orders the reads below after that write.
		await(t, "the lost cache watch logged", func(context.Context) (string, error) { return h.logs.String(), nil },
			func(logs string) bool { return strings.Contains(logs, "vector cache watch lost") })
		if health := h.c.Health(); health.Healthy || health.Status != "degraded" ||
			!strings.Contains(health.LastError, vectorCacheWatchLost) {
			t.Fatalf("Health after the loss = %+v; want degraded, not healthy, naming %q", health, vectorCacheWatchLost)
		}
	}

	t.Run("after the first build", func(t *testing.T) {
		client := fixtureClient(t)
		h := startEmbedding(t, client, `{}`, nil)
		a, b := entityID(1), entityID(2)
		h.titled(t, "gadget", a)
		awaitAnswer(t, h, "search", `{"query":"gadget"}`, hitCount[graph.SemanticData](1))

		loseIndex(t, h)
		refusedNow(t, h, "a lost cache watch", "search", `{"query":"gadget"}`, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
		refusedNow(t, h, "a lost cache watch", "similar", `{"entity_id":"`+a+`"}`, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)

		if _, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEmbeddingIndex); err != nil {
			t.Fatalf("recreate %s: %v", graph.BucketEmbeddingIndex, err)
		}
		h.awaitPending(t, b, h.put(t, b, entityValue(t, b, nil, "doc.meta.title", "gadget")))
		if health := h.c.Health(); health.Healthy || !strings.Contains(health.LastError, vectorCacheWatchLost) {
			t.Errorf("Health after the bucket came back = %+v; want the lost cache watch still named", health)
		}
		refusedNow(t, h, "the bucket back", "search", `{"query":"gadget"}`, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
		refusedNow(t, h, "the bucket back", "similar", `{"entity_id":"`+b+`"}`, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
	})

	t.Run("before the first build", func(t *testing.T) {
		store := newGatedStore()
		held := entityID(1)
		h := startAfter(t, fixtureClient(t), store.stores(t), nil, func(h *harness) {
			h.put(t, held, entityValue(t, held, bodyRef("held"), "doc.meta.title", "gadget"))
		})
		if key := <-store.opened; key != "held" {
			t.Fatalf("the Worker opened %q; want held", key)
		}
		loseIndex(t, h)
		refusedNow(t, h, "a lost cache watch before the first build", "search", `{"query":"gadget"}`,
			errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
	})
}

// heldLine holds the first log record with message, once armed, inside the component's logger
// until release closes: a handler that logs it is held in flight. entered closes once it is held.
type heldLine struct {
	message string
	armed   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

// holdingHandler is the component's slog.Handler, through Dependencies.Logger. It holds only the
// armed line and holds no lock while it does, so the component's other goroutines keep logging.
type holdingHandler struct {
	inner slog.Handler
	held  *heldLine
}

func (h holdingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h holdingHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == h.held.message && h.held.armed.CompareAndSwap(true, false) {
		close(h.held.entered)
		<-h.held.release
	}
	return h.inner.Handle(ctx, r)
}

func (h holdingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return holdingHandler{inner: h.inner.WithAttrs(attrs), held: h.held}
}

func (h holdingHandler) WithGroup(name string) slog.Handler {
	return holdingHandler{inner: h.inner.WithGroup(name), held: h.held}
}

// noResponders requires that nothing answers either verb's subject.
func noResponders(t *testing.T, client *natsclient.Client, what string) {
	t.Helper()
	for _, verb := range []string{"search", "similar"} {
		raw, err := client.RequestClassified(t.Context(), subjectOf(t, verb), []byte(`{"query":"gadget"}`), requestBound)
		if !natsclient.IsNoResponders(err) {
			t.Errorf("%s: %s = %s, %v; want no responders", what, verb, raw, err)
		}
	}
}

// TestNoResponderAfterStop: before Start and after Stop returns nothing answers either verb's
// subject, and a request in flight when Stop begins is answered: Stop drains the
// query subscriptions before it ends the run context, and returns only after the handler has.
func TestNoResponderAfterStop(t *testing.T) {
	t.Run("idle", func(t *testing.T) {
		client := fixtureClient(t)
		h := startAfter(t, client, nil, nil, func(*harness) { noResponders(t, client, "before Start") })
		awaitAnswer(t, h, "search", `{"query":"gadget"}`, hitCount[graph.SemanticData](0))
		stopComponent(t, h.c)
		noResponders(t, client, "after Stop")
	})

	// The request is held inside the follower's QueryReady, at its "query deferred" debug line,
	// which it logs while the first build is incomplete (the Worker holds the one entity's body
	// read). It is a similar request, the verb drained second, so search answering no responders
	// says Stop is draining similar, where it waits for the held handler.
	t.Run("a request in flight", func(t *testing.T) {
		client := fixtureClient(t)
		store := newGatedStore()
		held := entityID(1)
		similar := `{"entity_id":"` + held + `"}`
		hold := &heldLine{message: "query deferred", entered: make(chan struct{}), release: make(chan struct{})}
		h := startAfter(t, client, store.stores(t), func(w io.Writer) slog.Handler {
			return holdingHandler{inner: slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}), held: hold}
		}, func(h *harness) {
			h.put(t, held, entityValue(t, held, bodyRef("held"), "doc.meta.title", "gadget"))
		})
		releaseOnce := sync.OnceFunc(func() { close(hold.release) })
		t.Cleanup(releaseOnce) // registered after Start's, so it runs before the Stop that waits on the handler
		if key := <-store.opened; key != "held" {
			t.Fatalf("the Worker opened %q; want held", key)
		}

		type outcome struct {
			raw []byte
			err error
		}
		replied := make(chan outcome, 1)
		hold.armed.Store(true)
		go func() {
			raw, err := client.RequestClassified(context.WithoutCancel(t.Context()), subjectOf(t, "similar"), []byte(similar), requestBound)
			replied <- outcome{raw, err}
		}()
		<-hold.entered

		stopped := make(chan error, 1)
		go func() { stopped <- h.c.Stop(t.Context()) }()
		// search is drained first: once it has no responders, Stop is waiting on similar's drain.
		if _, err := probe.Await(t.Context(), func(ctx context.Context) (error, error) {
			_, err := client.RequestClassified(ctx, subjectOf(t, "search"), []byte(`{"query":"gadget"}`), requestBound)
			return err, nil
		}, natsclient.IsNoResponders); err != nil {
			t.Fatalf("search still served during Stop: %v", err)
		}
		select {
		case err := <-stopped:
			t.Fatalf("Stop returned %v with a similar handler in flight", err)
		default:
		}
		select {
		case <-store.cancelled:
			t.Fatal("the run context ended with a query handler in flight; the drain comes first")
		default:
		}

		releaseOnce()
		got := <-replied
		requireRefused(t, "the request in flight during Stop", got.raw, got.err, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
		if err := <-stopped; err != nil {
			t.Fatalf("Stop: %v", err)
		}
		noResponders(t, client, "after Stop")
		for _, line := range []string{"vector cache watch lost", "embedding worker watch lost"} {
			if strings.Contains(h.logs.String(), line) {
				t.Errorf("a Stop logged %q:\n%s", line, h.logs.String())
			}
		}
	})
}
