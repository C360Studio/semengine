package metric

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/pkg/security"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// serverOwner adapts Server to the lifecycle suite (design D3). It reads the four things a
// started Server retains: the listener, the http.Server, the serve goroutine, which Server holds
// through the serveDone channel until Stop has consumed its result, and the requests its handler
// admitted and has not returned from, with the metrics collection each one runs.
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
	if o.s.requests != nil && o.s.requests.pending() > 0 {
		held = append(held, "admitted request")
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
// result arrives. It proves the serve join only; that Stop also waits for admitted requests is
// TestServerRepeatedStopWaitsForAdmittedRequest's, through a real request.
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

// exitingCollector blocks its first Collect until released and reports when that Collect has
// returned, so a test can tell whether the collection a request started is still running.
type exitingCollector struct {
	desc                     *prometheus.Desc
	entered, release, exited chan struct{}
	once                     sync.Once
}

func newExitingCollector() *exitingCollector {
	return &exitingCollector{
		desc:    prometheus.NewDesc("semengine_lifecycle_admitted", "test", nil, nil),
		entered: make(chan struct{}), release: make(chan struct{}), exited: make(chan struct{}),
	}
}

func (c *exitingCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c *exitingCollector) Collect(ch chan<- prometheus.Metric) {
	first := false
	c.once.Do(func() { first = true; close(c.entered) })
	if !first {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, 1)
		return
	}
	<-c.release
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, 1)
	close(c.exited)
}

// TestServerRepeatedStopWaitsForAdmittedRequest: a request the server admitted is work the server
// owns until its handler returns. A Stop whose context ends first reports that context and leaves
// the work pending; a repeated Stop does not report completion while the request's collection
// still runs, and returns nil only once it has finished.
func TestServerRepeatedStopWaitsForAdmittedRequest(t *testing.T) {
	registry := NewMetricsRegistry()
	collector := newExitingCollector()
	registry.PrometheusRegistry().MustRegister(collector)
	server := NewServer(9090, "/metrics", registry, security.Config{})
	owner := serverOwner{server}
	require.NoError(t, server.StartWithListener(t.Context(), boundServerListener(t)))
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(collector.release) }) })

	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := testServerGET(t, server.Address(), nil)
		if err == nil {
			_ = response.Body.Close()
		}
	}()
	<-collector.entered

	ended, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, server.Stop(ended), context.Canceled, "first Stop with an ended context")
	require.ErrorIs(t, server.Stop(ended), context.Canceled,
		"a repeated Stop must not report completion while the admitted request's collection runs")
	require.Equal(t, []string{"admitted request"}, owner.Observe().Unresolved)

	stopped := make(chan error, 1)
	go func() { stopped <- server.Stop(t.Context()) }()
	release.Do(func() { close(collector.release) })
	require.NoError(t, <-stopped)
	select {
	case <-collector.exited:
	default:
		t.Fatal("Stop returned nil before the admitted request's collection finished")
	}
	<-requestDone
	require.Empty(t, owner.Observe().Unresolved)
	require.NoError(t, server.Stop(t.Context()), "a Stop after completion is a nil no-op")
}
