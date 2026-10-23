//go:build integration

package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
)

// The metrics forwarder against the package's shared NATS server (design D2, D4, D6 U-e; the pin's
// concurrent gather, metrics_forwarder_test.go:641-696, now over the real wire under -race). Every
// sample is read back from a METRICS stream over metrics.> through a consumer, as an operator's
// reader would. The 1 s push interval of the start-context test is a real-clock window, and every
// wait in this file is bounded by forwardedSampleWait (design § Harness rules).

// forwardedSampleWait bounds every wait in this file for a sample or a count to appear.
const forwardedSampleWait = 30 * time.Second

// forwardedSample is one published sample decoded by the wire names design D4 lists, not by the
// forwarder's own type, so a renamed or dropped field fails here.
type forwardedSample struct {
	Timestamp int64             `json:"timestamp"`
	Name      string            `json:"name"`
	Component string            `json:"component"`
	Type      string            `json:"type"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels"`
}

// forwardedWireNames are the keys design D4 names, the whole set of a sample's object.
var forwardedWireNames = []string{"component", "labels", "name", "timestamp", "type", "value"}

// metricsStream creates the METRICS stream over metrics.> on the client's server, the stream the
// forwarder's PublishToStream needs, and returns it.
func metricsStream(t *testing.T, js jetstream.JetStream) jetstream.Stream {
	t.Helper()
	stream, err := js.CreateStream(t.Context(), jetstream.StreamConfig{
		Name:     "METRICS",
		Subjects: []string{"metrics.>"},
		Storage:  jetstream.MemoryStorage,
	})
	require.NoError(t, err)
	return stream
}

// awaitForwardedSample reads subject from the start of stream through an ordered consumer until a
// sample satisfies until, and returns it. Every message's object must carry exactly D4's keys. It
// fails the test, naming what it waited for and the last sample read, when forwardedSampleWait
// passes first.
func awaitForwardedSample(
	t *testing.T, stream jetstream.Stream, subject, what string, until func(forwardedSample) bool,
) forwardedSample {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), forwardedSampleWait)
	defer cancel()
	consumer, err := stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{FilterSubjects: []string{subject}})
	require.NoError(t, err)
	last := "no sample yet"
	for {
		if ctx.Err() != nil {
			t.Fatalf("%s: %s not seen within %v; last sample: %s", subject, what, forwardedSampleWait, last)
		}
		// One short fetch per pass; the 30 s bound is ctx's, checked above, so a fetch that ends
		// empty (nats: timeout) only starts the next pass.
		msg, err := consumer.Next(jetstream.FetchMaxWait(time.Second))
		if errors.Is(err, nats.ErrTimeout) {
			continue
		}
		require.NoError(t, err, "read %s", subject)
		require.Equal(t, subject, msg.Subject())
		last = string(msg.Data())
		var keys map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(msg.Data(), &keys), "sample on %s: %s", subject, last)
		got := make([]string, 0, len(keys))
		for key := range keys {
			got = append(got, key)
		}
		sort.Strings(got)
		require.Equal(t, forwardedWireNames, got, "sample keys on %s: %s", subject, last)
		var sample forwardedSample
		require.NoError(t, json.Unmarshal(msg.Data(), &sample), "sample on %s: %s", subject, last)
		if until(sample) {
			return sample
		}
	}
}

// awaitForwarderErrors waits until semengine_errors_total{service="metrics-forwarder",type=errType}
// reads want on registry, and fails the test, naming the last reading, when forwardedSampleWait
// passes first.
func awaitForwarderErrors(t *testing.T, registry *metric.MetricsRegistry, errType string, want float64) {
	t.Helper()
	labels := map[string]string{"service": "metrics-forwarder", "type": errType}
	ctx, cancel := context.WithTimeout(t.Context(), forwardedSampleWait)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		got, ok := gaugeValue(t, registry, "semengine_errors_total", labels)
		if ok && got == want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("semengine_errors_total %v = %v (present %v) within %v, want %v",
				labels, got, ok, forwardedSampleWait, want)
		case <-ticker.C:
		}
	}
}

// startMetricsForwarder starts mf under ctx and stops it when the test ends under a bounded
// detached context.
func startMetricsForwarder(t *testing.T, ctx context.Context, mf *MetricsForwarder) {
	t.Helper()
	require.NoError(t, mf.Start(ctx))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		require.NoError(t, mf.Stop(stopCtx))
	})
}

// TestMetricsForwarderPublishesEachSample checks that the forwarder's first round, run at Start,
// publishes every sample of a family on metrics.<component>.<name> with the component label, and
// a sample with no component label on metrics.system.<name>, each with its type, value and labels
// (design D4; pin mf:340-387).
func TestMetricsForwarderPublishesEachSample(t *testing.T) {
	client := fixtureClient(t)
	js, err := client.JetStream()
	require.NoError(t, err)
	stream := metricsStream(t, js)

	mf, registry := newTestMetricsForwarder(t, client, `{"push_interval":"1h"}`, discardLogger)
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "probe_requests_total",
		Help: "Requests the probe served, by route.",
	}, []string{"component", "route"})
	free := prometheus.NewGauge(prometheus.GaugeOpts{Name: "probe_free_bytes", Help: "Free bytes."})
	registry.PrometheusRegistry().MustRegister(requests, free)
	requests.WithLabelValues("probe", "a").Add(2)
	requests.WithLabelValues("probe", "b").Add(5)
	free.Set(7)

	before := time.Now().UnixMilli()
	startMetricsForwarder(t, t.Context(), mf)

	const requestsSubject = "metrics.probe.probe_requests_total"
	routeA := awaitForwardedSample(t, stream, requestsSubject, "route a", func(s forwardedSample) bool {
		return s.Labels["route"] == "a"
	})
	routeB := awaitForwardedSample(t, stream, requestsSubject, "route b", func(s forwardedSample) bool {
		return s.Labels["route"] == "b"
	})
	freeBytes := awaitForwardedSample(t, stream, "metrics.system.probe_free_bytes", "the gauge",
		func(forwardedSample) bool { return true })
	after := time.Now().UnixMilli()

	for _, s := range []forwardedSample{routeA, routeB, freeBytes} {
		assert.GreaterOrEqual(t, s.Timestamp, before, "%s timestamp precedes Start", s.Name)
		assert.LessOrEqual(t, s.Timestamp, after, "%s timestamp follows the read", s.Name)
	}
	routeA.Timestamp, routeB.Timestamp, freeBytes.Timestamp = 0, 0, 0
	assert.Equal(t, forwardedSample{
		Name: "probe_requests_total", Component: "probe", Type: "counter", Value: 2,
		Labels: map[string]string{"component": "probe", "route": "a"},
	}, routeA)
	assert.Equal(t, forwardedSample{
		Name: "probe_requests_total", Component: "probe", Type: "counter", Value: 5,
		Labels: map[string]string{"component": "probe", "route": "b"},
	}, routeB)
	assert.Equal(t, forwardedSample{
		Name: "probe_free_bytes", Component: "system", Type: "gauge", Value: 7,
		Labels: map[string]string{},
	}, freeBytes)
}

// TestMetricsForwarderCountsAnUnencodableSample checks that a sample JSON cannot carry (a NaN
// gauge) is counted on semengine_errors_total{service="metrics-forwarder",type="metrics_publish"}
// and never published, and that the round goes on to publish the samples after it (design D4,
// S8). The gauge's family sorts before the counter's, so a round that stopped at the failure
// would never publish the counter; the 1 h interval keeps the test to the one round Start runs.
func TestMetricsForwarderCountsAnUnencodableSample(t *testing.T) {
	client := fixtureClient(t)
	js, err := client.JetStream()
	require.NoError(t, err)
	stream := metricsStream(t, js)

	mf, registry := newTestMetricsForwarder(t, client, `{"push_interval":"1h"}`, discardLogger)
	ratio := prometheus.NewGauge(prometheus.GaugeOpts{Name: "probe_a_ratio", Help: "A ratio with no value."})
	delivered := prometheus.NewCounter(prometheus.CounterOpts{Name: "probe_b_total", Help: "A delivered counter."})
	registry.PrometheusRegistry().MustRegister(ratio, delivered)
	ratio.Set(math.NaN())
	delivered.Add(3)

	startMetricsForwarder(t, t.Context(), mf)

	sample := awaitForwardedSample(t, stream, "metrics.system.probe_b_total", "the counter after the NaN",
		func(forwardedSample) bool { return true })
	assert.Equal(t, 3.0, sample.Value)
	awaitForwarderErrors(t, registry, "metrics_publish", 1)

	info, err := stream.Info(t.Context(), jetstream.WithSubjectFilter("metrics.system.probe_a_ratio"))
	require.NoError(t, err)
	assert.Empty(t, info.State.Subjects, "the NaN sample must not be published")
}

// TestMetricsForwarderKeepsPublishingAfterStartContextEnds checks that ending the context Start
// was given, after Start returned, stops nothing: rounds go on at the 1 s interval and carry the
// counter's later value (design D2; probe sweep.md § 3, H6, where the pin stops). The counter is
// raised from ten goroutines while rounds gather and publish, the pin's concurrent gather
// (metrics_forwarder_test.go:641-696) under -race over the real wire.
func TestMetricsForwarderKeepsPublishingAfterStartContextEnds(t *testing.T) {
	client := fixtureClient(t)
	js, err := client.JetStream()
	require.NoError(t, err)
	stream := metricsStream(t, js)

	mf, registry := newTestMetricsForwarder(t, client, `{"push_interval":"1s"}`, discardLogger)
	ticks := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "probe_ticks_total", Help: "Ticks.", ConstLabels: prometheus.Labels{"component": "probe"},
	})
	registry.PrometheusRegistry().MustRegister(ticks)
	ticks.Inc()

	startCtx, endStart := context.WithCancel(t.Context())
	startMetricsForwarder(t, startCtx, mf)

	const subject = "metrics.probe.probe_ticks_total"
	awaitForwardedSample(t, stream, subject, "value 1 before the Start context ends",
		func(s forwardedSample) bool { return s.Value == 1 })
	endStart()
	require.ErrorIs(t, startCtx.Err(), context.Canceled)

	var raisers sync.WaitGroup
	for range 10 {
		raisers.Go(func() {
			for range 10 {
				ticks.Inc()
			}
		})
	}
	raisers.Wait()

	awaitForwardedSample(t, stream, subject, "value 101 after the Start context ended",
		func(s forwardedSample) bool { return s.Value == 101 })
}
