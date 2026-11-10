//go:build integration

package graphclustering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
)

// shared is this package's NATS server. It starts at the first reset; TestMain removes it after the tests.
var shared natsfixture.Shared

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	code := m.Run()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := shared.Stop(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

func fixtureClient(t *testing.T) *natsclient.Client {
	t.Helper()
	return natsfixture.Open(t, shared.Reset(t), openClient)
}

func openClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(15*time.Second),
		natsclient.WithMaxReconnects(0),
		natsclient.WithHealthInterval(0),
	)
	if err != nil {
		return nil, nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	if err := client.WaitForConnection(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	return client, client.Close, nil
}

func ensureEntityStates(t *testing.T, client *natsclient.Client) jetstream.KeyValue {
	t.Helper()
	states, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEntityStates)
	if err != nil {
		t.Fatalf("ensure %s: %v", graph.BucketEntityStates, err)
	}
	return states
}

// newClustering builds an initialized, unstarted graph-clustering from raw through
// CreateGraphClustering, on a metrics registry of its own.
func newClustering(t *testing.T, client *natsclient.Client, raw string) *Component {
	t.Helper()
	built, err := CreateGraphClustering(json.RawMessage(raw), component.Dependencies{
		NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry(),
		Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("CreateGraphClustering: %v", err)
	}
	c := built.(*Component)
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

// startClustering ensures ENTITY_STATES, builds graph-clustering with the default configuration
// and starts it; the test's cleanup stops it.
func startClustering(t *testing.T, client *natsclient.Client) (*Component, jetstream.KeyValue) {
	t.Helper()
	states := ensureEntityStates(t, client)
	c := newClustering(t, client, `{}`)
	if err := c.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopComponent(t, c) })
	return c, states
}

// stopBound bounds a cleanup Stop: long enough for any healthy Stop, so the bound never decides
// a test (as graph-embedding's).
const stopBound = time.Hour

func stopComponent(t *testing.T, c *Component) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stopBound)
	defer cancel()
	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

// await polls observe until done accepts its value; go test -timeout is the one deadline.
func await[T any](t *testing.T, what string, observe func(context.Context) (T, error), done func(T) bool) T {
	t.Helper()
	v, err := probe.Await(t.Context(), observe, done)
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	return v
}

// storedEntity is a canonical ENTITY_STATES value for id.
func storedEntity(id string) []byte {
	return []byte(`{"id":"` + id + `","triples":[{"subject":"` + id +
		`","predicate":"test.state.value","object":1,"source":"test","timestamp":"2026-11-10T12:00:00Z"}]}`)
}

func put(t *testing.T, kv jetstream.KeyValue, key string, value []byte) uint64 {
	t.Helper()
	rev, err := kv.Put(t.Context(), key, value)
	if err != nil {
		t.Fatalf("put %s: %v", key, err)
	}
	return rev
}

// awaitApplied waits until the follower's floor reaches rev: every delivered revision at or below
// it has been applied to the maintainer, and the follower answers queries, so a tick publishes.
func awaitApplied(t *testing.T, c *Component, rev uint64) {
	t.Helper()
	await(t, fmt.Sprintf("revision %d applied and the follower ready", rev),
		func(ctx context.Context) (bool, error) {
			f := c.follower.Load()
			return f.Indexed() >= rev && f.QueryReady(ctx) == nil, nil
		}, func(ok bool) bool { return ok })
}

// memberCounts reads every COMMUNITY_INDEX record through a reader of the test's own, keyed
// "<level>.<group ID>".
func memberCounts(t *testing.T, client *natsclient.Client) map[string]int {
	t.Helper()
	reader, err := kvcatalog.OpenCatalogReader(t.Context(), client, graph.BucketCommunityIndex)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketCommunityIndex, err)
	}
	keys, err := natsclient.FilteredKeys(t.Context(), reader, ">")
	if err != nil {
		t.Fatalf("list %s: %v", graph.BucketCommunityIndex, err)
	}
	counts := map[string]int{}
	for _, k := range keys {
		entry, err := reader.Get(t.Context(), k)
		if err != nil {
			t.Fatalf("get %s: %v", k, err)
		}
		record, err := clustering.DecodeCommunity(k, entry.Value())
		if err != nil {
			t.Fatalf("decode %s: %v", k, err)
		}
		counts[fmt.Sprintf("%d.%s", record.Level, record.ID)] = record.MemberCount
	}
	return counts
}

// envelope reads graph-clustering's GRAPH_STATUS envelope; ok is false while none is stored.
func envelope(ctx context.Context, client *natsclient.Client) (status graph.IndexStatusResponse, ok bool, err error) {
	reader, err := kvcatalog.OpenCatalogReader(ctx, client, graph.BucketGraphStatus)
	if err != nil {
		return status, false, err
	}
	entry, err := reader.Get(ctx, "graph-clustering")
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return status, false, nil
	}
	if err != nil {
		return status, false, err
	}
	return status, true, json.Unmarshal(entry.Value(), &status)
}

// TestRefusedValueReportedNotWithheld: a live write to ENTITY_STATES whose bytes are not an entity
// state is applied as a refused entity: the tick still publishes every group, the refused entity
// counted in none, Health and the envelope's failed_reasons name refused_entity, and queries are
// not withheld. A later valid write for the key is counted and clears the refusal at the next
// tick (graph-clustering spec, "A value that does not decode"; design D1, D2).
func TestRefusedValueReportedNotWithheld(t *testing.T) {
	client := fixtureClient(t)
	c, states := startClustering(t, client)

	put(t, states, "acme.ops.test.alpha.doc.a1", storedEntity("acme.ops.test.alpha.doc.a1"))
	put(t, states, "acme.ops.test.beta.doc.b1", storedEntity("acme.ops.test.beta.doc.b1"))
	rev := put(t, states, "acme.ops.test.beta.doc.b2", []byte("not an entity state"))
	awaitApplied(t, c, rev)
	c.tick(t.Context())

	want := map[string]int{"0.acme.ops.test.alpha": 1, "0.acme.ops.test.beta": 1, "1.acme.ops.test": 2, "2.acme.ops": 2}
	if got := memberCounts(t, client); !maps.Equal(got, want) {
		t.Fatalf("member counts = %v; want %v, the refused entity in no group", got, want)
	}
	health := c.Health()
	if health.Healthy || !strings.Contains(health.LastError, "refused_entity 1") {
		t.Fatalf("Health = %+v; want unhealthy naming refused_entity 1", health)
	}
	if err := c.follower.Load().QueryReady(t.Context()); err != nil {
		t.Fatalf("QueryReady = %v; want nil, a refused entity withholds nothing", err)
	}
	// The follower publishes the envelope on its heartbeat; the read waits for one carrying the count.
	status := await(t, "an envelope naming refused_entity", func(ctx context.Context) (graph.IndexStatusResponse, error) {
		status, _, err := envelope(ctx, client)
		return status, err
	}, func(s graph.IndexStatusResponse) bool { return s.FailedCount > 0 })
	if status.FailedCount != 1 || !maps.Equal(status.FailedReasons, map[string]uint64{"refused_entity": 1}) ||
		status.State != graph.IndexStateDegraded {
		t.Fatalf("envelope = %+v; want degraded, failed_count 1, failed_reasons {refused_entity: 1}", status)
	}

	rev = put(t, states, "acme.ops.test.beta.doc.b2", storedEntity("acme.ops.test.beta.doc.b2"))
	awaitApplied(t, c, rev)
	c.tick(t.Context())
	want = map[string]int{"0.acme.ops.test.alpha": 1, "0.acme.ops.test.beta": 2, "1.acme.ops.test": 3, "2.acme.ops": 3}
	if got := memberCounts(t, client); !maps.Equal(got, want) {
		t.Fatalf("member counts after the valid write = %v; want %v", got, want)
	}
	if health := c.Health(); !health.Healthy {
		t.Fatalf("Health after the valid write = %+v; want healthy", health)
	}
}

// suiteOwner is graph-clustering as the lifecycle suite's owner. Observe lists what the component
// holds; the follower's watch is private to graph/follower, so TestStopLeavesNothing reads it
// from the server.
type suiteOwner struct{ c *Component }

func (o suiteOwner) Start(ctx context.Context) error { return o.c.Start(ctx) }
func (o suiteOwner) Stop(ctx context.Context) error  { return o.c.Stop(ctx) }

// Observe reads the component's handles without a lock: the suite observes after a Start or Stop
// has returned to it.
func (o suiteOwner) Observe() lifecycletest.Observation {
	var held []string
	if o.c.runCancel != nil {
		held = append(held, "run cancel")
	}
	if o.c.feeds != nil {
		held = append(held, "feeds")
	}
	return lifecycletest.Observation{Unresolved: held}
}

// watchConsumers is the number of consumers on ENTITY_STATES' backing stream: the follower's
// watch is one while graph-clustering runs.
func watchConsumers(ctx context.Context, client *natsclient.Client) (int, error) {
	js, err := client.JetStream()
	if err != nil {
		return -1, err
	}
	stream, err := js.Stream(ctx, "KV_"+graph.BucketEntityStates)
	if err != nil {
		return -1, err
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return -1, err
	}
	return info.State.Consumers, nil
}

// componentStacks returns the stacks of any goroutine in a graph-clustering Component method or a
// Follower method, or "".
func componentStacks() string {
	buf := make([]byte, 1<<20)
	var found []string
	for _, stack := range strings.Split(string(buf[:runtime.Stack(buf, true)]), "\n\n") {
		if strings.Contains(stack, "graphclustering.(*Component)") || strings.Contains(stack, "follower.(*Follower)") {
			found = append(found, stack)
		}
	}
	return strings.Join(found, "\n\n")
}

// TestStopLeavesNothing runs the lifecycle suite on graph-clustering, then stops one that has
// published and finds nothing of it running (graph-clustering spec, "Stop leaves nothing"). The
// must-fail owner's environment has no ENTITY_STATES, so its Start fails in the follower's Open
// before it acquires anything. The component is one-shot, so no restart is promised. The suite
// runs on the real clock, not in a synctest bubble: nats.go's socket read keeps a bubble's clock
// still (notes/2.2.md).
func TestStopLeavesNothing(t *testing.T) {
	client := fixtureClient(t)
	ensureEntityStates(t, client)
	factory := func() lifecycletest.Owner { return suiteOwner{newClustering(t, client, `{}`)} }
	mustFail := func() lifecycletest.Owner {
		js, err := client.JetStream()
		if err != nil {
			t.Fatalf("JetStream: %v", err)
		}
		if err := js.DeleteKeyValue(t.Context(), graph.BucketEntityStates); err != nil &&
			!errors.Is(err, jetstream.ErrBucketNotFound) {
			t.Fatalf("delete %s: %v", graph.BucketEntityStates, err)
		}
		return suiteOwner{newClustering(t, client, `{"startup_attempts": 1, "startup_interval_ms": 1}`)}
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	t.Run("StopAfterATick", func(t *testing.T) {
		states := ensureEntityStates(t, client) // the must-fail owner deleted it
		owner := factory()
		if err := owner.Start(t.Context()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		c := owner.(suiteOwner).c
		if held, want := owner.Observe().Unresolved, []string{"run cancel", "feeds"}; !slices.Equal(held, want) {
			t.Errorf("held while started = %v, want %v", held, want)
		}
		await(t, "the follower's watch on the server",
			func(ctx context.Context) (int, error) { return watchConsumers(ctx, client) },
			func(n int) bool { return n == 1 })
		rev := put(t, states, "acme.ops.test.alpha.doc.a1", storedEntity("acme.ops.test.alpha.doc.a1"))
		awaitApplied(t, c, rev)
		c.tick(t.Context())
		if got := memberCounts(t, client); got["0.acme.ops.test.alpha"] != 1 {
			t.Fatalf("member counts = %v; want the published alpha group", got)
		}

		stopComponent(t, c)
		if held := owner.Observe().Unresolved; len(held) != 0 {
			t.Errorf("held after a nil Stop = %v, want nothing", held)
		}
		// A goroutine that has signalled its join is still in the dump until it exits (#490), so
		// the scan polls; one really left behind is there until go test -timeout.
		await(t, "no goroutine in a Component or Follower method after Stop",
			func(context.Context) (string, error) { return componentStacks(), nil },
			func(stacks string) bool { return stacks == "" })
		await(t, "no nats.go watch delivery after Stop",
			func(context.Context) (string, error) { return natsfixture.NativeDeliveryStacks(), nil },
			func(stacks string) bool { return stacks == "" })
		// nats.go deletes the watch's consumer asynchronously after Stop's unsubscribe.
		await(t, "no consumer left on ENTITY_STATES after Stop",
			func(ctx context.Context) (int, error) { return watchConsumers(ctx, client) },
			func(n int) bool { return n == 0 })
	})
}
