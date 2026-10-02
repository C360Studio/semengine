package metric

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/pkg/security"
)

// serverOwner adapts Server to the lifecycle suite (design D3). It reads the three things a
// started Server retains: the listener, the http.Server and the serve goroutine, which Server
// holds through the serveDone channel until Stop has consumed its result.
type serverOwner struct{ s *Server }

func (o serverOwner) Start(ctx context.Context) error { return o.s.Start(ctx) }
func (o serverOwner) Stop(ctx context.Context) error  { return o.s.Stop(ctx) }

func (o serverOwner) Observe() lifecycletest.Observation {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	var held []string
	if o.s.listener != nil {
		held = append(held, "listener")
	}
	if o.s.server != nil {
		held = append(held, "http.Server")
	}
	if o.s.serveDone != nil {
		held = append(held, "serve goroutine")
	}
	return lifecycletest.Observation{Unresolved: held}
}

// newEphemeralServer returns a Server whose Start binds an ephemeral port. NewServer maps port 0 to
// 9090, so the field is set after construction, as TestServerNativeStartReportsOwnedEphemeralListener does.
func newEphemeralServer() *Server {
	s := NewServer(9090, "/metrics", NewMetricsRegistry(), security.Config{})
	s.port = 0
	return s
}

func TestServerLifecycleSuite(t *testing.T) {
	// The test holds a port on every interface, so the must-fail Server's own bind of ":port" fails
	// synchronously in Start (handler.go, net.Listen) on Linux and macOS alike.
	held, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("hold a port: %v", err)
	}
	t.Cleanup(func() { _ = held.Close() })
	heldPort := held.Addr().(*net.TCPAddr).Port
	mustFail := func() lifecycletest.Owner {
		return serverOwner{NewServer(heldPort, "/metrics", NewMetricsRegistry(), security.Config{})}
	}
	lifecycletest.Run(t, func() lifecycletest.Owner { return serverOwner{newEphemeralServer()} }, mustFail,
		lifecycletest.Promise{})
}

// TestServerAbortStopReportsContext is metric-abort-stop-reports-context (design D3). On an idle
// server Shutdown returns nil even when its context has ended, so Stop then reaches a select whose
// two cases, the serve result and ctx.Done, can both be ready. The test makes both ready on purpose
// by replacing serveDone with a channel already holding a result; the select then picks either
// case with equal chance, so over 30 servers a Stop that trusts the pick returns nil at least once
// unless the 2^-30 event happens. Every Stop handed an ended context must report that context.
func TestServerAbortStopReportsContext(t *testing.T) {
	const servers = 30
	var nils int
	for i := range servers {
		s := newEphemeralServer()
		if err := s.Start(t.Context()); err != nil {
			t.Fatalf("server %d: Start: %v", i, err)
		}
		s.mu.Lock()
		served := s.serveDone
		ready := make(chan error, 1)
		ready <- nil
		s.serveDone = ready
		s.mu.Unlock()

		ended, cancel := context.WithCancel(t.Context())
		cancel()
		err := s.Stop(ended)
		// Stop closed the listener, so the real serve goroutine returns; join it here.
		<-served
		switch {
		case err == nil:
			nils++
		case !errors.Is(err, context.Canceled):
			t.Errorf("server %d: Stop with an ended context returned %v; want context.Canceled", i, err)
		}
	}
	if nils > 0 {
		t.Errorf("%d of %d Stops with an ended context returned nil; want context.Canceled from every one", nils, servers)
	}
}

// TestServerForcedStopJoinsServeWithoutTimer is metric-forced-join-without-timer (design D3). Once
// Stop has force-closed the server and listener, Serve returns, so Stop waits for the serve result
// itself and not for a fixed bound. Inside the bubble the serve result is withheld while the fake
// clock moves well past any such bound: Stop must still be waiting, and must return once the
// result arrives.
func TestServerForcedStopJoinsServeWithoutTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		withheld := make(chan error, 1)
		s := NewServer(9090, "/metrics", NewMetricsRegistry(), security.Config{})
		s.used = true
		s.server = &http.Server{}
		s.serveDone = withheld

		ended, cancel := context.WithCancel(t.Context())
		cancel()
		returned := make(chan error, 1)
		go func() { returned <- s.Stop(ended) }()

		synctest.Wait()
		<-time.After(time.Hour) // the bubble's clock: an hour passes at once
		synctest.Wait()
		select {
		case err := <-returned:
			t.Fatalf("Stop returned %v before the serve result arrived; the forced join must wait for it", err)
		default:
		}

		withheld <- http.ErrServerClosed
		err := <-returned
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Stop returned %v; want context.Canceled", err)
		}
		if err != nil && strings.Contains(err.Error(), "forced metrics serve completion") {
			t.Errorf("Stop reported a join bound: %v", err)
		}
	})
}
