//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_ConcurrentCanonicalAppend verifies the canonical append lane
// preserves every distinct tuple under real JetStream CAS contention.
func TestIntegration_ConcurrentCanonicalAppend(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	natsClient := newFixtureClient(t, entityStream)
	configJSON, err := json.Marshal(DefaultConfig())
	require.NoError(t, err)
	created, err := CreateGraphIngest(configJSON, testDependencies(t, natsClient, withAuthority("c360", "test")))
	require.NoError(t, err)
	c := created.(*Component)
	owner := newGraphIngestTestOwner(c)
	defer owner.finish(ctx, t)
	require.NoError(t, c.Initialize())
	require.NoError(t, c.Start(owner.startContext(ctx)))

	const entityID = "c360.test.cas.concurrent.drone.001"
	require.NoError(t, c.CreateEntity(ctx, newTestEntity(entityID)))

	const writers = 20
	var wg sync.WaitGroup
	var successes atomic.Int32
	start := make(chan struct{})
	for index := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			request, marshalErr := json.Marshal(graph.AppendTriplesRequest{Triples: []message.Triple{{
				Subject: entityID, Predicate: "test.concurrent.value", Object: index,
				Timestamp: time.Now(), Confidence: 1, Source: "cas-test",
			}}})
			if marshalErr != nil {
				return
			}
			body, appendErr := c.handleCanonicalAppend(ctx, request)
			if appendErr != nil {
				return
			}
			var response graph.AppendTriplesResponse
			if json.Unmarshal(body, &response) == nil && len(response.Results) == 1 && response.Results[0].Outcome == graph.MutationApplied {
				successes.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, int32(writers), successes.Load())
	entry, err := c.entityBucket.Get(ctx, entityID)
	require.NoError(t, err)
	var entity graph.EntityState
	require.NoError(t, json.Unmarshal(entry.Value, &entity))
	// The seed's own statement is of another predicate: a create carries one (design D15).
	assert.Equal(t, writers, countDedupTriples(entity, "test.concurrent.value"), "no concurrent append may be lost")
}
