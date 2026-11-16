//go:build integration

package graphlexical

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/storage/objectstore"
	"github.com/c360studio/semengine/storage/storeregistry"
)

// bodyBucket is the object store the body tests read, under the instance stateEntry names.
const bodyBucket = "LEXICAL_BODIES"

// openBodyStore opens bodyBucket, creating it if it is absent, as the objectstore component
// provides it: under the instance "objectstore".
func openBodyStore(t *testing.T, client *natsclient.Client) *objectstore.Store {
	t.Helper()
	store, err := objectstore.NewStoreWithConfig(t.Context(), client,
		objectstore.Config{BucketName: bodyBucket, InstanceName: "objectstore"})
	if err != nil {
		t.Fatalf("open object store %s: %v", bodyBucket, err)
	}
	return store
}

// registered is a store registry holding store under its instance.
func registered(t *testing.T, store *objectstore.Store) *storeregistry.Registry {
	t.Helper()
	stores := storeregistry.New()
	if err := stores.Register(store.InstanceName(), store); err != nil {
		t.Fatalf("register %s: %v", store.InstanceName(), err)
	}
	return stores
}

// status reads graph-lexical's readiness envelope from GRAPH_STATUS.
func status(t *testing.T, client *natsclient.Client) func(context.Context) (graph.IndexStatusResponse, error) {
	t.Helper()
	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	return func(ctx context.Context) (graph.IndexStatusResponse, error) {
		var s graph.IndexStatusResponse
		kv, err := js.KeyValue(ctx, graph.BucketGraphStatus)
		if err != nil {
			return s, nil // the follower has not created it yet
		}
		entry, err := kv.Get(ctx, key)
		if err != nil {
			return s, nil // nothing published yet
		}
		return s, json.Unmarshal(entry.Value(), &s)
	}
}

// requireNotIndexed fails unless the bodies_not_indexed gauge reads want by state.
func requireNotIndexed(t *testing.T, c *Component, want map[string]int) {
	t.Helper()
	for _, name := range bodyStateNames[1:] {
		if got := testutil.ToFloat64(c.metrics.bodies.WithLabelValues(name)); got != float64(want[name]) {
			t.Errorf("bodies_not_indexed{state=%q} = %v; want %d", name, got, want[name])
		}
	}
}

// TestUnreadBodyCountedThenRepairedOnObjectStore: an entity's body cannot be read at start,
// because its object store's bucket is gone; a search for a body-only word misses it, one body is
// counted unread, the key reads degraded and Health names body_unread. Once the bucket is back
// with the body in it, the next repair pass reads it: the word is found, nothing is counted and
// the key reads ready.
func TestUnreadBodyCountedThenRepairedOnObjectStore(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	store := openBodyStore(t, client)
	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	if err := js.DeleteObjectStore(t.Context(), bodyBucket); err != nil {
		t.Fatalf("delete %s: %v", bodyBucket, err)
	}
	rev := put(t, states, pump, text{name: "pumpname", offloaded: true})

	c := startLexical(t, client, metric.NewMetricsRegistry(), registered(t, store))
	read := status(t, client)
	await(t, "the key reads degraded", read, func(s graph.IndexStatusResponse) bool {
		return s.State == graph.IndexStateDegraded && s.FailedCount == 1 && s.FailedReasons[failureBodyUnread] == 1
	})
	if f := c.follower.Load(); f.Indexed() < rev {
		t.Fatalf("floor %d below %d with the body unread; want the revision complete", f.Indexed(), rev)
	}
	if ids := hitIDs(search(t, c, "pumpbody")); len(ids) != 0 {
		t.Fatalf("search for a body-only word found %v before the body was read", ids)
	}
	if ids := hitIDs(search(t, c, "pumpname")); len(ids) != 1 {
		t.Fatalf("search for the name found %v; want the entity, indexed by its inline text", ids)
	}
	requireNotIndexed(t, c, map[string]int{"unread": 1})
	if h := c.Health(); h.Healthy || !strings.Contains(h.LastError, failureBodyUnread) {
		t.Fatalf("Health = %+v; want unhealthy, naming %s", h, failureBodyUnread)
	}

	back := openBodyStore(t, client)
	if err := back.Put(t.Context(), pump+"/body", []byte("pumpbody")); err != nil {
		t.Fatalf("put the body: %v", err)
	}
	// The next repair pass is at most repairInterval away; go test -timeout is the one deadline.
	await(t, "the key reads ready", read, func(s graph.IndexStatusResponse) bool {
		return s.Ready && s.State == graph.IndexStateReady
	})
	if ids := hitIDs(search(t, c, "pumpbody")); len(ids) != 1 {
		t.Fatalf("search for the body-only word found %v after the repair pass; want the entity", ids)
	}
	requireNotIndexed(t, c, map[string]int{})
	if h := c.Health(); !h.Healthy {
		t.Fatalf("Health = %+v after the repair pass; want healthy", h)
	}
}

// TestMissingAndUnresolvedBodiesOnObjectStore: one entity's body is missing from its object store
// and another names an instance no store is registered under. Both are counted, missing and no
// store, the key reads ready and Health is healthy.
func TestMissingAndUnresolvedBodiesOnObjectStore(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	store := openBodyStore(t, client)
	put(t, states, pump, text{name: "pumpname", offloaded: true})
	rev := put(t, states, valve, text{name: "valvename", offloaded: true, instance: "elsewhere"})

	c := startLexical(t, client, metric.NewMetricsRegistry(), registered(t, store))
	awaitApplied(t, c, rev)
	await(t, "the key reads ready", status(t, client), func(s graph.IndexStatusResponse) bool {
		return s.Ready && s.State == graph.IndexStateReady && s.FailedCount == 0
	})
	requireNotIndexed(t, c, map[string]int{"missing": 1, "no_store": 1})
	if h := c.Health(); !h.Healthy {
		t.Fatalf("Health = %+v; want healthy", h)
	}
}
