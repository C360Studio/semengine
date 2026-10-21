package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/pkg/errs"
)

// errUnexpectedAcquisition is what a listen function returns when a test expects it not to be
// called.
var errUnexpectedAcquisition = errors.New("unexpected acquisition")

// refusingListen is a listen function that records that it was called and binds nothing.
func refusingListen(called *bool) func(string, string) (net.Listener, error) {
	return func(string, string) (net.Listener, error) {
		*called = true
		return nil, errUnexpectedAcquisition
	}
}

// startHealthOnBoundListener starts manager's health listener on a loopback listener the test
// bound (pin service_manager_health_listener_test.go:259). Port 0 means disabled, so no call asks
// for a kernel-picked port; the unexported startHealthListener takes the listen function instead
// (design D2, "Test-only code").
func startHealthOnBoundListener(ctx context.Context, t *testing.T, manager *Manager) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	require.NoError(t, manager.startHealthListener(ctx, 1, func(string, string) (net.Listener, error) {
		return listener, nil
	}))
	return listener
}

// TestListenerStartsRejectCanceledContextBeforeAcquisition keeps the pin test's health half: an
// ended or nil context is refused before the listen function runs, and the refused call does not
// use up the listener, so a later call with a live context binds. The main half is the lifecycle
// suite's PreCancelledStartRefused (TestManagerPassesTheLifecycleSuite).
func TestListenerStartsRejectCanceledContextBeforeAcquisition(t *testing.T) {
	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	stopManagerAtEnd(t, manager)
	called := false
	require.ErrorIs(t, manager.startHealthListener(canceled, 1, refusingListen(&called)), context.Canceled)
	require.False(t, called)
	var nilCtx context.Context
	require.Error(t, manager.startHealthListener(nilCtx, 1, refusingListen(&called)))
	require.False(t, called)

	listener := startHealthOnBoundListener(t.Context(), t, manager)
	status, _, err := httpGet(t.Context(), "http://"+listener.Addr().String()+"/healthz")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
}

// TestStartHealthListener_BindsHealthAndHealthz verifies that StartHealthListener binds /health
// and /healthz on the requested port and serves the manager's answers. The bind happens before the
// call returns (design D2), so the pin's poll for the listener is not carried.
//
// spec: operator-surface / The dedicated health listener
func TestStartHealthListener_BindsHealthAndHealthz(t *testing.T) {
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	listener := startHealthOnBoundListener(t.Context(), t, manager)
	stopManagerAtEnd(t, manager)
	addr := "http://" + listener.Addr().String()

	// /healthz is the liveness probe: 200 whenever the listener serves.
	status, body, err := httpGet(t.Context(), addr+"/healthz")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "OK", body)

	// /health aggregates the services and, with a NATS client, its status. With no service and
	// no client the aggregate is healthy.
	status, _, err = httpGet(t.Context(), addr+"/health")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
}

// TestStartHealthListener_ZeroIsNoOp verifies the disabled path: port 0 binds nothing and returns
// nil, and StopAll then has nothing to stop.
func TestStartHealthListener_ZeroIsNoOp(t *testing.T) {
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())

	called := false
	require.NoError(t, manager.startHealthListener(t.Context(), 0, refusingListen(&called)))
	require.False(t, called, "port 0 acquired a listener")
	require.NoError(t, manager.StartHealthListener(t.Context(), 0))
	require.NoError(t, manager.StopAll(stopContext(t)))
}

// TestStartHealthListenerReportsBindFailureSynchronously: StartHealthListener on the port of a
// listener the test holds returns the bind error.
func TestStartHealthListenerReportsBindFailureSynchronously(t *testing.T) {
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	err := manager.StartHealthListener(t.Context(), heldPort(t))
	require.Error(t, err)
	require.Contains(t, err.Error(), "bind")
	require.NoError(t, manager.StopAll(stopContext(t)))
}

// TestHealthListenerCannotRebindAfterCompletedStop: after StopAll stopped the health listener, a
// second start is refused before its listen function runs (design D2; the pin stopped it with the
// dropped StopHealthListener).
//
// spec: operator-surface / The dedicated health listener
func TestHealthListenerCannotRebindAfterCompletedStop(t *testing.T) {
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	startHealthOnBoundListener(t.Context(), t, manager)
	require.NoError(t, manager.StopAll(stopContext(t)))

	called := false
	require.Error(t, manager.startHealthListener(t.Context(), 1, refusingListen(&called)))
	require.False(t, called)
	require.Error(t, manager.StartHealthListener(t.Context(), heldPort(t)))
}

// TestStartHealthListener_DoubleStartErrors verifies the one-shot guard: a second call with a
// non-zero port is refused rather than binding again.
func TestStartHealthListener_DoubleStartErrors(t *testing.T) {
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	startHealthOnBoundListener(t.Context(), t, manager)
	stopManagerAtEnd(t, manager)

	called := false
	require.Error(t, manager.startHealthListener(t.Context(), 1, refusingListen(&called)))
	require.False(t, called)
}

// TestStopAll_TearsDownHealthListener: StopAll stops the dedicated health listener even when
// StartAll never ran, and a later StartAll is refused (design D2). StopAll returns after the
// listener closed, so the pin's poll for a refused request becomes one refused dial.
func TestStopAll_TearsDownHealthListener(t *testing.T) {
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	listener := startHealthOnBoundListener(t.Context(), t, manager)
	addr := listener.Addr().String()
	status, _, err := httpGet(t.Context(), "http://"+addr+"/healthz")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)

	require.NoError(t, manager.StopAll(stopContext(t)))
	requireListenerReleased(t, addr)
	require.ErrorIs(t, manager.StartAll(t.Context()), errs.ErrAlreadyStarted)
}

// TestRequestContextOutlivesStartContext: ending StartAll's context after it returned ends no
// request's context. A request to the main listener made afterwards is answered, and the handler
// sees a live request context (design D2, S2; replaces the main half of the pin's
// TestListenerBaseContextsPreserveExactStartValues, which pinned the opposite).
//
// spec: service-lifecycle / The service manager's StartAll bounds StartAll
func TestRequestContextOutlivesStartContext(t *testing.T) {
	seen := make(chan error, 1)
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	manager.UseHTTPMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				select {
				case seen <- r.Context().Err():
				default:
				}
			}
			next.ServeHTTP(w, r)
		})
	})

	startCtx, cancelStart := context.WithCancel(t.Context())
	require.NoError(t, manager.StartAll(startCtx))
	stopManagerAtEnd(t, manager)
	cancelStart()

	status, body, err := httpGet(t.Context(), observedManagerHTTPURL(t, manager)+"/healthz")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "OK", body)
	require.NoError(t, <-seen, "the request context ended with the StartAll context")
}

// TestHealthListenerRequestContextOutlivesStartContext: ending the context given to
// StartHealthListener after it returned stops nothing: /healthz answers 200 with OK, and the
// context the server gives a new connection has not ended. The health routes take no middleware,
// so the test reads that context where the server makes it, as the pin's
// TestListenerBaseContextsPreserveExactStartValues did (design D2, S2).
//
// spec: operator-surface / The dedicated health listener
func TestHealthListenerRequestContextOutlivesStartContext(t *testing.T) {
	manager := createTestServiceManager(ManagerConfig{}, createTestServiceDependencies())
	startCtx, cancelStart := context.WithCancel(t.Context())
	listener := startHealthOnBoundListener(startCtx, t, manager)
	stopManagerAtEnd(t, manager)
	cancelStart()

	status, body, err := httpGet(t.Context(), "http://"+listener.Addr().String()+"/healthz")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "OK", body)

	manager.mu.RLock()
	server := manager.healthServer
	manager.mu.RUnlock()
	require.NotNil(t, server, "the health listener has no server")
	base := context.Background()
	if server.BaseContext != nil {
		base = server.BaseContext(listener)
	}
	require.NoError(t, base.Err(), "the health server's connection context ended with the start context")
}
