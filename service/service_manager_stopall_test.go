package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/pkg/errs"
)

// stopSpyService is a Service whose Stop returns a set error and counts its calls (pin
// service_manager_stopall_test.go:17).
type stopSpyService struct {
	MockService
	stopErr   error
	stopCalls atomic.Int64
}

func (s *stopSpyService) Stop(context.Context) error {
	s.stopCalls.Add(1)
	return s.stopErr
}

// startedManagerWith starts a manager composing the stand-in component-manager and then services,
// in that registration order. StopAll before StartAll stops no service (design D2), so the pin's
// direct writes into the service map become a StartAll.
func startedManagerWith(t *testing.T, services ...Service) *Manager {
	t.Helper()
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	for _, svc := range services {
		require.NoError(t, manager.RegisterInstance(svc.Name(), svc))
	}
	require.NoError(t, manager.StartAll(t.Context()))
	return manager
}

// TestServiceManager_StopAll_Idempotency: a service whose Stop returns errs.ErrAlreadyStopped is
// stopped, a genuine failure is returned naming its service while the others are still stopped,
// and a clean pass returns nil (pin gh#520; design D2).
func TestServiceManager_StopAll_Idempotency(t *testing.T) {
	t.Run("already-stopped service is not aggregated as fatal", func(t *testing.T) {
		already := &stopSpyService{
			MockService: MockService{name: "already", status: StatusStopped},
			stopErr:     errs.ErrAlreadyStopped,
		}
		clean := &stopSpyService{MockService: MockService{name: "clean", status: StatusRunning, healthy: true}}
		manager := startedManagerWith(t, already, clean)

		require.NoError(t, manager.StopAll(stopContext(t)), "an already-stopped service must not fail StopAll")
		require.EqualValues(t, 1, already.stopCalls.Load(), "already-stopped service is still visited")
		require.EqualValues(t, 1, clean.stopCalls.Load(), "clean service is stopped")
	})

	t.Run("genuine stop failure is surfaced and others still stopped", func(t *testing.T) {
		other := &stopSpyService{MockService: MockService{name: "other", status: StatusRunning, healthy: true}}
		failing := &stopSpyService{
			MockService: MockService{name: "failing", status: StatusRunning, healthy: true},
			stopErr:     errors.New("boom"),
		}
		// Reverse registration order stops "failing" before "other".
		manager := startedManagerWith(t, other, failing)

		err := manager.StopAll(stopContext(t))
		require.Error(t, err, "a genuine stop error must be surfaced")
		require.Contains(t, err.Error(), "failing")
		require.EqualValues(t, 1, other.stopCalls.Load(), "remaining services are stopped despite one failure")

		failing.stopErr = nil
		require.NoError(t, manager.StopAll(stopContext(t)))
	})

	t.Run("fully clean shutdown returns nil", func(t *testing.T) {
		a := &stopSpyService{MockService: MockService{name: "a", status: StatusRunning, healthy: true}}
		b := &stopSpyService{
			MockService: MockService{name: "b", status: StatusStopped},
			stopErr:     errs.ErrAlreadyStopped,
		}
		manager := startedManagerWith(t, a, b)

		require.NoError(t, manager.StopAll(stopContext(t)))
	})
}

// TestStopAllCountsAlreadyStoppedAsStopped: a started manager holding one service whose Stop
// returns errs.ErrAlreadyStopped and one whose Stop returns nil; StopAll returns nil, each Stop was
// called once, and a second StopAll calls nothing (owner Q2, the SS#1218 substitute; design D2).
//
// spec: service-lifecycle / StopAll counts a service already stopped as stopped
func TestStopAllCountsAlreadyStoppedAsStopped(t *testing.T) {
	already := &stopSpyService{
		MockService: MockService{name: "already", status: StatusStopped},
		stopErr:     errs.ErrAlreadyStopped,
	}
	clean := &stopSpyService{MockService: MockService{name: "clean", status: StatusRunning, healthy: true}}
	manager := startedManagerWith(t, already, clean)

	require.NoError(t, manager.StopAll(stopContext(t)))
	require.EqualValues(t, 1, already.stopCalls.Load())
	require.EqualValues(t, 1, clean.stopCalls.Load())

	require.NoError(t, manager.StopAll(stopContext(t)), "a StopAll after a nil StopAll returns nil")
	require.EqualValues(t, 1, already.stopCalls.Load(), "a StopAll after a nil StopAll calls nothing")
	require.EqualValues(t, 1, clean.stopCalls.Load(), "a StopAll after a nil StopAll calls nothing")
	_, found := manager.GetService("clean")
	require.False(t, found, "after a nil StopAll GetService finds nothing")
}
