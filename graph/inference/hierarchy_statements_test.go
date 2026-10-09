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

// TestAddToContainersReportsEveryFailure holds design D21: AddToContainers
// returns an error when any part of the inference fails (a container birth, the
// existence check a forward edge rests on, or an inverse edge) and returns no
// statements with it, so graph-ingest can refuse the birth. At the pin a failed
// inverse edge was only a warning. Each container is failed on its own while
// the other two succeed, so a statement returned beside the error shows.
func TestAddToContainersReportsEveryFailure(t *testing.T) {
	const entityID = "c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-001"
	injected := errors.New("injected failure")

	containers := map[string]string{
		"type":     "c360.semstreams-hierarchy-test.sensor.environmental.temperature.group",
		"taxonomy": "c360.semstreams-hierarchy-test.sensor.environmental.group.container",
		"source":   "c360.semstreams-hierarchy-test.sensor.group.container.level",
	}
	failures := map[string]func(*fakeStore){
		"container birth":                func(s *fakeStore) { s.createErr = injected },
		"forward edge's container check": func(s *fakeStore) { s.existsErr = injected },
		"inverse edge":                   func(s *fakeStore) { s.addErr = injected },
	}

	for level, containerID := range containers {
		for failure, arrange := range failures {
			t.Run(level+" "+failure, func(t *testing.T) {
				store := newFakeStore()
				arrange(store)
				store.failOn = containerID
				hi := newTestInference(t, store)

				triples, err := hi.AddToContainers(context.Background(), entityID, hierarchyTestTime)

				require.Error(t, err, "a failed part of the inference must fail the call")
				assert.ErrorIs(t, err, injected)
				assert.Empty(t, triples, "no statements are returned beside an error")
			})
		}
	}
}

// TestAddToContainersStampsSourceAndTime holds design D15: every statement
// the inference derives (the membership edges it returns, the inverse edges it
// writes, and each new container's type statement) names
// graph-ingest's hierarchy producer as its source and carries the triggering
// time it was given, never the clock.
func TestAddToContainersStampsSourceAndTime(t *testing.T) {
	const entityID = "c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-001"
	at := time.Date(2025, 3, 4, 5, 6, 7, 8, time.UTC)

	store := newFakeStore()
	hi := newTestInference(t, store)

	returned, err := hi.AddToContainers(context.Background(), entityID, at)
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

// TestAddToContainersRefusesZeroTime: an inference given no
// triggering time has nothing to stamp its statements with, so it refuses
// before it births a container or writes an edge.
func TestAddToContainersRefusesZeroTime(t *testing.T) {
	store := newFakeStore()
	hi := newTestInference(t, store)

	triples, err := hi.AddToContainers(context.Background(),
		"c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-001", time.Time{})

	require.Error(t, err)
	assert.ErrorIs(t, err, errHierarchyTimeUnset)
	assert.Empty(t, triples)
	assert.Empty(t, store.getCreatedEntities())
	assert.Empty(t, store.getTriples())
}
