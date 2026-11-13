//go:build integration

package graphlexical

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/graph/lexical"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/storage/storeregistry"
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

// newLexical builds an initialized, unstarted graph-lexical from raw through CreateGraphLexical,
// on registry, reading bodies from stores (nil for none), logging nowhere.
func newLexical(t *testing.T, client *natsclient.Client, raw string, registry *metric.MetricsRegistry,
	stores *storeregistry.Registry) *Component {
	t.Helper()
	built, err := CreateGraphLexical(json.RawMessage(raw), component.Dependencies{
		NATSClient: client, MetricsRegistry: registry, StoreRegistry: stores, Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("CreateGraphLexical: %v", err)
	}
	c := built.(*Component)
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

// stopBound bounds a cleanup Stop: long enough for any healthy Stop, so the bound never decides
// a test.
const stopBound = time.Hour

func stopComponent(t *testing.T, c *Component) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stopBound)
	defer cancel()
	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

// startLexical builds graph-lexical on registry, reading bodies from stores (nil for none), and
// starts it; the test's cleanup stops it.
func startLexical(t *testing.T, client *natsclient.Client, registry *metric.MetricsRegistry,
	stores *storeregistry.Registry) *Component {
	t.Helper()
	c := newLexical(t, client, `{}`, registry, stores)
	if err := c.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopComponent(t, c) })
	return c
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

// awaitApplied waits until the follower's floor reaches rev and it answers queries: every
// delivered revision at or below rev is in the index.
func awaitApplied(t *testing.T, c *Component, rev uint64) {
	t.Helper()
	await(t, fmt.Sprintf("revision %d applied and the follower ready", rev),
		func(ctx context.Context) (bool, error) {
			f := c.follower.Load()
			return f.Indexed() >= rev && f.QueryReady(ctx) == nil, nil
		}, func(ok bool) bool { return ok })
}

func put(t *testing.T, kv jetstream.KeyValue, key string, tx text) uint64 {
	t.Helper()
	rev, err := kv.Put(t.Context(), key, stateEntry(t, key, 0, tx).Value)
	if err != nil {
		t.Fatalf("put %s: %v", key, err)
	}
	return rev
}

// remove deletes key and returns the delete marker's revision, the stream's last sequence:
// nothing else writes the bucket.
func remove(t *testing.T, kv jetstream.KeyValue, key string) uint64 {
	t.Helper()
	if err := kv.Delete(t.Context(), key); err != nil {
		t.Fatalf("delete %s: %v", key, err)
	}
	status, err := kv.Status(t.Context())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	return status.(*jetstream.KeyValueBucketStatus).StreamInfo().State.LastSeq
}

// search asks c's index for query, at most ten hits.
func search(t *testing.T, c *Component, query string) lexical.Result {
	t.Helper()
	q, err := lexical.ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", query, err)
	}
	result, err := c.apply.index.Search(q, lexical.Options{Limit: 10})
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}
	return result
}

func hitIDs(result lexical.Result) []string {
	ids := make([]string, 0, len(result.Hits))
	for _, hit := range result.Hits {
		ids = append(ids, hit.EntityID)
	}
	return ids
}

// TestRestartRebuildsSameAnswers: entities are written, one edited and one deleted before
// graph-lexical starts; a search finds only the live entities whose current words match, and a
// second graph-lexical started after the first stopped gives the same hits, order, scores and
// match counts.
func TestRestartRebuildsSameAnswers(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	const (
		north = "c360.lex.test.site.pump.north"
		south = "c360.lex.test.site.pump.south"
		east  = "c360.lex.test.site.valve.east"
		west  = "c360.lex.test.site.valve.west"
	)
	put(t, states, north, text{name: "north pump", inline: "feeds the cooling loop"})
	put(t, states, south, text{name: "south pump", inline: "feeds the boiler"})
	put(t, states, east, text{name: "east valve", inline: "drains the boiler"})
	put(t, states, west, text{name: "west valve", inline: "drains the cooling loop"})
	put(t, states, south, text{name: "south pump", inline: "spare, offline"})
	last := remove(t, states, west)

	// Which entities hold each query's words, written out from the writes above.
	queries := map[string][]string{
		"pump":    {north, south},
		"valve":   {east},
		"boiler":  {east},
		"cooling": {north},
		"offline": {south},
		"west":    nil,
		"loop":    {north},
	}
	answers := func(c *Component) map[string]lexical.Result {
		got := map[string]lexical.Result{}
		for query, want := range queries {
			result := search(t, c, query)
			if ids := hitIDs(result); !slices.Equal(slices.Sorted(slices.Values(ids)), want) {
				t.Fatalf("search %q found %v; want %v", query, ids, want)
			}
			got[query] = result
		}
		return got
	}

	first := newLexical(t, client, `{}`, metric.NewMetricsRegistry(), nil)
	if err := first.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	awaitApplied(t, first, last)
	before := answers(first)
	stopComponent(t, first)

	second := startLexical(t, client, metric.NewMetricsRegistry(), nil)
	awaitApplied(t, second, last)
	after := answers(second)
	for query, want := range before {
		if got := after[query]; !slices.Equal(got.Hits, want.Hits) || got.Matched != want.Matched {
			t.Fatalf("search %q after the restart = %+v; before it = %+v", query, got, want)
		}
	}
}

// TestIndexGauges: after three entities are written and one deleted, the entity gauge reads 2 and
// the word gauge the words the two hold, on the registry graph-lexical was given.
func TestIndexGauges(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	registry := metric.NewMetricsRegistry()
	c := startLexical(t, client, registry, nil)

	put(t, states, "c360.lex.test.site.pump.a", text{name: "alpha pump", inline: "north"}) // 3 words
	put(t, states, "c360.lex.test.site.pump.b", text{name: "bravo valve"})                 // 2 words
	put(t, states, "c360.lex.test.site.pump.c", text{name: "charlie tank", inline: "spare"})
	rev := remove(t, states, "c360.lex.test.site.pump.c")
	awaitApplied(t, c, rev)

	families, err := registry.PrometheusRegistry().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	got := map[string]float64{}
	for _, family := range families {
		for _, m := range family.GetMetric() {
			got[family.GetName()] = m.GetGauge().GetValue()
		}
	}
	want := map[string]float64{"semengine_graph_lexical_index_entities": 2, "semengine_graph_lexical_index_words": 5}
	for name, value := range want {
		if v, ok := got[name]; !ok || v != value {
			t.Errorf("%s = %v (present %t); want %v", name, v, ok, value)
		}
	}
}
