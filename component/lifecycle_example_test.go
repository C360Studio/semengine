package component_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semengine/component"
)

// sensor is a component as an outside author writes one: it holds a component.Lifecycle and
// gives it the two functions only the sensor knows, start and cleanup, and its stuck state at
// each Health read.
type sensor struct {
	lifecycle component.Lifecycle
	// calibrationLost is the sensor's one stuck state.
	calibrationLost atomic.Bool

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

var _ component.LifecycleComponent = (*sensor)(nil)

func (s *sensor) Initialize() error               { return s.lifecycle.Initialize() }
func (s *sensor) Start(ctx context.Context) error { return s.lifecycle.Start(ctx, s.start, s.cleanup) }
func (s *sensor) Stop(ctx context.Context) error  { return s.lifecycle.Stop(ctx) }
func (s *sensor) Health() component.HealthStatus  { return s.lifecycle.Health(s.stuck()) }
func (s *sensor) DataFlow() component.FlowMetrics { return s.lifecycle.DataFlow() }

func (s *sensor) InputPorts() []component.Port  { return nil }
func (s *sensor) OutputPorts() []component.Port { return nil }

// start creates the run context for the sensor's continuing work (ADR-113): ending Start's
// context after Start returned stops nothing.
func (s *sensor) start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-runCtx.Done() // the sensor's polling loop would run here
	}()
	s.mu.Lock()
	s.cancel, s.done = cancel, done
	s.mu.Unlock()
	return nil
}

// cleanup ends the run context and joins the worker within Stop's bound.
func (s *sensor) cleanup(ctx context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// receive is what the sensor's subscription calls for each reading.
func (s *sensor) receive(payload []byte) {
	s.lifecycle.CountInput(len(payload))
}

func (s *sensor) stuck() string {
	if s.calibrationLost.Load() {
		return "calibration lost"
	}
	return ""
}

func ExampleLifecycle() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s := &sensor{}
	if err := s.Initialize(); err != nil {
		fmt.Println("initialize:", err)
		return
	}
	if err := s.Start(ctx); err != nil {
		fmt.Println("start:", err)
		return
	}
	s.receive([]byte("21.5"))
	s.receive([]byte("21.7"))
	fmt.Println(s.Health().Status, s.DataFlow().Messages, s.DataFlow().Bytes)

	s.calibrationLost.Store(true)
	h := s.Health()
	fmt.Println(h.Status, h.LastError)

	if err := s.Stop(ctx); err != nil {
		fmt.Println("stop:", err)
		return
	}
	fmt.Println(s.Health().Status, s.DataFlow().Messages)
	// Output:
	// running 2 8
	// degraded calibration lost
	// stopped 2
}
