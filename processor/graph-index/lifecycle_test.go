package graphindex

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/dispatch"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

var _ component.LifecycleComponent = (*Component)(nil)

// TestLifecycleBeforeStart covers the lifecycle checks that need no server: before Start the
// component is stopped and unhealthy with nothing counted, Initialize may be called again, Start
// before Initialize and Start without a NATS client are refused, and Stop before Start returns
// nil.
func TestLifecycleBeforeStart(t *testing.T) {
	newComponent := func(t *testing.T) *Component {
		t.Helper()
		client, err := natsclient.NewClient("")
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		built, err := CreateGraphIndex(json.RawMessage(`{}`), component.Dependencies{NATSClient: client})
		if err != nil {
			t.Fatalf("CreateGraphIndex: %v", err)
		}
		return built.(*Component)
	}

	t.Run("health and data flow before Start", func(t *testing.T) {
		c := newComponent(t)
		health := c.Health()
		if health.Healthy || health.Status != "stopped" || health.ErrorCount != 0 {
			t.Errorf("Health before Start = %+v, want stopped, not healthy, no errors", health)
		}
		// LastActivity is the zero time before the first input (component.Lifecycle.DataFlow).
		if flow := c.DataFlow(); flow != (component.FlowMetrics{}) {
			t.Errorf("DataFlow before Start = %+v, want zero", flow)
		}
	})

	t.Run("Initialize twice", func(t *testing.T) {
		c := newComponent(t)
		if err := c.Initialize(); err != nil {
			t.Fatalf("first Initialize: %v", err)
		}
		if err := c.Initialize(); err != nil {
			t.Errorf("second Initialize: %v", err)
		}
	})

	t.Run("Start before Initialize", func(t *testing.T) {
		if err := newComponent(t).Start(t.Context()); err == nil {
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
		if err := newComponent(t).Stop(t.Context()); err != nil {
			t.Errorf("Stop before Start: %v", err)
		}
	})
}

// TestStopWithEndedContextInitiatesPoolShutdownAndRetries checks that Stop with an already-expired
// context cancels runCtx and initiates pool.Shutdown even when feeds have not exited yet, keeping
// poolCtx live so accepted items finish draining on the retry Stop.
func TestStopWithEndedContextInitiatesPoolShutdownAndRetries(t *testing.T) {
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	built, err := CreateGraphIndex(json.RawMessage(`{}`), component.Dependencies{NATSClient: client})
	if err != nil {
		t.Fatalf("CreateGraphIndex: %v", err)
	}
	c := built.(*Component)
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	releaseFeeds := make(chan struct{})
	var runCtx, poolCtx context.Context
	if err := c.lifecycle.Start(t.Context(), func(ctx context.Context) error {
		var poolCancel, runCancel context.CancelFunc
		poolCtx, poolCancel = context.WithCancel(context.WithoutCancel(ctx))
		runCtx, runCancel = context.WithCancel(poolCtx)
		c.poolCancel, c.runCancel = poolCancel, runCancel

		pool, err := dispatch.NewKeyedPool(poolCtx, dispatch.KeyedConfig[indexWork]{
			Lanes:      1,
			QueueDepth: 8,
			Name:       "graph_index_test",
			Process:    func(context.Context, int, indexWork) {},
		})
		if err != nil {
			return err
		}
		c.pool = pool

		feeds := make(chan struct{})
		c.feeds = feeds
		go func() {
			<-runCtx.Done()
			<-releaseFeeds
			close(feeds)
		}()
		return nil
	}, c.cleanup); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		select {
		case <-releaseFeeds:
		default:
			close(releaseFeeds)
		}
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.Stop(stopCtx)
	})

	expired, cancelExpired := context.WithDeadline(t.Context(), time.Unix(1, 0))
	defer cancelExpired()
	if err := c.Stop(expired); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop(expired) = %v, want context.DeadlineExceeded", err)
	}
	if runCtx.Err() == nil {
		t.Fatal("Stop(expired) must cancel runCtx")
	}
	if err := c.pool.SubmitBlocking(t.Context(), "probe", indexWork{}); !errors.Is(err, dispatch.ErrStopped) {
		t.Fatalf("SubmitBlocking after Stop(expired) = %v, want dispatch.ErrStopped (pool.Shutdown initiated)", err)
	}
	if poolCtx.Err() != nil {
		t.Fatal("Stop(expired) must keep poolCtx live until accepted work drains")
	}

	close(releaseFeeds)
	if err := c.Stop(t.Context()); err != nil {
		t.Fatalf("retry Stop: %v", err)
	}
	if poolCtx.Err() == nil {
		t.Fatal("retry Stop must cancel poolCtx once pool drained")
	}
	if c.pool != nil || c.feeds != nil || c.runCancel != nil || c.poolCancel != nil {
		t.Fatal("retry Stop must clear lifecycle handles")
	}
}
