package graphingest

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semengine/message"
)

// TestInputLaneNaksAfterFailedStartRollback: a Start that builds the ingest pool and an input
// lane, then fails, is rolled back by release, which ends the lane's submit context and shuts
// its pool down. A consumer whose setup failed is not recorded for the rollback to drain, so
// its callback can still run (checkpoint 3 B2). An input that reaches the lane then, one that
// decodes and would otherwise be applied, is Nak'd so the server redelivers it: it is never
// acked or terminated, nothing is stored, and nothing panics.
func TestInputLaneNaksAfterFailedStartRollback(t *testing.T) {
	const entityID = "c360.test.robotics.gcs.drone.straddle"
	c, _ := createTestComponentWithMockKVBucket(t, withAuthority("c360", "test"))
	registerMergeTestPayload(t, c)
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	errStart := errors.New("start fails after building the input lane")
	var lane *inputLane
	var submitCtx context.Context
	// As start does: the pool and submit contexts are ended by release, through the cancels
	// recorded on the component.
	failingStart := func(ctx context.Context) error {
		poolCtx, poolCancel := context.WithCancel(ctx)
		c.ingestPoolCancel = poolCancel
		var submitCancel context.CancelFunc
		submitCtx, submitCancel = context.WithCancel(ctx)
		c.ingestSubmitCancel = submitCancel
		if err := c.buildIngestPool(poolCtx); err != nil {
			return err
		}
		lane = &inputLane{
			c:          c,
			pool:       c.ingestPool,
			consumer:   testConsumer,
			heartbeat:  testHeartbeat,
			generation: 1,
			subject:    "entity.>",
		}
		return errStart
	}
	if err := c.lifecycle.Start(t.Context(), failingStart, c.release); !errors.Is(err, errStart) {
		t.Fatalf("Start = %v, want the start failure", err)
	}
	// Start returned, so the rollback's release has run.
	if !errors.Is(submitCtx.Err(), context.Canceled) {
		t.Fatalf("submit context after the rollback = %v, want canceled", submitCtx.Err())
	}

	payload := &mergeTestGraphable{entityID: entityID, triples: []message.Triple{
		{Subject: entityID, Predicate: "test.state.value", Object: "streamed", Confidence: 1},
	}}
	msg := newStreamLaneMsg(streamLaneData(t, payload, "straddle-producer"), 1)
	// deliver settles a refused submit before it returns, on this goroutine.
	lane.deliver(submitCtx, msg)

	select {
	case disposition := <-msg.settled:
		if disposition != "nak" {
			t.Fatalf("disposition = %s, want nak", disposition)
		}
	default:
		t.Fatal("deliver returned without settling the input")
	}
	select {
	case extra := <-msg.settled:
		t.Fatalf("a second disposition after the nak: %s", extra)
	default:
	}
	if _, _, ok := storedRevision(t, c, entityID); ok {
		t.Fatalf("entity %s was stored by an input the rollback refused", entityID)
	}
}
