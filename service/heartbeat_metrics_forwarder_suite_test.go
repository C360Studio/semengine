package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/pkg/errs"
)

// suiteHeartbeatMetricsForwarderConfig is the default configuration for both services: the suite
// judges the lifecycle, which no setting changes. Both are built over discardLogger
// (graph_triples_http_test.go): the suite reads no record.
const suiteHeartbeatMetricsForwarderConfig = `{}`

// heartbeatOwner adapts the heartbeat service to the lifecycle suite (lifecycle-suite, "Observe
// adapter contract"; design D2, D6 U-a). A started heartbeat holds its base service and its one
// loop. Observe lists "base-status=<status>" while the base service is neither stopped nor
// starting, and "loop" while the loop goroutine has not returned, read from hb.loopDone, the
// channel Stop's join waits on. It reads the base service's status through its accessor and adds
// nothing to the service (#186, #190).
type heartbeatOwner struct {
	hb *HeartbeatService
}

func (o *heartbeatOwner) Start(ctx context.Context) error { return o.hb.Start(ctx) }
func (o *heartbeatOwner) Stop(ctx context.Context) error  { return o.hb.Stop(ctx) }

func (o *heartbeatOwner) Observe() lifecycletest.Observation {
	return observeOneLoop(o.hb.Status(), o.hb.loopDone)
}

// metricsForwarderOwner adapts the metrics forwarder to the lifecycle suite, as heartbeatOwner
// does the heartbeat, reading mf.loopDone.
type metricsForwarderOwner struct {
	mf *MetricsForwarder
}

func (o *metricsForwarderOwner) Start(ctx context.Context) error { return o.mf.Start(ctx) }
func (o *metricsForwarderOwner) Stop(ctx context.Context) error  { return o.mf.Stop(ctx) }

func (o *metricsForwarderOwner) Observe() lifecycletest.Observation {
	return observeOneLoop(o.mf.Status(), o.mf.loopDone)
}

// observeOneLoop is the observation both adapters make of a service with one base service and one
// loop: the base status while neither stopped nor starting, and "loop" while loopDone is set and
// open.
func observeOneLoop(status Status, loopDone chan struct{}) lifecycletest.Observation {
	var held []string
	if status != StatusStopped && status != StatusStarting {
		held = append(held, "base-status="+status.String())
	}
	if loopDone != nil && !isClosed(loopDone) {
		held = append(held, "loop")
	}
	return lifecycletest.Observation{Unresolved: held}
}

// TestHeartbeatPassesTheLifecycleSuite runs the lifecycle suite on the heartbeat service (ADR-113;
// design D2; the pin stopped its loop when Start's context ended, probe sweep.md § 3, H6). Every
// heartbeat is built through newTestHeartbeat, a service manager holding a component manager, so
// Start finds the component manager it needs. The must-fail owner is a heartbeat stopped before
// Start, whose Start is refused (base_lifecycle_test.go's precedent). The service is one-shot: no
// restart is promised.
//
// spec: service-lifecycle / The heartbeat service's Start bounds Start and Stop is the only stop
func TestHeartbeatPassesTheLifecycleSuite(t *testing.T) {
	newOwner := func() *heartbeatOwner {
		return &heartbeatOwner{hb: newTestHeartbeat(t, suiteHeartbeatMetricsForwarderConfig, discardLogger)}
	}
	runOneLoopSuite(t, func() lifecycletest.Owner { return newOwner() })
}

// TestMetricsForwarderPassesTheLifecycleSuite runs the lifecycle suite on the metrics forwarder
// (ADR-113; design D2; the pin stopped its loop when Start's context ended, probe sweep.md § 3,
// H6). Every forwarder is built through NewMetricsForwarderService over a client that never
// connects: Start does no I/O, and a failed publish is the loop's to log (design D2, "Startup
// work"). The must-fail owner is a forwarder stopped before Start. The service is one-shot.
//
// spec: service-lifecycle / The metrics forwarder's Start bounds Start and Stop is the only stop
func TestMetricsForwarderPassesTheLifecycleSuite(t *testing.T) {
	newOwner := func() *metricsForwarderOwner {
		mf, _ := newTestMetricsForwarder(t, newUnconnectedClient(t), suiteHeartbeatMetricsForwarderConfig,
			discardLogger)
		return &metricsForwarderOwner{mf: mf}
	}
	runOneLoopSuite(t, func() lifecycletest.Owner { return newOwner() })
}

// runOneLoopSuite runs the lifecycle suite, and the two checks it does not make, on owners newOwner
// builds, each a service with one base service and one loop.
func runOneLoopSuite(t *testing.T, newOwner lifecycletest.Factory) {
	t.Helper()
	mustFail := func() lifecycletest.Owner {
		owner := newOwner()
		require.NoError(t, owner.Stop(stopContext(t)))
		return owner
	}
	lifecycletest.Run(t, newOwner, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles": the suite judges only what is left
	// after a Stop, so an adapter that never reported the loop would pass it.
	t.Run("StartedOwnerReportsItsHandles", func(t *testing.T) {
		owner := newOwner()
		require.NoError(t, owner.Start(t.Context()))
		require.Equal(t, []string{"base-status=running", "loop"}, owner.Observe().Unresolved)
		require.NoError(t, owner.Stop(stopContext(t)))
		require.Empty(t, owner.Observe().Unresolved, "a Stop that returned nil left a handle")
	})

	// design D2, "Refusals": a Start after any Stop is refused with errs.ErrAlreadyStarted and
	// holds nothing. The suite's FailedStartHoldsNothing accepts any refusal.
	t.Run("StartAfterStopIsRefused", func(t *testing.T) {
		owner := newOwner()
		require.NoError(t, owner.Stop(stopContext(t)))
		require.ErrorIs(t, owner.Start(t.Context()), errs.ErrAlreadyStarted)
		require.Empty(t, owner.Observe().Unresolved)
	})
}
