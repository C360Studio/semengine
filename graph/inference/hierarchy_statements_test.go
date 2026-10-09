package inference

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hierarchyLevel names one container level's switch in HierarchyConfig.
type hierarchyLevel struct {
	name   string
	config HierarchyConfig
}

var hierarchyLevels = []hierarchyLevel{
	{"type", HierarchyConfig{Enabled: true, CreateTypeEdges: true}},
	{"taxonomy", HierarchyConfig{Enabled: true, CreateSystemEdges: true}},
	{"source", HierarchyConfig{Enabled: true, CreateDomainEdges: true}},
}

// TestGetHierarchyTriplesReportsEveryFailure holds design D21: GetHierarchyTriples
// returns an error when any part of the inference fails (a container birth, the
// existence check a forward edge rests on, or an inverse edge) and returns no
// statements with it, so graph-ingest can refuse the birth. At the pin a failed
// inverse edge was only a warning. Each
// container level is failed on its own, so a level that drops its error fails
// its own case.
func TestGetHierarchyTriplesReportsEveryFailure(t *testing.T) {
	const entityID = "c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-001"
	injected := errors.New("injected failure")

	type failureCase struct {
		name    string
		config  HierarchyConfig
		arrange func(*fakeStore)
	}
	var cases []failureCase
	for _, level := range hierarchyLevels {
		cases = append(cases,
			failureCase{level.name + " container birth", level.config,
				func(s *fakeStore) { s.createErr = injected }},
			failureCase{level.name + " forward edge's container check", level.config,
				func(s *fakeStore) { s.existsErr = injected }},
			failureCase{level.name + " inverse edge", level.config,
				func(s *fakeStore) { s.addErr = injected }},
		)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			tc.arrange(store)
			hi := NewHierarchyInference(store, store, tc.config, hierarchyTestAuthority, nil)

			triples, err := hi.GetHierarchyTriples(context.Background(), entityID, hierarchyTestTime)

			require.Error(t, err, "a failed part of the inference must fail the call")
			assert.ErrorIs(t, err, injected)
			assert.Empty(t, triples, "no statements are returned beside an error")
		})
	}
}

// TestGetHierarchyTriplesStampsSourceAndTime holds design D15: every statement
// the inference derives (the membership edges it returns, the inverse edges it
// writes, and each new container's type statement) names
// graph-ingest's hierarchy producer as its source and carries the triggering
// time it was given, never the clock.
func TestGetHierarchyTriplesStampsSourceAndTime(t *testing.T) {
	const entityID = "c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-001"
	at := time.Date(2025, 3, 4, 5, 6, 7, 8, time.UTC)

	store := newFakeStore()
	hi := NewHierarchyInference(store, store, HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: true,
		CreateDomainEdges: true,
	}, hierarchyTestAuthority, nil)

	returned, err := hi.GetHierarchyTriples(context.Background(), entityID, at)
	require.NoError(t, err)

	// Three membership edges are returned; three container inverse edges are
	// written; three containers are born with one statement each.
	require.Len(t, returned, 3)
	added := store.getTriples()
	require.Len(t, added, 3)
	created := store.getCreatedEntities()
	require.Len(t, created, 3)

	check := func(kind string, triples []message.Triple) {
		t.Helper()
		for _, triple := range triples {
			assert.Equal(t, graph.SourceHierarchy, triple.Source,
				"%s %s %s: source", kind, triple.Subject, triple.Predicate)
			assert.True(t, triple.Timestamp.Equal(at),
				"%s %s %s: timestamp %v, want %v", kind, triple.Subject, triple.Predicate, triple.Timestamp, at)
		}
	}
	check("returned", returned)
	check("added", added)
	for _, container := range created {
		require.Len(t, container.Triples, 1)
		check("container", container.Triples)
	}
}

// TestGetHierarchyTriplesRefusesZeroTime: an enabled inference given no
// triggering time has nothing to stamp its statements with, so it refuses
// before it births a container or writes an edge.
func TestGetHierarchyTriplesRefusesZeroTime(t *testing.T) {
	store := newFakeStore()
	hi := NewHierarchyInference(store, store,
		HierarchyConfig{Enabled: true, CreateTypeEdges: true}, hierarchyTestAuthority, nil)

	triples, err := hi.GetHierarchyTriples(context.Background(),
		"c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-001", time.Time{})

	require.Error(t, err)
	assert.ErrorIs(t, err, errHierarchyTimeUnset)
	assert.Empty(t, triples)
	assert.Empty(t, store.getCreatedEntities())
	assert.Empty(t, store.getTriples())
}
