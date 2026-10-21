//go:build integration

package service

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
)

// TestGraphTriplesReadsEntityStates: /graph/triples reads ENTITY_STATES on a live server through
// the manager's NATS client. A subject filter answers that entity's triples and no other's; the
// answer is the first limit triples of the entities taken in sorted key order, whatever order
// they were written in; a malformed filter answers 400 naming it; an entity the decoder refuses
// answers 500 (design D6, Q-A).
//
// spec: operator-surface / /graph/triples answers a sorted, validated read of ENTITY_STATES
func TestGraphTriplesReadsEntityStates(t *testing.T) {
	client := fixtureClient(t)
	bucket, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEntityStates)
	require.NoError(t, err)

	first := semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "1")
	second := semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "2")
	third := semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "3")
	state := semantictest.Predicate(t, "test", "state", "value")
	owner := semantictest.Predicate(t, "test", "state", "owner")
	triples := map[string][]message.Triple{}
	// Written out of key order, so only a sort before the limit answers the first key's triple.
	for _, id := range []string{third, first, second} {
		triples[id] = []message.Triple{entityTriple(id, state, "on"), entityTriple(id, owner, "ops")}
		_, err := bucket.Put(t.Context(), id, storedEntity(t, id, triples[id]))
		require.NoError(t, err)
	}

	deps := createTestServiceDependencies()
	deps.NATSClient = client
	manager := createTestServiceManager(ManagerConfig{}, deps)
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	startTestManager(t, manager)
	url := observedManagerHTTPURL(t, manager) + "/graph/triples"

	status, body, err := httpGet(t.Context(), url+"?subject="+second)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, body)
	require.JSONEq(t, tripleJSON(t, triples[second]), body)

	status, body, err = httpGet(t.Context(), url+"?limit=1")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, body)
	require.JSONEq(t, tripleJSON(t, triples[first][:1]), body)

	status, body, err = httpGet(t.Context(), url+"?subject=a.b")
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, body, "subject")

	refused := semantictest.EntityID(t, "acme", "ops", "test", "service", "entity", "0")
	_, err = bucket.Put(t.Context(), refused, []byte(`{"id":"not-an-entity-id","triples":[]}`))
	require.NoError(t, err)
	status, body, err = httpGet(t.Context(), url)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, status, "an entity the decoder refuses was answered: %s", body)
}

// TestHealthPublisherPublishesEachServiceAtStart: with a NATS client, StartAll publishes each
// composed service's health on health.service.<name> as an object with the keys timestamp,
// name, status and health (design D2, S8).
//
// spec: service-lifecycle / The service manager publishes health and counts its drops
func TestHealthPublisherPublishesEachServiceAtStart(t *testing.T) {
	client := fixtureClient(t)
	stream, err := client.EnsureStream(t.Context(), jetstream.StreamConfig{
		Name:     "HEALTH",
		Subjects: []string{"health.>"},
		Storage:  jetstream.MemoryStorage,
		MaxAge:   time.Hour,
		MaxBytes: 1 << 20,
		Discard:  jetstream.DiscardOld,
	})
	require.NoError(t, err)

	deps := createTestServiceDependencies()
	deps.NATSClient = client
	manager := createTestServiceManager(ManagerConfig{}, deps)
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	require.NoError(t, manager.RegisterInstance("probe", &MockService{name: "probe", status: StatusRunning, healthy: true}))
	startTestManager(t, manager)

	consumer, err := stream.OrderedConsumer(t.Context(), jetstream.OrderedConsumerConfig{
		FilterSubjects: []string{"health.service.probe"},
	})
	require.NoError(t, err)
	msg, err := consumer.Next(jetstream.FetchMaxWait(clientTimeout))
	require.NoError(t, err, "no health message for probe")

	var published map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(msg.Data(), &published), string(msg.Data()))
	for _, key := range []string{"timestamp", "name", "status", "health"} {
		require.Contains(t, published, key)
	}
	require.JSONEq(t, `"probe"`, string(published["name"]))
	require.JSONEq(t, `"running"`, string(published["status"]))
}
