package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/pkg/errs"
)

// stopBound is the fresh finite context a test's Stop or Shutdown gets.
const stopBound = 10 * time.Second

// stopContext returns a fresh context bounded by stopBound that does not end with the test's
// context, so it serves a Stop in the test body and in a cleanup alike.
func stopContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
	t.Cleanup(cancel)
	return ctx
}

// errCheckFailed is the error a failing countingCheck returns.
var errCheckFailed = errors.New("health check failed")

// countingCheck returns a health check that counts its calls and fails while failing is set.
func countingCheck(calls *atomic.Int64, failing *atomic.Bool) HealthCheckFunc {
	return func() error {
		calls.Add(1)
		if failing.Load() {
			return errCheckFailed
		}
		return nil
	}
}

func TestBaseServiceStartRejectsCanceledContext(t *testing.T) {
	s := NewBaseServiceWithOptions("canceled-start")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := s.Start(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, StatusStopped, s.Status())
	require.NoError(t, s.Stop(stopContext(t)))
}

func TestBaseServiceCompletedStopRejectsSameInstanceRestart(t *testing.T) {
	s := NewBaseServiceWithOptions("terminal")
	require.NoError(t, s.Start(t.Context()))
	require.NoError(t, s.Stop(stopContext(t)))
	require.NoError(t, s.Stop(stopContext(t)))
	require.ErrorIs(t, s.Start(t.Context()), errs.ErrAlreadyStarted)
}

func TestBaseServiceStopBeforeStartRejectsSameInstanceStart(t *testing.T) {
	s := NewBaseServiceWithOptions("stopped-before-start")
	require.NoError(t, s.Stop(stopContext(t)))
	require.NoError(t, s.Stop(stopContext(t)))
	require.ErrorIs(t, s.Start(t.Context()), errs.ErrAlreadyStarted)
}

// TestStartContextEndingKeepsHealthChecksRunning replaces the pin's
// TestBaseServiceParentCancellationConvergesAndJoins and TestService_ContextCancellation, which
// asserted the pin's abort (design D2, ADR-113): ending Start's context after Start returned stops
// nothing.
//
// spec: service-lifecycle / BaseService's Start bounds Start and Stop is the only stop
func TestStartContextEndingKeepsHealthChecksRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int64
		var failing atomic.Bool
		s := NewBaseServiceWithOptions("start-context-ends")
		s.SetHealthCheck(countingCheck(&calls, &failing))

		startCtx, cancelStart := context.WithCancel(t.Context())
		require.NoError(t, s.Start(startCtx))
		synctest.Wait()
		require.Equal(t, int64(1), calls.Load(), "the health check runs once at Start")

		cancelStart()
		<-time.After(60 * time.Second)
		synctest.Wait()

		require.Equal(t, StatusRunning, s.Status(), "ending the Start context stopped the service")
		require.Equal(t, int64(3), calls.Load(), "two more checks run in the 60 s after the Start context ended")
		require.True(t, s.IsHealthy())

		require.NoError(t, s.Stop(stopContext(t)))
		require.Equal(t, StatusStopped, s.Status())
		require.False(t, s.IsHealthy())
	})
}

// TestStopWithAnEndedContextStopsNothing: a Stop given a context that has already ended returns
// that context's error and stops nothing (design D2; the guard's reading).
//
// spec: service-lifecycle / BaseService's Start bounds Start and Stop is the only stop
func TestStopWithAnEndedContextStopsNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int64
		var failing atomic.Bool
		s := NewBaseServiceWithOptions("ended-stop")
		s.SetHealthCheck(countingCheck(&calls, &failing))
		require.NoError(t, s.Start(t.Context()))
		synctest.Wait()
		require.Equal(t, int64(1), calls.Load())

		ended, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, s.Stop(ended), context.Canceled)
		require.Equal(t, StatusRunning, s.Status(), "a Stop with an ended context began the stop")

		<-time.After(30 * time.Second)
		synctest.Wait()
		require.Equal(t, int64(2), calls.Load(), "health checks stopped after a Stop with an ended context")
		require.Equal(t, StatusRunning, s.Status())

		require.NoError(t, s.Stop(stopContext(t)))
		require.Equal(t, StatusStopped, s.Status())
	})
}

// TestRepeatedStopWaitsForABlockedHealthCheck replaces the pin's
// TestBaseServiceStopIsOneShotAfterFailedJoin, which asserted that a later Stop returns nil at once
// without waiting (design D2). Here a Stop whose context ends while it waits returns that error,
// and a later Stop waits again for the blocked check.
//
// spec: service-lifecycle / BaseService's Start bounds Start and Stop is the only stop
func TestRepeatedStopWaitsForABlockedHealthCheck(t *testing.T) {
	for _, tt := range []struct {
		name    string
		stopCtx func(t *testing.T) (context.Context, context.CancelFunc)
		// cancel says the test cancels the context while Stop waits; otherwise its deadline
		// passes once every goroutine in the bubble blocks.
		cancel  bool
		wantErr error
	}{
		{
			name: "canceled",
			stopCtx: func(t *testing.T) (context.Context, context.CancelFunc) {
				return context.WithCancel(t.Context())
			},
			cancel:  true,
			wantErr: context.Canceled,
		},
		{
			name: "deadline",
			stopCtx: func(t *testing.T) (context.Context, context.CancelFunc) {
				return context.WithTimeout(t.Context(), time.Second)
			},
			wantErr: context.DeadlineExceeded,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var calls atomic.Int64
				entered := make(chan struct{})
				release := make(chan struct{})
				s := NewBaseServiceWithOptions("blocked-check")
				s.SetHealthCheck(func() error {
					if calls.Add(1) == 1 {
						close(entered)
						<-release
					}
					return nil
				})
				require.NoError(t, s.Start(t.Context()))
				<-entered

				stopCtx, cancelStop := tt.stopCtx(t)
				defer cancelStop()
				firstStop := make(chan error, 1)
				go func() { firstStop <- s.Stop(stopCtx) }()
				synctest.Wait()
				require.Equal(t, StatusStopping, s.Status(), "a Stop with a live context begins the stop")
				require.Empty(t, firstStop, "Stop returned while the health check still blocks")

				if tt.cancel {
					cancelStop()
				}
				require.ErrorIs(t, <-firstStop, tt.wantErr)
				require.Equal(t, StatusStopping, s.Status())

				secondStop := make(chan error, 1)
				go func() { secondStop <- s.Stop(stopContext(t)) }()
				synctest.Wait()
				require.Empty(t, secondStop, "a later Stop returned without waiting for the blocked check")

				close(release)
				require.NoError(t, <-secondStop)
				require.Equal(t, StatusStopped, s.Status())
				require.False(t, s.IsHealthy())

				<-time.After(60 * time.Second)
				synctest.Wait()
				require.Equal(t, int64(1), calls.Load(), "a health check ran after the stop began")
			})
		})
	}
}

// baseServiceOwner adapts BaseService to the lifecycle suite (lifecycle-suite, "Observe adapter
// contract"; design D2). A started BaseService holds one thing, its health goroutine, which Stop
// waits on through the healthDone channel; Observe lists "health-loop" while that channel is open.
// Nothing is added to BaseService for this adapter (#186, #190).
//
// Calls is empty: BaseService makes no external call, and the suite's no-op checks compare Calls
// before and after a call, so a count of the adapter's own Start and Stop calls would fail them by
// construction (config design D2's reason).
type baseServiceOwner struct {
	s *BaseService
}

func (o *baseServiceOwner) Start(ctx context.Context) error { return o.s.Start(ctx) }
func (o *baseServiceOwner) Stop(ctx context.Context) error  { return o.s.Stop(ctx) }

func (o *baseServiceOwner) Observe() lifecycletest.Observation {
	o.s.mu.RLock()
	done := o.s.healthDone
	o.s.mu.RUnlock()
	var held []string
	if done != nil && !isClosed(done) {
		held = append(held, "health-loop")
	}
	return lifecycletest.Observation{Unresolved: held}
}

// isClosed reports whether done has been closed, without waiting.
func isClosed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// TestBaseServicePassesTheLifecycleSuite runs the lifecycle suite on BaseService (ADR-113; design
// D2). The must-fail owner is a BaseService stopped before Start, whose Start is refused. The
// service is one-shot: no restart is promised.
//
// spec: service-lifecycle / BaseService's Start bounds Start and Stop is the only stop
func TestBaseServicePassesTheLifecycleSuite(t *testing.T) {
	factory := func() lifecycletest.Owner {
		return &baseServiceOwner{s: NewBaseServiceWithOptions("suite")}
	}
	mustFail := func() lifecycletest.Owner {
		s := NewBaseServiceWithOptions("suite-must-fail")
		require.NoError(t, s.Stop(stopContext(t)))
		return &baseServiceOwner{s: s}
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles". The suite judges only what is left
	// after a Stop, so an adapter that never reported the health loop would pass it.
	t.Run("StartedOwnerReportsTheHealthLoop", func(t *testing.T) {
		owner := &baseServiceOwner{s: NewBaseServiceWithOptions("suite-started")}
		require.NoError(t, owner.Start(t.Context()))
		require.Equal(t, []string{"health-loop"}, owner.Observe().Unresolved)
		require.NoError(t, owner.Stop(stopContext(t)))
		require.Empty(t, owner.Observe().Unresolved, "a Stop that returned nil left the health loop running")
	})
}
