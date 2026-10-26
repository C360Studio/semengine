//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/natsclient"
)

// The Graphable lane's poison outcome through the real consume closure and a
// broker (design D8 Q13; graph-ingest-recovery, "A failing payload is poison, not
// a redelivery loop"). Carried from SemStreams PR #1437 at 0ea823a6
// (processor/graph-ingest/fact_lane_fence_integration_test.go), on the test-
// registered fence payload in place of agentic's LoopExecutionEntity; the PR
// asserted an ack-drop, the spec requires a termination.

// TestIntegration_FactLane_InvalidPayloadIsTerminated publishes a payload its own
// Validate refuses (at the PR, a LoopExecutionEntity whose loop_id cannot form an
// entity ID): the lane refuses it before any identity method runs.
func TestIntegration_FactLane_InvalidPayloadIsTerminated(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	c, natsClient, owner := startFenceWireComponent(ctx, t)
	defer owner.finish(ctx, t)

	assertPoisonTerminated(ctx, t, c, natsClient, fenceWireJSON(t, fenceModeInvalid))
}

// TestIntegration_FactLane_PanickingEntityIDIsTerminated is the scenario "Payload
// panics while giving its entity ID" end to end. Without the lane's recover the
// panic reaches natsclient's safeHandleMessage, which Naks, and the message is
// redelivered.
func TestIntegration_FactLane_PanickingEntityIDIsTerminated(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	c, natsClient, owner := startFenceWireComponent(ctx, t)
	defer owner.finish(ctx, t)

	assertPoisonTerminated(ctx, t, c, natsClient, fenceWireJSON(t, fenceModePanicID))
}

// terminatedAdvisory is the part of the broker's MSG_TERMINATED advisory the test
// reads: which stream sequence was terminated, after how many deliveries.
type terminatedAdvisory struct {
	StreamSeq  uint64 `json:"stream_seq"`
	Deliveries uint64 `json:"deliveries"`
}

// assertPoisonTerminated publishes data on the Graphable-lane stream, then a
// conforming message, and asserts the lane's poison outcome through server-side
// state: the broker reports the poison message terminated on its first delivery,
// it is counted once and never redelivered, and the next message is applied.
func assertPoisonTerminated(
	ctx context.Context, t *testing.T, c *Component, natsClient *natsclient.Client, data []byte,
) {
	t.Helper()
	before := c.Health().ErrorCount

	// The broker publishes this advisory when a consumer terminates a message; an
	// acknowledgement publishes none. Subscribed before the publish so it cannot be
	// missed.
	advisories, err := natsClient.GetConnection().SubscribeSync("$JS.EVENT.ADVISORY.CONSUMER.MSG_TERMINATED.ENTITY.>")
	require.NoError(t, err)
	defer func() { assert.NoError(t, advisories.Unsubscribe()) }()

	poison, err := natsClient.PublishToStreamWithAck(ctx, "entity.fence.poison", data)
	require.NoError(t, err)

	waitCtx, cancelWait := context.WithTimeout(ctx, wireRequestTimeout)
	defer cancelWait()
	msg, err := advisories.NextMsgWithContext(waitCtx)
	require.NoError(t, err, "the broker reported no terminated message")
	var advisory terminatedAdvisory
	require.NoError(t, json.Unmarshal(msg.Data, &advisory))
	assert.Equal(t, poison.Sequence, advisory.StreamSeq, "the terminated message is the poison message")
	assert.Equal(t, uint64(1), advisory.Deliveries, "terminated on its first delivery")

	_, err = natsClient.PublishToStreamWithAck(ctx, "entity.fence.next", fenceWireJSON(t, fenceModeConforming))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, _, readErr := c.readEntity(ctx, fenceEntityID)
		return readErr == nil
	}, wireRequestTimeout, 20*time.Millisecond, "the next message on the stream is applied")

	require.Eventually(t, func() bool {
		pending, ackPending, _ := entityStreamConsumerState(ctx, t, natsClient)
		return pending == 0 && ackPending == 0
	}, wireRequestTimeout, 20*time.Millisecond, "both messages are settled")
	_, _, redelivered := entityStreamConsumerState(ctx, t, natsClient)
	assert.Zero(t, redelivered, "the poison message is not redelivered")
	assert.Equal(t, before+1, c.Health().ErrorCount, "the poison message is counted once")
}

// startFenceWireComponent starts graph-ingest on a fixture broker under the
// c360.test authority, with the fence test type registered in the deployment's
// payload registry before the factory builds the decoder from it.
func startFenceWireComponent(ctx context.Context, t *testing.T) (*Component, *natsclient.Client, *graphIngestTestOwner) {
	t.Helper()
	natsClient := newFixtureClient(t, entityStream)

	cfgJSON, err := json.Marshal(DefaultConfig())
	require.NoError(t, err)
	deps := testDependencies(t, natsClient, withAuthority("c360", "test"), withFenceTestPayload(t, nil))
	comp, err := CreateGraphIngest(cfgJSON, deps)
	require.NoError(t, err)
	c := comp.(*Component)
	owner := newGraphIngestTestOwner(c)
	defer owner.provisionalFinish(ctx, t)
	require.NoError(t, c.Initialize())
	require.NoError(t, c.Start(owner.startContext(ctx)))
	owner.transfer()
	return c, natsClient, owner
}

// entityStreamConsumerState sums the ENTITY stream consumers' pending,
// ack-pending and redelivered counts, read from the server.
func entityStreamConsumerState(
	ctx context.Context, t *testing.T, natsClient *natsclient.Client,
) (pending uint64, ackPending int, redelivered int) {
	t.Helper()
	js, err := natsClient.JetStream()
	require.NoError(t, err)
	stream, err := js.Stream(ctx, entityStream.name)
	require.NoError(t, err)
	consumers := stream.ListConsumers(ctx)
	for info := range consumers.Info() {
		pending += info.NumPending
		ackPending += info.NumAckPending
		redelivered += info.NumRedelivered
	}
	require.NoError(t, consumers.Err())
	return pending, ackPending, redelivered
}
