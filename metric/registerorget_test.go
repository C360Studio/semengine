package metric

import (
	"sync"
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

// gatheredSeries returns the value of every series of the named family the registry gathers:
// a counter's or gauge's value, a histogram's sample count. The oracle is Prometheus' own Gather,
// never the collector handle the test wrote through.
func gatheredSeries(t *testing.T, r *MetricsRegistry, name string) []float64 {
	t.Helper()
	families, err := r.PrometheusRegistry().Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		values := make([]float64, 0, len(family.Metric))
		for _, m := range family.Metric {
			values = append(values, seriesValue(family.GetType(), m))
		}
		return values
	}
	return nil
}

func seriesValue(kind dto.MetricType, m *dto.Metric) float64 {
	switch kind {
	case dto.MetricType_COUNTER:
		return m.GetCounter().GetValue()
	case dto.MetricType_GAUGE:
		return m.GetGauge().GetValue()
	case dto.MetricType_HISTOGRAM:
		return float64(m.GetHistogram().GetSampleCount())
	default:
		return -1
	}
}

// sameKey registers two independently constructed, identical collectors under one key and writes
// once through each returned handle; both writes must reach the one gathered series.
func sameKey[C prometheus.Collector](name string, newC func() C, write func(C)) func(*testing.T) {
	return func(t *testing.T) {
		r := NewMetricsRegistry()
		first, err := RegisterOrGet(r, "svc", "m", newC())
		require.NoError(t, err)
		second, err := RegisterOrGet(r, "svc", "m", newC())
		require.NoError(t, err)
		write(first)
		write(second)
		require.Equal(t, []float64{2}, gatheredSeries(t, r, name),
			"both writes through the returned handles must be gathered on the one series")
	}
}

// Test 1 (design D9).
func TestRegisterOrGetSameKeyReturnsCanonicalCollector(t *testing.T) {
	t.Run("Counter", sameKey("d9_counter_total",
		func() prometheus.Counter {
			return prometheus.NewCounter(prometheus.CounterOpts{Name: "d9_counter_total", Help: "h"})
		}, func(c prometheus.Counter) { c.Inc() }))
	t.Run("Gauge", sameKey("d9_gauge",
		func() prometheus.Gauge { return prometheus.NewGauge(prometheus.GaugeOpts{Name: "d9_gauge", Help: "h"}) },
		func(g prometheus.Gauge) { g.Inc() }))
	t.Run("Histogram", sameKey("d9_histogram",
		func() prometheus.Histogram {
			return prometheus.NewHistogram(prometheus.HistogramOpts{Name: "d9_histogram", Help: "h"})
		}, func(h prometheus.Histogram) { h.Observe(1) }))
	t.Run("CounterVec", sameKey("d9_countervec_total",
		func() *prometheus.CounterVec {
			return prometheus.NewCounterVec(prometheus.CounterOpts{Name: "d9_countervec_total", Help: "h"}, []string{"l"})
		}, func(c *prometheus.CounterVec) { c.WithLabelValues("x").Inc() }))
	t.Run("GaugeVec", sameKey("d9_gaugevec",
		func() *prometheus.GaugeVec {
			return prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "d9_gaugevec", Help: "h"}, []string{"l"})
		}, func(g *prometheus.GaugeVec) { g.WithLabelValues("x").Inc() }))
	t.Run("HistogramVec", sameKey("d9_histogramvec",
		func() *prometheus.HistogramVec {
			return prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "d9_histogramvec", Help: "h"}, []string{"l"})
		}, func(h *prometheus.HistogramVec) { h.WithLabelValues("x").Observe(1) }))
}

// Test 2 (design D9): a gauge satisfies prometheus.Counter, so the type check is on the exact
// concrete type; same name and help give the two the same descriptor.
func TestRegisterOrGetRefusesSameKeyOfAnotherType(t *testing.T) {
	r := NewMetricsRegistry()
	gauge, err := RegisterOrGet(r, "svc", "m",
		prometheus.NewGauge(prometheus.GaugeOpts{Name: "d9_kind", Help: "h"}))
	require.NoError(t, err)
	gauge.Set(5)
	counter, err := RegisterOrGet[prometheus.Counter](r, "svc", "m",
		prometheus.NewCounter(prometheus.CounterOpts{Name: "d9_kind", Help: "h"}))
	require.Error(t, err, "a counter under a gauge's key must be refused")
	require.True(t, errs.IsFatal(err))
	require.Nil(t, counter, "a refusal returns the zero collector")
	require.Equal(t, []float64{5}, gatheredSeries(t, r, "d9_kind"), "the canonical gauge is untouched")
}

// Test 3 (design D9).
func TestRegisterOrGetRefusesSameKeyWithDifferentHelp(t *testing.T) {
	r := NewMetricsRegistry()
	first, err := RegisterOrGet(r, "svc", "m",
		prometheus.NewCounter(prometheus.CounterOpts{Name: "d9_help_total", Help: "first"}))
	require.NoError(t, err)
	first.Inc()
	other, err := RegisterOrGet(r, "svc", "m",
		prometheus.NewCounter(prometheus.CounterOpts{Name: "d9_help_total", Help: "second"}))
	require.Error(t, err, "a different help text is a different descriptor")
	require.True(t, errs.IsFatal(err))
	require.Nil(t, other)
	require.Equal(t, []float64{1}, gatheredSeries(t, r, "d9_help_total"))
}

// A held key met by a candidate of the same concrete type, name and help but other label names is
// refused (the metric-registry delta, task 1.1). TestPropRegisterOrGetHistory cannot reach this
// case: its generator fixes the label names by kind.
func TestRegisterOrGetRefusesSameKeyWithDifferentLabelNames(t *testing.T) {
	r := NewMetricsRegistry()
	first, err := RegisterOrGet(r, "svc", "k",
		prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "d9_labels", Help: "labels"}, []string{"a"}))
	require.NoError(t, err)
	first.WithLabelValues("x").Set(4)
	other, err := RegisterOrGet(r, "svc", "k",
		prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "d9_labels", Help: "labels"}, []string{"b"}))
	require.Error(t, err, "other label names are a different descriptor")
	require.True(t, errs.IsFatal(err))
	require.ErrorContains(t, err, `logical metric key "svc.k" has a different descriptor`)
	require.Nil(t, other, "a refusal returns the zero collector")
	require.Equal(t, []float64{4}, gatheredSeries(t, r, "d9_labels"), "the canonical gauge is untouched")
}

// Test 4 (design D9).
func TestRegisterOrGetRefusesNilCandidates(t *testing.T) {
	r := NewMetricsRegistry()
	var typedNil *prometheus.GaugeVec
	require.NotPanics(t, func() {
		got, err := RegisterOrGet(r, "svc", "typed-nil", typedNil)
		require.Error(t, err, "a typed nil *GaugeVec")
		require.True(t, errs.IsFatal(err))
		require.Nil(t, got)
	})
	var nilCounter prometheus.Counter
	require.NotPanics(t, func() {
		got, err := RegisterOrGet(r, "svc", "nil-interface", nilCounter)
		require.Error(t, err, "a nil Counter interface")
		require.True(t, errs.IsFatal(err))
		require.Nil(t, got)
	})
	require.False(t, r.Unregister("svc", "typed-nil"), "nothing is stored for a refused candidate")
	require.False(t, r.Unregister("svc", "nil-interface"), "nothing is stored for a refused candidate")
}

// Test 5 (design D9): the same descriptor under a second key.
func TestRegisterOrGetRefusesCrossKeyAlias(t *testing.T) {
	r := NewMetricsRegistry()
	a, err := RegisterOrGet(r, "svc", "k1",
		prometheus.NewCounter(prometheus.CounterOpts{Name: "d9_alias_total", Help: "h"}))
	require.NoError(t, err)
	b, err := RegisterOrGet(r, "svc", "k2",
		prometheus.NewCounter(prometheus.CounterOpts{Name: "d9_alias_total", Help: "h"}))
	require.Error(t, err, "a descriptor already owned by another key must be refused")
	require.True(t, errs.IsFatal(err))
	require.Nil(t, b)
	require.False(t, r.Unregister("svc", "k2"), "nothing was stored under the alias key")
	a.Inc()
	require.Equal(t, []float64{1}, gatheredSeries(t, r, "d9_alias_total"), "k1's series survives, with k1's write")
}

// Test 6 (design D9): a core metric owns its descriptor.
func TestRegisterOrGetRefusesCoreMetricCollision(t *testing.T) {
	r := NewMetricsRegistry()
	r.CoreMetrics().ServiceStatus.WithLabelValues("core").Set(2)
	alias, err := RegisterOrGet(r, "svc", "status", prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "semstreams", Subsystem: "service", Name: "status",
		Help: "Service status (0=stopped, 1=starting, 2=running, 3=stopping, 4=failed)",
	}, []string{"service"}))
	require.Error(t, err)
	require.True(t, errs.IsFatal(err))
	require.Nil(t, alias)
	require.False(t, r.Unregister("svc", "status"))
	require.Equal(t, []float64{2}, gatheredSeries(t, r, "semstreams_service_status"))
}

// Test 7 (design D9): a collector registered directly through PrometheusRegistry owns its
// descriptor too.
func TestRegisterOrGetRefusesDirectRegistrationCollision(t *testing.T) {
	r := NewMetricsRegistry()
	direct := prometheus.NewCounter(prometheus.CounterOpts{Name: "d9_direct_total", Help: "h"})
	r.PrometheusRegistry().MustRegister(direct)
	direct.Inc()
	alias, err := RegisterOrGet(r, "svc", "direct",
		prometheus.NewCounter(prometheus.CounterOpts{Name: "d9_direct_total", Help: "h"}))
	require.Error(t, err)
	require.True(t, errs.IsFatal(err))
	require.Nil(t, alias)
	require.False(t, r.Unregister("svc", "direct"))
	require.Equal(t, []float64{1}, gatheredSeries(t, r, "d9_direct_total"))
}

// Test 8 (design D9), generalising the pin's registry_test.go:188: concurrent registrations of
// one key all receive the one canonical collector, and every write through it is gathered.
func TestRegisterOrGetConcurrentCallersShareOneCollector(t *testing.T) {
	r := NewMetricsRegistry()
	const workers = 24
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]prometheus.Counter, workers)
	failures := make([]error, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], failures[i] = RegisterOrGet(r, "svc", "concurrent",
				prometheus.NewCounter(prometheus.CounterOpts{Name: "d9_concurrent_total", Help: "h"}))
			if failures[i] == nil {
				results[i].Inc()
			}
		}()
	}
	close(start)
	wg.Wait()
	for i := range workers {
		require.NoError(t, failures[i])
		require.Same(t, results[0], results[i], "worker %d got a collector other than the canonical one", i)
	}
	require.Equal(t, []float64{workers}, gatheredSeries(t, r, "d9_concurrent_total"))
}
