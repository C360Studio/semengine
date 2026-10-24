package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/errs"
)

// newListenerMetrics returns a Metrics built by NewMetrics from an empty configuration, whose
// server will serve on a loopback listener the test bound. testListener is the one test field
// Metrics keeps: metric.NewServer maps port 0 to 9090, so no configuration asks for a
// kernel-picked port (design D4, S9).
func newListenerMetrics(t *testing.T, registry *metric.MetricsRegistry) *Metrics {
	t.Helper()
	svc, err := NewMetrics(json.RawMessage(`{}`), &Dependencies{MetricsRegistry: registry})
	require.NoError(t, err)
	m, ok := svc.(*Metrics)
	require.True(t, ok, "NewMetrics returned %T", svc)
	m.testListener = boundMetricsListener(t)
	return m
}

// boundMetricsListener returns an ephemeral loopback listener, closed when the test ends unless a
// started server took it over first.
func boundMetricsListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

// heldPort returns a port a listener the test holds is bound to, on every interface, so a server
// binding ":port" fails (probe H1 on #343, as TestMaybeStartPProfLogsABindFailure holds its port).
func heldPort(t *testing.T) int {
	t.Helper()
	// Every address: blocks a server's ":port" bind (probe H1 on #343).
	held, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = held.Close() })
	return held.Addr().(*net.TCPAddr).Port
}

// requireListenerReleased fails unless listenerReleased reports addr, the address a listener
// reported, released.
func requireListenerReleased(t *testing.T, addr string) {
	t.Helper()
	require.NoError(t, listenerReleased(addr), "the listener at %s is not released", addr)
}

// listenerReleased returns nil when a dial to the listener's loopback address is refused. A
// listener bound on every address reports an unspecified host ("[::]" or "0.0.0.0"); a dial to
// that reaches the local system in both families, so any other process whose loopback socket
// shares the port answers it. On the owner's Mac a daemon holds 127.0.0.1:63150, and macOS hands
// 63150 to a port-0 listener on every address regardless, so the released port read as still
// accepting (#388; #372 was the same socket answering 404). The dial goes to the loopback address
// of the listener's own family, which only that listener serves. Only a refused connection counts:
// any other dial error is a broken probe, not a released port.
func listenerReleased(addr string) error {
	target, err := loopbackAddr(addr)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("tcp", target, time.Second)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("the listener at %s still accepts connections on %s", addr, target)
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("probe of %s: %w", target, err)
	}
	return nil
}

// loopbackAddr returns addr with an unspecified host replaced by the loopback address of its
// family; any other host is kept.
func loopbackAddr(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsUnspecified() {
		return addr, nil
	}
	if ip.To4() != nil {
		return net.JoinHostPort("127.0.0.1", port), nil
	}
	return net.JoinHostPort("::1", port), nil
}

// TestListenerReleasedSeesOnlyTheListenersOwnFamily: a loopback socket of another process on the
// port a listener bound on every address has just released must not read as that listener still
// accepting (#388). A Go tcp4 loopback listener stands in for the daemon: macOS lets it bind beside
// the open wildcard listener, Linux refuses it, and the check runs either way. The open listener
// must still read as held, so the check is shown able to fail.
func TestListenerReleasedSeesOnlyTheListenersOwnFamily(t *testing.T) {
	// Every address: plays the manager's every-address listener, as #388 needs.
	wildcard, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = wildcard.Close() })
	addr := wildcard.Addr().String()
	port := wildcard.Addr().(*net.TCPAddr).Port
	require.Error(t, listenerReleased(addr), "an open listener read as released")

	standIn, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Logf("this OS refuses a loopback socket beside %s: %v", addr, err)
	} else {
		t.Cleanup(func() { _ = standIn.Close() })
	}
	require.NoError(t, wildcard.Close())
	require.NoError(t, listenerReleased(addr))
}

// blockingCollector is a Prometheus collector whose Collect blocks until unblock is called, so a
// scrape stays in flight while a test stops the server.
type blockingCollector struct {
	desc        *prometheus.Desc
	entered     chan struct{}
	enterOnce   sync.Once
	release     chan struct{}
	releaseOnce sync.Once
}

func newBlockingCollector() *blockingCollector {
	return &blockingCollector{
		desc:    prometheus.NewDesc("semengine_test_blocking", "A collector that blocks until released.", nil, nil),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (c *blockingCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c *blockingCollector) Collect(ch chan<- prometheus.Metric) {
	c.enterOnce.Do(func() { close(c.entered) })
	<-c.release
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, 1)
}

func (c *blockingCollector) unblock() { c.releaseOnce.Do(func() { close(c.release) }) }

// scrape requests the metrics path at addr once, on a connection it does not keep.
func scrape(ctx context.Context, addr string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/metrics", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if err := response.Body.Close(); err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("scrape answered %d", response.StatusCode)
	}
	return nil
}

// TestMetricsStopRejectsConcurrentStop is the pin's
// TestMetricsStopRunsProviderOutsideLockAndRejectsConcurrentStop on the real server: the pin's
// fake provider went with the metricsServer interface (design D4, S10). A scrape held in flight
// keeps one Stop waiting for it; a second Stop made meanwhile is refused as transient, whichever
// of the two entered first, and the waiting one returns nil once the scrape ends.
//
// spec: service-lifecycle / The metrics service serves on its own or through the manager
func TestMetricsStopRejectsConcurrentStop(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	collector := newBlockingCollector()
	require.NoError(t, registry.PrometheusRegistry().Register(collector))
	m := newListenerMetrics(t, registry)
	addr := m.testListener.Addr().String()
	require.NoError(t, m.Start(t.Context()))
	t.Cleanup(func() {
		collector.unblock()
		require.NoError(t, m.Stop(stopContext(t)))
	})

	scraped := make(chan error, 1)
	go func() { scraped <- scrape(t.Context(), addr) }()
	<-collector.entered

	stops := make(chan error, 2)
	for range 2 {
		go func() { stops <- m.Stop(stopContext(t)) }()
	}
	// The Stop that entered first waits for the scrape, which waits for unblock, so the first
	// result is the refused one.
	refused := <-stops
	require.Error(t, refused)
	require.True(t, errs.IsTransient(refused), "a concurrent Stop is refused as transient, got %v", refused)

	collector.unblock()
	require.NoError(t, <-stops)
	require.NoError(t, <-scraped, "the scrape in flight when Stop began was not served")
	require.NoError(t, m.Stop(stopContext(t)), "a Stop after one that returned nil")
	requireListenerReleased(t, addr)
}

// TestMetricsRollsBackItsServerWhenBaseServiceStartFails: when the BaseService refuses its Start
// after the server has bound, Metrics.Start returns that refusal, holds no server and has released
// the listener (pin TestMetricsRollsBackBoundProviderWhenBaseCommitFails, built through NewMetrics).
//
// spec: service-lifecycle / The metrics service serves on its own or through the manager
func TestMetricsRollsBackItsServerWhenBaseServiceStartFails(t *testing.T) {
	m := newListenerMetrics(t, metric.NewMetricsRegistry())
	addr := m.testListener.Addr().String()
	// A BaseService stopped before its Start refuses that Start (PR 1,
	// TestBaseServiceStopBeforeStartRejectsSameInstanceStart).
	require.NoError(t, m.BaseService.Stop(stopContext(t)))

	err := m.Start(t.Context())
	require.ErrorIs(t, err, errs.ErrAlreadyStarted)
	require.Nil(t, m.server, "a failed Start kept its server")
	requireListenerReleased(t, addr)
	require.NoError(t, m.Stop(stopContext(t)))
}

// TestMetricsLifecycleContextAndStopBeforeStartAreImmutable: a Start refused for a nil or ended
// context, and a Stop refused for a nil context, leave the service unused, so it still starts;
// a Stop before any Start makes a later Start refused. The pin read its private used flag; this
// reads what a caller sees.
//
// spec: service-lifecycle / The metrics service serves on its own or through the manager
func TestMetricsLifecycleContextAndStopBeforeStartAreImmutable(t *testing.T) {
	t.Run("refused calls leave it unused", func(t *testing.T) {
		m := newListenerMetrics(t, metric.NewMetricsRegistry())
		var nilCtx context.Context // the nil context is the input under test
		ended, cancel := context.WithCancel(t.Context())
		cancel()

		require.Error(t, m.Start(nilCtx))
		require.Error(t, m.Start(ended))
		require.Error(t, m.Stop(nilCtx))

		require.NoError(t, m.Start(t.Context()), "a refused call used the instance")
		require.NoError(t, m.Stop(stopContext(t)))
	})
	t.Run("stop before start", func(t *testing.T) {
		m := newListenerMetrics(t, metric.NewMetricsRegistry())
		require.NoError(t, m.Stop(stopContext(t)))
		require.ErrorIs(t, m.Start(t.Context()), errs.ErrAlreadyStarted)
		require.NoError(t, m.Stop(stopContext(t)))
	})
}

// TestMetricsRefusesAClaimAfterStart: the manager's claim on the metrics server is refused, wrapping
// errs.ErrAlreadyStarted, once Metrics.Start or Stop has been called or a server was already
// claimed; the first claim on an unused Metrics returns a server (design D4).
//
// spec: service-lifecycle / The metrics service serves on its own or through the manager
func TestMetricsRefusesAClaimAfterStart(t *testing.T) {
	t.Run("after Start", func(t *testing.T) {
		m := newListenerMetrics(t, metric.NewMetricsRegistry())
		require.NoError(t, m.Start(t.Context()))
		t.Cleanup(func() { require.NoError(t, m.Stop(stopContext(t))) })

		server, err := m.claimManagerServer()
		require.ErrorIs(t, err, errs.ErrAlreadyStarted)
		require.Nil(t, server)
	})
	t.Run("after Stop", func(t *testing.T) {
		m := newListenerMetrics(t, metric.NewMetricsRegistry())
		require.NoError(t, m.Stop(stopContext(t)))

		server, err := m.claimManagerServer()
		require.ErrorIs(t, err, errs.ErrAlreadyStarted)
		require.Nil(t, server)
	})
	t.Run("second claim", func(t *testing.T) {
		m := newListenerMetrics(t, metric.NewMetricsRegistry())
		first, err := m.claimManagerServer()
		require.NoError(t, err)
		require.NotNil(t, first)

		second, err := m.claimManagerServer()
		require.ErrorIs(t, err, errs.ErrAlreadyStarted)
		require.Nil(t, second)
	})
}

// metricsOwner adapts a standalone Metrics to the lifecycle suite (lifecycle-suite, "Observe adapter
// contract"). A started Metrics holds its server and its BaseService's health goroutine; Observe
// lists "metrics-server" while the server field is set and "health-loop" while that goroutine's
// done channel is open. Nothing is added to Metrics for the adapter (#186, #190).
//
// Observe reads the server field without Metrics' lock: the suite calls Observe only after the
// Start or Stop it judges has returned, and the health goroutine only reads the field, so the read
// does not race. Calls is empty for BaseService's reason (base_lifecycle_test.go).
type metricsOwner struct {
	m *Metrics
}

func (o *metricsOwner) Start(ctx context.Context) error { return o.m.Start(ctx) }
func (o *metricsOwner) Stop(ctx context.Context) error  { return o.m.Stop(ctx) }

func (o *metricsOwner) Observe() lifecycletest.Observation {
	var held []string
	if o.m.server != nil {
		held = append(held, "metrics-server")
	}
	base := o.m.BaseService
	base.mu.RLock()
	done := base.healthDone
	base.mu.RUnlock()
	if done != nil && !isClosed(done) {
		held = append(held, "health-loop")
	}
	return lifecycletest.Observation{Unresolved: held}
}

// TestMetricsPassesTheLifecycleSuite runs the lifecycle suite on a standalone Metrics serving on a
// test listener (design D4, D7; the pin passed 7 of the 8 checks). The must-fail owner is a Metrics
// configured with a port a listener the test holds, so its Start cannot bind. The service is
// one-shot: no restart is promised.
//
// spec: service-lifecycle / The metrics service serves on its own or through the manager
func TestMetricsPassesTheLifecycleSuite(t *testing.T) {
	factory := func() lifecycletest.Owner {
		return &metricsOwner{m: newListenerMetrics(t, metric.NewMetricsRegistry())}
	}
	mustFail := func() lifecycletest.Owner {
		raw := json.RawMessage(fmt.Sprintf(`{"port":%d}`, heldPort(t)))
		svc, err := NewMetrics(raw, &Dependencies{MetricsRegistry: metric.NewMetricsRegistry()})
		require.NoError(t, err)
		return &metricsOwner{m: svc.(*Metrics)}
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles": the suite judges only what is left
	// after a Stop, so an adapter that never reported a handle would pass it.
	t.Run("StartedOwnerReportsItsHandles", func(t *testing.T) {
		owner := &metricsOwner{m: newListenerMetrics(t, metric.NewMetricsRegistry())}
		require.NoError(t, owner.Start(t.Context()))
		require.Equal(t, []string{"metrics-server", "health-loop"}, owner.Observe().Unresolved)
		require.NoError(t, owner.Stop(stopContext(t)))
		require.Empty(t, owner.Observe().Unresolved, "a Stop that returned nil left a handle")
	})
}
