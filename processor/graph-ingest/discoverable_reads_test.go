//go:build integration

package graphingest

import (
	"context"
	"testing"
	"time"
)

// TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop proves graph-ingest's side of
// component.LifecycleComponent's read line (design D2, "No wait for readers"): Health and
// DataFlow return while the component's own Stop is still running, and again after it
// returned, each reporting a stopped component. Stop is held open by a consume callback
// held on the poison warning (bindHeldInput, lifecycle_owner_test.go): the shell clears
// running as release begins, and the cleanup then waits for that callback, which the test
// holds. On today's tree the test passes; it is shown able to fail by mutation row M7 of
// design D10 (the shell holding its lock across start and cleanup), under which Health
// does not return while Stop is held.
func TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	owner, pause := newFixtureInputComponent(t)
	runCtx, runCancel := context.WithCancel(t.Context())
	// Sixty messages: while running, DataFlow reports a rate above zero, so the zero rate
	// after release began is a change.
	for range 60 {
		owner.lifecycle.CountInput(0)
	}
	owner.cancel = runCancel
	bindHeldInput(ctx, ctx, t, owner, pause)
	startGuardAcquiringNothing(t, owner)

	if health := owner.Health(); !health.Healthy || health.Status != "running" {
		t.Fatalf("Health before Stop = {Healthy: %v, Status: %q}, want {true, \"running\"}",
			health.Healthy, health.Status)
	}
	if flow := owner.DataFlow(); flow.MessagesPerSecond <= 0 {
		t.Fatalf("DataFlow before Stop: MessagesPerSecond = %v, want above zero", flow.MessagesPerSecond)
	}

	stopResult := make(chan error, 1)
	go func() { stopResult <- owner.Stop(ctx) }()
	awaitReleaseBegun(ctx, t, owner) // release has cleared running; cleanup waits for the held callback

	assertStoppedReads(t, owner, "during Stop")
	select {
	case err := <-stopResult:
		t.Fatalf("Stop returned %v while a consume callback is held; the reads did not run during Stop", err)
	default:
	}

	pause.Release()
	if err := <-stopResult; err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if runCtx.Err() == nil {
		t.Fatal("run context still live after Stop returned")
	}
	assertStoppedReads(t, owner, "after Stop")
}

// assertStoppedReads checks what Health and DataFlow report for a component whose
// release cleared running: not healthy, status "stopped", no uptime, and zero rates. It
// reports with Errorf so a failure during Stop still lets the test release Stop.
func assertStoppedReads(t *testing.T, c *Component, when string) {
	t.Helper()
	if health := c.Health(); health.Healthy || health.Status != "stopped" || health.Uptime != 0 || health.LastError != "" {
		t.Errorf("Health %s = {Healthy: %v, Status: %q, Uptime: %v, LastError: %q}, want {false, \"stopped\", 0, \"\"}",
			when, health.Healthy, health.Status, health.Uptime, health.LastError)
	}
	if flow := c.DataFlow(); flow.MessagesPerSecond != 0 || flow.BytesPerSecond != 0 || flow.ErrorRate != 0 {
		t.Errorf("DataFlow %s = {MessagesPerSecond: %v, BytesPerSecond: %v, ErrorRate: %v}, want all zero",
			when, flow.MessagesPerSecond, flow.BytesPerSecond, flow.ErrorRate)
	}
}
