package graphingest

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop proves graph-ingest's side of
// component.LifecycleComponent's read line (design D2, "No wait for readers"): Health and
// DataFlow return while the component's own Stop is still running, and again after it
// returned, each reporting a stopped component. Stop is held open by the consumer fixture
// of TestLifecycleOwnerRunningStopPreservesEffectSettlementOrder: release clears running,
// the cleanup drains the consumer, and then waits for its Closed, which the test holds.
// On today's tree the test passes; it is shown able to fail by design D7's mutant (release
// holding c.mu across its cleanup), under which Health blocks and the test does not return.
func TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop(t *testing.T) {
	consumer := &graphIngestLifecycleConsumeContext{
		closed: make(chan struct{}), drainSeen: make(chan struct{}),
	}
	runCtx, runCancel := context.WithCancel(t.Context())
	owner := createTestComponent(t)
	owner.running = true
	// A start a minute ago and sixty messages since: while running, DataFlow reports a
	// rate above zero, so the zero rate after release cleared running is a change.
	owner.startTime = time.Now().Add(-time.Minute)
	atomic.StoreInt64(&owner.messagesProcessed, 60)
	owner.cancel = runCancel
	owner.consumers = []graphIngestConsumerBinding{{handle: consumer}}
	startGuardAcquiringNothing(t, owner)

	if health := owner.Health(); !health.Healthy || health.Status != "running" {
		t.Fatalf("Health before Stop = {Healthy: %v, Status: %q}, want {true, \"running\"}",
			health.Healthy, health.Status)
	}
	if flow := owner.DataFlow(); flow.MessagesPerSecond <= 0 {
		t.Fatalf("DataFlow before Stop: MessagesPerSecond = %v, want above zero", flow.MessagesPerSecond)
	}

	stopResult := make(chan error, 1)
	go func() { stopResult <- owner.Stop(t.Context()) }()
	<-consumer.drainSeen // release has cleared running; cleanup now waits for Closed

	assertStoppedReads(t, owner, "during Stop")
	select {
	case err := <-stopResult:
		t.Fatalf("Stop returned %v before the consumer reported Closed; the reads did not run during Stop", err)
	default:
	}

	close(consumer.closed)
	if err := <-stopResult; err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if runCtx.Err() == nil {
		t.Fatal("run context still live after Stop returned")
	}
	assertStoppedReads(t, owner, "after Stop")
}

// assertStoppedReads checks what Health and DataFlow report for a component whose
// release cleared running: not healthy, status "stopped", no uptime, and zero rates.
// It reports with Errorf so a failure during Stop still lets the test release Stop.
func assertStoppedReads(t *testing.T, c *Component, when string) {
	t.Helper()
	health := c.Health()
	if health.Healthy || health.Status != "stopped" || health.Uptime != 0 || health.LastError != "" {
		t.Errorf("Health %s = {Healthy: %v, Status: %q, Uptime: %v, LastError: %q}, want {false, \"stopped\", 0, \"\"}",
			when, health.Healthy, health.Status, health.Uptime, health.LastError)
	}
	flow := c.DataFlow()
	if flow.MessagesPerSecond != 0 || flow.BytesPerSecond != 0 || flow.ErrorRate != 0 {
		t.Errorf("DataFlow %s = {MessagesPerSecond: %v, BytesPerSecond: %v, ErrorRate: %v}, want all zero",
			when, flow.MessagesPerSecond, flow.BytesPerSecond, flow.ErrorRate)
	}
}
