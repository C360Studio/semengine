package graphclustering

import (
	"errors"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/pkg/errs"
)

var _ component.LifecycleComponent = (*Component)(nil)

// TestLifecycleBeforeStart: before Start the component is stopped and unhealthy with nothing
// counted, Initialize may be called again, Start before Initialize and Start without a NATS client
// are refused, and Stop before Start returns nil.
func TestLifecycleBeforeStart(t *testing.T) {
	t.Run("health and data flow before Start", func(t *testing.T) {
		c := mustCreate(t, `{}`)
		health := c.Health()
		if health.Healthy || health.Status != "stopped" || health.ErrorCount != 0 {
			t.Errorf("Health before Start = %+v, want stopped, not healthy, no errors", health)
		}
		if flow := c.DataFlow(); flow != (component.FlowMetrics{}) {
			t.Errorf("DataFlow before Start = %+v, want zero", flow)
		}
	})

	t.Run("Initialize twice", func(t *testing.T) {
		c := mustCreate(t, `{}`)
		if err := c.Initialize(); err != nil {
			t.Fatalf("first Initialize: %v", err)
		}
		if err := c.Initialize(); err != nil {
			t.Errorf("second Initialize: %v", err)
		}
	})

	t.Run("Start before Initialize", func(t *testing.T) {
		if err := mustCreate(t, `{}`).Start(t.Context()); err == nil {
			t.Error("Start before Initialize returned nil")
		}
	})

	t.Run("Start without a NATS client", func(t *testing.T) {
		c := &Component{}
		if err := c.Initialize(); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		if err := c.Start(t.Context()); !errors.Is(err, errs.ErrNoConnection) {
			t.Errorf("Start without a client = %v, want ErrNoConnection", err)
		}
	})

	t.Run("Stop before Start", func(t *testing.T) {
		if err := mustCreate(t, `{}`).Stop(t.Context()); err != nil {
			t.Errorf("Stop before Start: %v", err)
		}
	})
}
