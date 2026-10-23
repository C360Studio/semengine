package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/errs"
)

// The metrics forwarder's construction, Stop and publish-loop tests (design D2, D3, D4; D6 U-c).
// Every forwarder is built through newTestMetricsForwarder over a client that never connects, so
// every publish fails at once with natsclient.ErrNotConnected and the loop's failure records and
// counts are what a test reads. The tests that read the loop run inside a synctest bubble: the
// unconnected client starts no goroutine and no timer, so the bubble settles once the loop waits
// on its ticker (design "Harness rules").

const (
	forwarderPublishFailedLog = "failed to publish metrics to NATS"
	forwarderGatherFailedLog  = "failed to gather metrics"

	// forwarderProbeSubject is the subject of probeCounter's one sample: it has no component
	// label, so the forwarder publishes it under system (design D4).
	forwarderProbeSubject = "metrics.system.forwarder_probe_total"
)

// forwarderErrors is the label set of the forwarder's drop counts on semengine_errors_total.
func forwarderErrors(errorType string) map[string]string {
	return map[string]string{"service": "metrics-forwarder", "type": errorType}
}

// registerCounter registers a counter named name on registry and increments it, so a gather
// returns one sample of it.
func registerCounter(registry *metric.MetricsRegistry, name string) {
	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: name, Help: "U-c probe counter"})
	registry.PrometheusRegistry().MustRegister(counter)
	counter.Inc()
}

// publishFailures returns the subject of every Debug "failed to publish metrics to NATS" record,
// in the order they were logged.
func publishFailures(h *recordingHandler) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var subjects []string
	for _, r := range h.records {
		if r.Level != slog.LevelDebug || r.Message != forwarderPublishFailedLog {
			continue
		}
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "subject" {
				subjects = append(subjects, a.Value.String())
				return false
			}
			return true
		})
	}
	return subjects
}

// countRecords returns how many records with message msg were logged at Debug.
func countRecords(h *recordingHandler, msg string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.records {
		if r.Level == slog.LevelDebug && r.Message == msg {
			n++
		}
	}
	return n
}

// countOf returns how many times subject occurs in subjects.
func countOf(subjects []string, subject string) int {
	n := 0
	for _, s := range subjects {
		if s == subject {
			n++
		}
	}
	return n
}

// stopForwarderAtCleanup stops mf when the test ends, under a bounded context that does not end
// with the test's. Inside a synctest bubble the cleanup runs in the bubble, before it returns.
func stopForwarderAtCleanup(t *testing.T, mf *MetricsForwarder) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		if err := mf.Stop(ctx); err != nil {
			t.Errorf("cleanup Stop: %v", err)
		}
	})
}

// --- configuration (design D3; pin metrics_forwarder_test.go:27-186, :971-1022) ---------------

// TestMetricsForwarderConstructionRefusals is D3's forwarder refusals, one subtest each, every one
// an invalid-data refusal naming what it refuses, and the pin's accepted rows read as the applied
// values. The pin refused "push_interval": "" through a second decode; it now takes the default,
// as the heartbeat's and storage observability's intervals do. A key the forwarder does not define,
// or one in another letter case, is refused (#344).
//
// spec: service-lifecycle / The metrics forwarder refuses an invalid configuration
func TestMetricsForwarderConstructionRefusals(t *testing.T) {
	type refusal struct {
		config string
		deps   func(t *testing.T) *Dependencies
		names  string
	}
	full := func(t *testing.T) *Dependencies {
		return &Dependencies{NATSClient: newUnconnectedClient(t), MetricsRegistry: metric.NewMetricsRegistry()}
	}
	withConfig := func(config, names string) refusal { return refusal{config: config, deps: full, names: names} }

	refusals := map[string]refusal{
		"nil dependencies": {config: `{}`, deps: func(*testing.T) *Dependencies { return nil }, names: "requires dependencies"},
		"nil NATS client": {config: `{}`, deps: func(*testing.T) *Dependencies {
			return &Dependencies{MetricsRegistry: metric.NewMetricsRegistry()}
		}, names: "NATS client"},
		"nil metrics registry": {config: `{}`, deps: func(t *testing.T) *Dependencies {
			return &Dependencies{NATSClient: newUnconnectedClient(t)}
		}, names: "metrics registry"},
		"interval missing its unit":  withConfig(`{"push_interval":"10"}`, "push_interval"),
		"interval with unknown unit": withConfig(`{"push_interval":"10x"}`, "push_interval"),
		"negative interval":          withConfig(`{"push_interval":"-5s"}`, "push_interval"),
		"zero interval":              withConfig(`{"push_interval":"0s"}`, "push_interval"),
		"malformed JSON":             withConfig(`{"push_interval": `, "metrics-forwarder config"),
		"unknown key":                withConfig(`{"push_intervall":"5s"}`, "push_intervall"),
		"key in other letter case":   withConfig(`{"Push_Interval":"5s"}`, "Push_Interval"),
		"flag in other letter case":  withConfig(`{"Include_Go_Metrics":true}`, "Include_Go_Metrics"),
	}
	for name, tc := range refusals {
		t.Run(name, func(t *testing.T) {
			built, err := NewMetricsForwarderService(json.RawMessage(tc.config), tc.deps(t))
			require.Error(t, err, "construction must refuse, not default")
			require.Nil(t, built)
			assert.True(t, errs.IsInvalid(err), "want an invalid-data refusal, got %v", err)
			assert.Contains(t, err.Error(), tc.names, "the refusal names what it refuses")
		})
	}

	type applied struct {
		interval    time.Duration
		goMetrics   bool
		procMetrics bool
	}
	accepted := map[string]struct {
		config json.RawMessage
		want   applied
	}{
		"custom interval":          {json.RawMessage(`{"push_interval":"10s"}`), applied{interval: 10 * time.Second}},
		"one-minute interval":      {json.RawMessage(`{"push_interval":"1m"}`), applied{interval: time.Minute}},
		"empty document":           {json.RawMessage(`{}`), applied{interval: 5 * time.Second}},
		"no document":              {nil, applied{interval: 5 * time.Second}},
		"empty interval":           {json.RawMessage(`{"push_interval":""}`), applied{interval: 5 * time.Second}},
		"both system metric flags": {json.RawMessage(`{"include_go_metrics":true,"include_proc_metrics":true}`), applied{5 * time.Second, true, true}},
		"go metrics only":          {json.RawMessage(`{"include_go_metrics":true}`), applied{5 * time.Second, true, false}},
		"process metrics only":     {json.RawMessage(`{"include_proc_metrics":true}`), applied{5 * time.Second, false, true}},
	}
	for name, tc := range accepted {
		t.Run("accepted/"+name, func(t *testing.T) {
			built, err := NewMetricsForwarderService(tc.config, full(t))
			require.NoError(t, err)
			mf, ok := built.(*MetricsForwarder)
			require.True(t, ok, "NewMetricsForwarderService returned %T, want *MetricsForwarder", built)
			assert.Equal(t, tc.want, applied{mf.interval, mf.includeGoMetrics, mf.includeProcMetrics})
		})
	}
}

// --- lifecycle edges (design D2) ---------------------------------------------------------------

// TestMetricsForwarderFailedStopIsRetried is D2's "Stop": a Stop whose context ends before the
// loop returns reports that context's error, and the next Stop waits for the loop again. The
// composition's logger holds the loop's first "failed to publish metrics to NATS" record (every
// publish fails over a client that never connects), so the loop is inside its log call and cannot
// return until released.
//
// The two 200 ms windows are this file's only real-clock windows besides the bound on the held
// record (design "Harness rules"). The first Stop's deadline cannot fail a correct service: the
// held loop keeps the join from finishing, so it can only end at the deadline. The second window
// is an observation: a forwarder that forgets its loop after a failed Stop returns nil from the
// second Stop at once; a correct one waits for the held loop.
//
// spec: service-lifecycle / The metrics forwarder's Start bounds Start and Stop is the only stop
func TestMetricsForwarderFailedStopIsRetried(t *testing.T) {
	const window = 200 * time.Millisecond
	handler := newHoldingHandler(t, forwarderPublishFailedLog)
	mf, registry := newTestMetricsForwarder(t, newUnconnectedClient(t), `{}`, slog.New(handler))
	// By default every series a fresh registry gathers is go_ or process_, which are left out:
	// without the probe the loop publishes nothing and logs nothing.
	registerCounter(registry, "forwarder_probe_total")
	require.NoError(t, mf.Start(t.Context()))
	stopForwarderAtCleanup(t, mf)
	t.Cleanup(handler.releaseHeld)

	held := time.NewTimer(stopBound)
	defer held.Stop()
	select {
	case <-handler.heldRecord():
	case <-held.C:
		t.Fatalf("the loop's first publish failure was not logged within %s", stopBound)
	}

	firstCtx, cancelFirst := context.WithTimeout(t.Context(), window)
	defer cancelFirst()
	require.ErrorIs(t, mf.Stop(firstCtx), context.DeadlineExceeded,
		"a Stop whose loop cannot return reports its context's error")

	secondCtx, cancelSecond := context.WithTimeout(t.Context(), stopBound)
	defer cancelSecond()
	secondResult := make(chan error, 1)
	go func() { secondResult <- mf.Stop(secondCtx) }()
	observation := time.NewTimer(window)
	defer observation.Stop()
	select {
	case err := <-secondResult:
		t.Fatalf("the second Stop returned (%v) while the loop was held: the failed Stop forgot its loop", err)
	case <-observation.C:
	}

	handler.releaseHeld()
	require.NoError(t, <-secondResult, "the second Stop finishes the join the first left")
	require.NoError(t, mf.Stop(stopContext(t)), "a Stop after one that returned nil returns nil")
	owner := &metricsForwarderOwner{mf: mf}
	assert.Empty(t, owner.Observe().Unresolved, "a stopped forwarder holds no loop and no base status")
}

// --- the publish loop (design D4; pin metrics_forwarder_test.go:218-271, :608-640, :810-848) ----

// TestMetricsForwarderCadence carries the pin's TestMetricsForwarder_TickerInterval onto the
// production constructor: with push_interval 100ms, the probe counter's subject is attempted once
// at Start and once per tick. 350 ms of the bubble's clock owe the ticks at 100, 200 and 300 ms, so
// the count is exactly four: a loop that never ticks leaves one, one without its first round three,
// a faster one more. Rounds are counted as the probe subject's failed publishes, one per round.
//
// spec: service-lifecycle / The metrics forwarder publishes every sample and counts its drops
func TestMetricsForwarderCadence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		handler := &recordingHandler{}
		mf, registry := newTestMetricsForwarder(t, newUnconnectedClient(t), `{"push_interval":"100ms"}`,
			slog.New(handler))
		registerCounter(registry, "forwarder_probe_total")

		require.NoError(t, mf.Start(t.Context()))
		stopForwarderAtCleanup(t, mf)
		<-time.After(350 * time.Millisecond) // the bubble's clock: rounds at 0, 100, 200 and 300 ms
		synctest.Wait()
		require.NoError(t, mf.Stop(stopContext(t)))

		assert.Equal(t, 4, countOf(publishFailures(handler), forwarderProbeSubject),
			"one round at Start plus one per 100ms tick in 350ms")
	})
}

// TestMetricsForwarderIncludeFlagsReachThePublisher: a go_ and a process_ counter registered on the
// injected registry are attempted only when include_go_metrics or include_proc_metrics is set, and
// the probe counter, neither, always is (pin metrics_forwarder_test.go:971-1022's flags, read at
// the publisher rather than at the config field).
//
// spec: service-lifecycle / The metrics forwarder publishes every sample and counts its drops
func TestMetricsForwarderIncludeFlagsReachThePublisher(t *testing.T) {
	const (
		goSubject   = "metrics.system.go_forwarder_probe_total"
		procSubject = "metrics.system.process_forwarder_probe_total"
	)
	cases := map[string]struct {
		config           string
		wantGo, wantProc bool
	}{
		"neither flag":      {config: `{}`},
		"go metrics":        {config: `{"include_go_metrics":true}`, wantGo: true},
		"process metrics":   {config: `{"include_proc_metrics":true}`, wantProc: true},
		"both flags":        {config: `{"include_go_metrics":true,"include_proc_metrics":true}`, wantGo: true, wantProc: true},
		"both flags false":  {config: `{"include_go_metrics":false,"include_proc_metrics":false}`},
		"process flag only": {config: `{"include_go_metrics":false,"include_proc_metrics":true}`, wantProc: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				handler := &recordingHandler{}
				mf, registry := newTestMetricsForwarder(t, newUnconnectedClient(t), tc.config, slog.New(handler))
				registerCounter(registry, "forwarder_probe_total")
				registerCounter(registry, "go_forwarder_probe_total")
				registerCounter(registry, "process_forwarder_probe_total")

				require.NoError(t, mf.Start(t.Context()))
				stopForwarderAtCleanup(t, mf)
				synctest.Wait() // the first round has run; the loop waits on its ticker
				require.NoError(t, mf.Stop(stopContext(t)))

				attempted := publishFailures(handler)
				assert.Equal(t, 1, countOf(attempted, forwarderProbeSubject), "the probe counter is always published")
				assert.Equal(t, tc.wantGo, countOf(attempted, goSubject) == 1, "go_ attempted: %v", attempted)
				assert.Equal(t, tc.wantProc, countOf(attempted, procSubject) == 1, "process_ attempted: %v", attempted)
			})
		})
	}
}

// TestMetricsForwarderPublishFailureIsCounted is D4's "Drops are declared" for a publish: every
// failed publish increments semengine_errors_total{service="metrics-forwarder",
// type="metrics_publish"} and is logged at Debug naming its subject, and the loop goes on. The
// count is read after Stop, which joins the loop, and must equal the records one to one.
//
// spec: service-lifecycle / The metrics forwarder publishes every sample and counts its drops
func TestMetricsForwarderPublishFailureIsCounted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		handler := &recordingHandler{}
		mf, registry := newTestMetricsForwarder(t, newUnconnectedClient(t), `{"push_interval":"100ms"}`,
			slog.New(handler))
		registerCounter(registry, "forwarder_probe_total")

		require.NoError(t, mf.Start(t.Context()))
		stopForwarderAtCleanup(t, mf)
		<-time.After(150 * time.Millisecond) // the bubble's clock: rounds at 0 and 100 ms
		synctest.Wait()
		require.NoError(t, mf.Stop(stopContext(t)))

		attempted := publishFailures(handler)
		require.Equal(t, 2, countOf(attempted, forwarderProbeSubject),
			"the loop goes on after a failed publish: %v", attempted)
		count := requireGauge(t, registry, "semengine_errors_total", forwarderErrors("metrics_publish"))
		assert.Equal(t, float64(len(attempted)), count, "every failed publish is counted once")
		_, gatherCounted := gaugeValue(t, registry, "semengine_errors_total", forwarderErrors("metrics_gather"))
		assert.False(t, gatherCounted, "a publish failure is not a gather failure")
	})
}

// invalidCollector is a collector whose every collection reports an invalid metric, so the
// registry's Gather returns an error (pin metrics_forwarder_test.go:810-848's failing gatherer,
// on the injected registry rather than a mock).
type invalidCollector struct{ desc *prometheus.Desc }

func newInvalidCollector() *invalidCollector {
	return &invalidCollector{desc: prometheus.NewDesc("forwarder_invalid_probe", "U-c failing collector", nil, nil)}
}

func (c *invalidCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }
func (c *invalidCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.NewInvalidMetric(c.desc, errors.New("U-c probe: collection failed"))
}

// TestMetricsForwarderGatherFailureIsCounted is D4's "Drops are declared" for a gather: a failed
// gather increments semengine_errors_total{service="metrics-forwarder",type="metrics_gather"}, is
// logged at Debug, and skips that round, so nothing is published, not even the samples the gather
// did return. requireGauge needs a clean gather, so the failing collector is unregistered after
// Stop, before the count is read.
//
// spec: service-lifecycle / The metrics forwarder publishes every sample and counts its drops
func TestMetricsForwarderGatherFailureIsCounted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		handler := &recordingHandler{}
		mf, registry := newTestMetricsForwarder(t, newUnconnectedClient(t), `{"push_interval":"100ms"}`,
			slog.New(handler))
		registerCounter(registry, "forwarder_probe_total")
		failing := newInvalidCollector()
		registry.PrometheusRegistry().MustRegister(failing)

		require.NoError(t, mf.Start(t.Context()))
		stopForwarderAtCleanup(t, mf)
		<-time.After(150 * time.Millisecond) // the bubble's clock: rounds at 0 and 100 ms
		synctest.Wait()
		require.NoError(t, mf.Stop(stopContext(t)))
		require.True(t, registry.PrometheusRegistry().Unregister(failing))

		assert.Equal(t, 2, countRecords(handler, forwarderGatherFailedLog), "each round's failed gather is logged")
		count := requireGauge(t, registry, "semengine_errors_total", forwarderErrors("metrics_gather"))
		assert.Equal(t, float64(2), count, "each failed gather is counted once")
		assert.Empty(t, publishFailures(handler), "a round whose gather failed publishes nothing")
		_, publishCounted := gaugeValue(t, registry, "semengine_errors_total", forwarderErrors("metrics_publish"))
		assert.False(t, publishCounted, "a skipped round attempts no publish")
	})
}
