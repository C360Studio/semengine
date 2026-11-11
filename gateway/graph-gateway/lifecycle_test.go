package graphgateway

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/pkg/errs"
)

var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// suiteOwner is graph-gateway as the lifecycle suite's owner: start is how it starts, the
// component's own Start for a good owner.
type suiteOwner struct {
	c     *Component
	start func(context.Context) error
}

func (o suiteOwner) Start(ctx context.Context) error { return o.start(ctx) }

func (o suiteOwner) Stop(ctx context.Context) error { return o.c.Stop(ctx) }

// Observe reports what the gateway retains: the HTTP requests it admitted and has not returned
// from. It holds no NATS state and runs no goroutine of its own (design D5).
// TestStopWaitsForInFlightRequest shows a started gateway reporting one.
func (o suiteOwner) Observe() lifecycletest.Observation {
	var held []string
	if o.c.requests.Pending() > 0 {
		held = append(held, "admitted request")
	}
	return lifecycletest.Observation{Unresolved: held}
}

func newInitialized(t *testing.T) *Component {
	t.Helper()
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphGateway: %v", err)
	}
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

// TestGraphGatewayLifecycleSuite runs the lifecycle suite on the production Start. The failing owner
// runs the shell with a start function that fails, so its rollback runs the production cleanup.
func TestGraphGatewayLifecycleSuite(t *testing.T) {
	factory := func() lifecycletest.Owner {
		c := newInitialized(t)
		return suiteOwner{c, c.Start}
	}
	mustFail := func() lifecycletest.Owner {
		c := newInitialized(t)
		return suiteOwner{c, func(ctx context.Context) error {
			return c.lifecycle.Start(ctx, func(context.Context) error { return errors.New("start fails") }, c.cleanup)
		}}
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})
}

// TestLifecycleBeforeStart: before Start the gateway is stopped and unhealthy with nothing counted,
// Start before Initialize and Start without a NATS client are refused, and Stop before Start returns
// nil.
func TestLifecycleBeforeStart(t *testing.T) {
	c := newInitialized(t)
	if health := c.Health(); health.Healthy || health.Status != "stopped" || health.ErrorCount != 0 {
		t.Errorf("Health before Start = %+v, want stopped, not healthy, no errors", health)
	}
	if flow := c.DataFlow(); flow != (component.FlowMetrics{}) {
		t.Errorf("DataFlow before Start = %+v, want zero", flow)
	}
	uninitialized, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphGateway: %v", err)
	}
	if err := uninitialized.Start(t.Context()); err == nil {
		t.Error("Start before Initialize returned nil")
	}
	bare := &Component{}
	if err := bare.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := bare.Start(t.Context()); !errors.Is(err, errs.ErrNoConnection) {
		t.Errorf("Start without a client = %v, want ErrNoConnection", err)
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Errorf("Stop before Start: %v", err)
	}
}
