//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

// TestIntegration_DataFlowCountsInputMessages: DataFlow counts each input message once,
// a stream delivery or a mutation request, and its payload bytes; containers born for a
// message, a merge that writes nothing, and queries add no message (#302; design D2). The
// oracle is the test's own tally of what it sent.
func TestIntegration_DataFlowCountsInputMessages(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	comp := createTestComponentWithHierarchyConfig(t, true)
	owner := newGraphIngestTestOwner(comp)
	defer owner.finish(ctx, t)
	require.NoError(t, comp.Initialize())
	registerMergeTestPayload(t, comp) // decoder before Start: no consumer race
	require.NoError(t, comp.Start(owner.startContext(ctx)))

	const (
		entityID    = "c360.platform.robotics.mav1.drone.flow001"
		containerID = "c360.platform.robotics.mav1.drone.group"
		absentID    = "c360.platform.robotics.mav1.drone.flow404"
	)
	var sentMessages, sentBytes int64
	publish := func(timestamp time.Time) {
		t.Helper()
		payload := &mergeTestGraphable{entityID: entityID, triples: []message.Triple{{
			Subject: entityID, Predicate: "test.flow.value", Object: "v", Source: "dataflow-test",
			Timestamp: timestamp, Confidence: 1.0,
		}}}
		data, err := json.Marshal(message.NewBaseMessage(payload.Schema(), payload, "dataflow-test"))
		require.NoError(t, err)
		require.NoError(t, comp.natsClient.PublishToStream(ctx, "entity."+entityID, data))
		sentMessages++
		sentBytes += int64(len(data))
	}
	// One stream message whose birth creates its containers.
	now := time.Now()
	publish(now)
	require.Eventually(t, func() bool {
		_, _, err := comp.readEntity(ctx, entityID)
		return err == nil
	}, 10*time.Second, 20*time.Millisecond, "the entity never landed")
	_, _, err := comp.readEntity(ctx, containerID)
	require.NoError(t, err, "the birth created its type container")

	// One stream message whose only set is older than the stored one: its merge writes nothing.
	staleBefore := testutil.ToFloat64(comp.staleSets)
	publish(now.Add(-time.Hour))
	require.Eventually(t, func() bool {
		return testutil.ToFloat64(comp.staleSets) == staleBefore+1
	}, 10*time.Second, 20*time.Millisecond, "the older set was never declared")

	// One append to three subjects: two applied, one absent.
	updatedBefore := testutil.ToFloat64(comp.entitiesUpdated)
	appendRequest := graph.AppendTriplesRequest{Triples: withTestMetadata(
		message.Triple{Subject: entityID, Predicate: "test.flow.extra", Object: "a", Timestamp: now},
		message.Triple{Subject: containerID, Predicate: "test.flow.extra", Object: "b", Timestamp: now},
		message.Triple{Subject: absentID, Predicate: "test.flow.extra", Object: "c", Timestamp: now},
	)}
	appendData, err := json.Marshal(appendRequest)
	require.NoError(t, err)
	reply, err := comp.natsClient.RequestClassified(ctx, "graph.mutation.triple.append", appendData, wireRequestTimeout)
	require.NoError(t, err)
	sentMessages++
	sentBytes += int64(len(appendData))
	var appended graph.AppendTriplesResponse
	require.NoError(t, json.Unmarshal(reply, &appended))
	applied := 0
	for _, result := range appended.Results {
		if result.Outcome == graph.MutationApplied {
			applied++
		}
	}
	require.Equal(t, 2, applied, "append results: %#v", appended.Results)
	assert.Equal(t, float64(applied), testutil.ToFloat64(comp.entitiesUpdated)-updatedBefore,
		"entities_updated_total rises by the append's applied subjects")

	// One refused create.
	refusedData, err := json.Marshal(graph.CreateEntityRequest{})
	require.NoError(t, err)
	_, err = comp.natsClient.RequestClassified(ctx, "graph.mutation.entity.create", refusedData, wireRequestTimeout)
	require.Error(t, err, "a create with no entity is refused")
	sentMessages++
	sentBytes += int64(len(refusedData))

	// One entity query: a read, not an input message.
	queryData, err := json.Marshal(map[string]string{"id": entityID})
	require.NoError(t, err)
	_, err = comp.natsClient.RequestClassified(ctx, "graph.ingest.query.entity", queryData, wireRequestTimeout)
	require.NoError(t, err)

	require.Equal(t, int64(4), sentMessages, "two stream messages and two mutation requests were sent")
	assert.Equal(t, sentMessages, comp.DataFlow().Messages,
		"one count per stream delivery and per mutation request")
	assert.Equal(t, sentBytes, comp.DataFlow().Bytes,
		"the payload bytes of the counted messages")
}
