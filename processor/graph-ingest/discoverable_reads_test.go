package graphingest

import (
	"context"
	"testing"
	"time"
)

// TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop proves graph-ingest's side of
// component.LifecycleComponent's read line (design D2, "No wait for readers"): Health and
// DataFlow return while the component's own Stop is still running, and again after it
// returned, each reporting a stopped component. Stop is held open by the consumer fixture
// of TestLifecycleOwnerRunningStopPreservesEffectSettlementOrder: the shell clears running
// as release begins, the cleanup drains the consumer, and then waits for its Closed, which
// the test holds. On today's tree the test passes; it is shown able to fail by mutation
// row M7 of design D10 (the shell holding its lock across start and cleanup), under which
// Health does not return within readBound.
func TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop(t *testing.T) {
	consumer := &graphIngestLifecycleConsumeContext{
		closed: make(chan struct{}), drainSeen: make(chan struct{}),
	}
	runCtx, runCancel := context.WithCancel(t.Context())
	owner := createTestComponent(t)
	// Sixty messages: while running, DataFlow reports a rate above zero, so the zero rate
	// after release began is a change.
	for range 60 {
		owner.lifecycle.CountInput(0)
	}
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

// readBound is how long a read may take before the test reports it as waiting. Health
// and DataFlow wait on nothing but c.mu, which a read holds for microseconds once it has
// it; ten seconds is far above that on a loaded runner and far below go test's timeout,
// so a read held behind Stop fails an assertion instead of hanging the run.
const readBound = 10 * time.Second

// readWithin runs read on its own goroutine and returns its answer, or reports with Errorf,
// naming description, when it has not returned within readBound. The answer comes over a
// channel, so a read that returns after the bound races with nothing.
func readWithin[T any](t *testing.T, description string, read func() T) (T, bool) {
	t.Helper()
	answer := make(chan T, 1)
	go func() { answer <- read() }()
	timer := time.NewTimer(readBound)
	defer timer.Stop()
	select {
	case value := <-answer:
		return value, true
	case <-timer.C:
		t.Errorf("%s did not return within %v", description, readBound)
		var zero T
		return zero, false
	}
}

// assertStoppedReads checks what Health and DataFlow report for a component whose
// release cleared running: not healthy, status "stopped", no uptime, and zero rates. Each
// read must return within readBound. It reports with Errorf so a failure during Stop
// still lets the test release Stop.
func assertStoppedReads(t *testing.T, c *Component, when string) {
	t.Helper()
	if health, ok := readWithin(t, "Health "+when, c.Health); ok &&
		(health.Healthy || health.Status != "stopped" || health.Uptime != 0 || health.LastError != "") {
		t.Errorf("Health %s = {Healthy: %v, Status: %q, Uptime: %v, LastError: %q}, want {false, \"stopped\", 0, \"\"}",
			when, health.Healthy, health.Status, health.Uptime, health.LastError)
	}
	if flow, ok := readWithin(t, "DataFlow "+when, c.DataFlow); ok &&
		(flow.MessagesPerSecond != 0 || flow.BytesPerSecond != 0 || flow.ErrorRate != 0) {
		t.Errorf("DataFlow %s = {MessagesPerSecond: %v, BytesPerSecond: %v, ErrorRate: %v}, want all zero",
			when, flow.MessagesPerSecond, flow.BytesPerSecond, flow.ErrorRate)
	}
}
