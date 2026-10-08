//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/inference"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/payloadfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
)

// fixtureClientTimeout is the fixture client's request timeout, the value the pin's
// NewTestClient family used.
const fixtureClientTimeout = 15 * time.Second

// openFixtureClient opens a natsclient.Client the way the pin's NewTestClient built it: no
// reconnects, no health monitor. It is the open function this package gives
// natsfixture.Open (design D3), which bounds it and closes it on cleanup.
func openFixtureClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(fixtureClientTimeout),
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

// fixtureStream is a stream the fixture creates and owns: the pin's
// natsclient.TestStreamConfig.
type fixtureStream struct {
	name     string
	subjects []string
}

// entityStream is the ENTITY stream on entity.> that DefaultConfig's input port reads.
var entityStream = fixtureStream{name: "ENTITY", subjects: []string{"entity.>"}}

// newFixtureClient replaces the pin's natsclient.NewTestClient(t, WithKV(),
// WithStreams(streams...)) (design D3): a broker of this test's own, the streams created
// on it, and a client connected through natsfixture.Open. The fixture's Stop and the
// client's Close run on cleanup, each under a bounded context, the Close first.
func newFixtureClient(t *testing.T, streams ...fixtureStream) *natsclient.Client {
	t.Helper()
	f := natsfixture.New(t)
	if err := f.Start(t.Context()); err != nil {
		t.Fatalf("natsfixture Start: %v", err)
	}
	for _, s := range streams {
		if _, err := f.CreateStream(t.Context(), s.name, s.subjects...); err != nil {
			t.Fatalf("CreateStream %s: %v", s.name, err)
		}
	}
	return natsfixture.Open(t, f, openFixtureClient)
}

// startBatchTestComponent starts graph-ingest with its default configuration on a
// fixture broker under the c360.test authority. At the pin it is in
// batch_integration_test.go:23-50, which a later stage ports; it moves here with its
// first ported reader (keyed_ingest_integration_test.go). The pin slept 100ms after
// Start; nothing was waited for, because Start returns only once start has
// provisioned the buckets and bound the consumers and handlers, so the sleep is gone.
func startBatchTestComponent(ctx context.Context, t *testing.T) (*Component, *graphIngestTestOwner) {
	t.Helper()

	natsClient := newFixtureClient(t, entityStream)

	config := DefaultConfig()
	deps := testDependencies(t, natsClient, withAuthority("c360", "test"))

	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	comp, err := CreateGraphIngest(configJSON, deps)
	require.NoError(t, err)

	c := comp.(*Component)
	owner := newGraphIngestTestOwner(c)
	defer owner.provisionalFinish(ctx, t)
	require.NoError(t, c.Initialize())
	require.NoError(t, c.Start(owner.startContext(ctx)))

	owner.transfer()
	return c, owner
}

// startPrefixTestComponent starts graph-ingest with its default configuration on a
// fixture broker under the authority opts give, and returns its client for prefix
// queries. At the pin it is in query_prefix_integration_test.go:22-52, which a later
// stage ports; it moves here with its first ported readers
// (cache_coherence_integration_test.go, cache_stale_repopulation_integration_test.go).
// The pin slept 100ms after Start "to allow subscriptions to stabilise"; Start returns
// only once the query handlers are subscribed, so the sleep is gone, as in
// startBatchTestComponent.
func startPrefixTestComponent(ctx context.Context, t *testing.T, opts ...testComponentOption) (*Component, *natsclient.Client, *graphIngestTestOwner) {
	t.Helper()

	nc := newFixtureClient(t, entityStream)

	config := DefaultConfig()
	deps := testDependencies(t, nc, opts...)
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	comp, err := CreateGraphIngest(configJSON, deps)
	require.NoError(t, err)

	c := comp.(*Component)
	owner := newGraphIngestTestOwner(c)
	defer owner.provisionalFinish(ctx, t)
	require.NoError(t, c.Initialize())
	require.NoError(t, c.Start(owner.startContext(ctx)))

	owner.transfer()
	return c, nc, owner
}

// seedPrefixEntity writes a minimal entity to the KV bucket through CreateEntity so
// it shows up in prefix queries. It moves here from the pin's
// query_prefix_integration_test.go:54-68 with startPrefixTestComponent; the pin's
// Version: 1 is gone (design D15).
func seedPrefixEntity(t *testing.T, ctx context.Context, c *Component, id string) {
	t.Helper()
	entity := &graph.EntityState{
		ID:          id,
		MessageType: testEntityType(),
		Triples: []message.Triple{
			{Subject: id, Predicate: "test.entity.attribute", Object: "val", Timestamp: time.Now()},
		},
		UpdatedAt: time.Now(),
	}
	require.NoError(t, c.CreateEntity(ctx, entity))
}

// mergeTestGraphable is a minimal Graphable payload that stamps a caller-supplied
// triple set on an entity ID. It and registerMergeTestPayload are in the pin's
// merge_entity_integration_test.go:329-378, which a later stage ports; they move here
// with their first ported readers.
type mergeTestGraphable struct {
	entityID string
	triples  []message.Triple
}

func (g *mergeTestGraphable) EntityID() string          { return g.entityID }
func (g *mergeTestGraphable) Triples() []message.Triple { return g.triples }
func (g *mergeTestGraphable) Schema() message.Type {
	return message.Type{Domain: "test", Category: "merge", Version: "v1"}
}

func (g *mergeTestGraphable) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		EntityID string           `json:"entity_id"`
		Triples  []message.Triple `json:"triples"`
	}{g.entityID, g.triples})
}

func (g *mergeTestGraphable) UnmarshalJSON(data []byte) error {
	var v struct {
		EntityID string           `json:"entity_id"`
		Triples  []message.Triple `json:"triples"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	g.entityID = v.EntityID
	g.triples = v.Triples
	return nil
}

func (g *mergeTestGraphable) Validate() error { return nil }

// registerMergeTestPayload gives c a decoder that decodes test.merge.v1 into a
// mergeTestGraphable. The pin started from payloadbuiltins.Register (:369); design D2
// takes the per-package RegisterPayloads graph-ingest's fixture registry uses instead.
func registerMergeTestPayload(t *testing.T, c *Component) {
	t.Helper()
	reg := payloadfixture.NewWithSubset(t, message.RegisterPayloads, inference.RegisterPayloads)
	require.NoError(t, reg.Register(&payloadregistry.Registration{
		Domain:      "test",
		Category:    "merge",
		Version:     "v1",
		Description: "merge-entity integration-test payload",
		Factory:     func() any { return &mergeTestGraphable{} },
	}))
	c.decoder = message.NewDecoder(reg)
}
