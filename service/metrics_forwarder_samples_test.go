package service

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
)

// The metrics forwarder's mapping from gathered families to published samples (design D4,
// U-d): the pin's mapping tests (metrics_forwarder_test.go:273-607, :849-969) as tables over
// metricSamples. The pin drove each through a started forwarder, a mock publisher and a
// sleep; the mapping is one plain function here, so each case is a call. Expected values are
// the pin's behaviour written out as literals, never computed by the code under test.
//
// Named examples suffice, no generated checks: each sample maps on its own, and each case is
// one input class the pin distinguishes (family type, label set, name prefix and flags).

// samplesNow is the time every case passes; the timestamp a sample carries is its UnixMilli.
var samplesNow = time.Date(2026, 10, 23, 12, 30, 45, 678_000_000, time.UTC)

const samplesNowMillis int64 = 1_792_758_645_678

func samplesFamily(name string, typ *dto.MetricType, metrics ...*dto.Metric) *dto.MetricFamily {
	return &dto.MetricFamily{Name: &name, Type: typ, Metric: metrics}
}

func samplesType(typ dto.MetricType) *dto.MetricType { return &typ }

func samplesFloat(v float64) *float64 { return &v }

func samplesCount(v uint64) *uint64 { return &v }

// samplesLabels builds label pairs from name, value, name, value, ...
func samplesLabels(pairs ...string) []*dto.LabelPair {
	labels := make([]*dto.LabelPair, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		name, value := pairs[i], pairs[i+1]
		labels = append(labels, &dto.LabelPair{Name: &name, Value: &value})
	}
	return labels
}

func samplesNamed(t *testing.T, samples []metricSample, name string) metricSample {
	t.Helper()
	var found []metricSample
	for _, s := range samples {
		if s.Name == name {
			found = append(found, s)
		}
	}
	require.Len(t, found, 1, "samples named %s", name)
	return found[0]
}

// TestMetricSamplesMapsWhatTheRegistryGathers is the pin's GatherMetrics (:273-331): a
// counter, a gauge and a histogram registered on a metrics registry and gathered the way the
// forwarder gathers, each mapped to one sample on metrics.system.<name>. The histogram's
// value is its sample count (2), not its sum (0.75).
func TestMetricSamplesMapsWhatTheRegistryGathers(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_messages_total", Help: "Test counter"})
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_active_connections", Help: "Test gauge"})
	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "test_request_duration_seconds", Help: "Test histogram",
	})
	registry.PrometheusRegistry().MustRegister(counter, gauge, histogram)
	counter.Inc()
	gauge.Set(42)
	histogram.Observe(0.5)
	histogram.Observe(0.25)

	families, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)
	samples := metricSamples(families, false, false, samplesNow)

	empty := map[string]string{}
	require.Equal(t, metricSample{
		Subject: "metrics.system.test_messages_total", Timestamp: samplesNowMillis,
		Name: "test_messages_total", Component: "system", Type: "counter", Value: 1, Labels: empty,
	}, samplesNamed(t, samples, "test_messages_total"))
	require.Equal(t, metricSample{
		Subject: "metrics.system.test_active_connections", Timestamp: samplesNowMillis,
		Name: "test_active_connections", Component: "system", Type: "gauge", Value: 42, Labels: empty,
	}, samplesNamed(t, samples, "test_active_connections"))
	require.Equal(t, metricSample{
		Subject: "metrics.system.test_request_duration_seconds", Timestamp: samplesNowMillis,
		Name: "test_request_duration_seconds", Component: "system", Type: "histogram", Value: 2, Labels: empty,
	}, samplesNamed(t, samples, "test_request_duration_seconds"))
}

// TestMetricSamplesCarryThePayloadFields is the pin's MessageFormat (:394-462): a counter
// labelled component=test-input and status=success, added 123, gathered from a registry.
// Every payload field is set, and labels carry every label, component included.
func TestMetricSamplesCarryThePayloadFields(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_messages_total", Help: "Test counter"},
		[]string{"component", "status"},
	)
	registry.PrometheusRegistry().MustRegister(counter)
	counter.With(prometheus.Labels{"component": "test-input", "status": "success"}).Add(123)

	families, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)

	require.Equal(t, metricSample{
		Subject:   "metrics.test-input.test_messages_total",
		Timestamp: samplesNowMillis,
		Name:      "test_messages_total",
		Component: "test-input",
		Type:      "counter",
		Value:     123,
		Labels:    map[string]string{"component": "test-input", "status": "success"},
	}, samplesNamed(t, metricSamples(families, false, false, samplesNow), "test_messages_total"))
}

// TestMetricSamplesSubjectNamesTheComponent is the pin's SubjectPattern (:333-392) and
// ExtractComponent (:464-606): the subject is metrics.<component>.<name>, the component the
// sample's component label, else system.
func TestMetricSamplesSubjectNamesTheComponent(t *testing.T) {
	tests := []struct {
		name          string
		metricName    string
		labels        []string
		wantSubject   string
		wantComponent string
		wantLabels    map[string]string
	}{
		{
			name: "component label", metricName: "messages_published_total",
			labels:      []string{"component", "udp-input"},
			wantSubject: "metrics.udp-input.messages_published_total", wantComponent: "udp-input",
			wantLabels: map[string]string{"component": "udp-input"},
		},
		{
			name: "another component", metricName: "processing_duration_seconds",
			labels:      []string{"component", "graph-processor"},
			wantSubject: "metrics.graph-processor.processing_duration_seconds", wantComponent: "graph-processor",
			wantLabels: map[string]string{"component": "graph-processor"},
		},
		{
			name: "component among other labels", metricName: "test_extract_component_metric",
			labels:      []string{"status", "active", "component", "processor"},
			wantSubject: "metrics.processor.test_extract_component_metric", wantComponent: "processor",
			wantLabels: map[string]string{"status": "active", "component": "processor"},
		},
		{
			name: "no component label falls back to system", metricName: "health_status",
			labels:      []string{"status", "healthy"},
			wantSubject: "metrics.system.health_status", wantComponent: "system",
			wantLabels: map[string]string{"status": "healthy"},
		},
		{
			name: "no labels falls back to system", metricName: "active_connections",
			wantSubject: "metrics.system.active_connections", wantComponent: "system",
			wantLabels: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			families := []*dto.MetricFamily{samplesFamily(tt.metricName, samplesType(dto.MetricType_COUNTER),
				&dto.Metric{Label: samplesLabels(tt.labels...), Counter: &dto.Counter{Value: samplesFloat(1)}})}

			require.Equal(t, []metricSample{{
				Subject: tt.wantSubject, Timestamp: samplesNowMillis, Name: tt.metricName,
				Component: tt.wantComponent, Type: "counter", Value: 1, Labels: tt.wantLabels,
			}}, metricSamples(families, false, false, samplesNow))
		})
	}
}

// TestMetricSamplesValueAndTypeFollowTheFamily covers the pin's value and type mapping
// (metrics_forwarder.go:389-426), which the pin's MessageFormat checked only as one of five
// type names: a counter's, gauge's or untyped value; a histogram's or summary's sample count,
// never its sum; 0 when the metric carries no value; the family type in lower case, untyped
// when it is absent or not one of the five.
func TestMetricSamplesValueAndTypeFollowTheFamily(t *testing.T) {
	tests := []struct {
		name      string
		typ       *dto.MetricType
		metric    *dto.Metric
		wantType  string
		wantValue float64
	}{
		{
			name: "counter", typ: samplesType(dto.MetricType_COUNTER),
			metric:   &dto.Metric{Counter: &dto.Counter{Value: samplesFloat(7)}},
			wantType: "counter", wantValue: 7,
		},
		{
			name: "gauge", typ: samplesType(dto.MetricType_GAUGE),
			metric:   &dto.Metric{Gauge: &dto.Gauge{Value: samplesFloat(-2.5)}},
			wantType: "gauge", wantValue: -2.5,
		},
		{
			name: "histogram is its sample count", typ: samplesType(dto.MetricType_HISTOGRAM),
			metric:   &dto.Metric{Histogram: &dto.Histogram{SampleCount: samplesCount(3), SampleSum: samplesFloat(7.5)}},
			wantType: "histogram", wantValue: 3,
		},
		{
			name: "summary is its sample count", typ: samplesType(dto.MetricType_SUMMARY),
			metric:   &dto.Metric{Summary: &dto.Summary{SampleCount: samplesCount(4), SampleSum: samplesFloat(9)}},
			wantType: "summary", wantValue: 4,
		},
		{
			name: "untyped", typ: samplesType(dto.MetricType_UNTYPED),
			metric:   &dto.Metric{Untyped: &dto.Untyped{Value: samplesFloat(5)}},
			wantType: "untyped", wantValue: 5,
		},
		{
			name: "absent type is untyped", typ: nil,
			metric:   &dto.Metric{Gauge: &dto.Gauge{Value: samplesFloat(6)}},
			wantType: "untyped", wantValue: 6,
		},
		{
			name: "type outside the five is untyped", typ: samplesType(dto.MetricType_GAUGE_HISTOGRAM),
			metric:   &dto.Metric{Gauge: &dto.Gauge{Value: samplesFloat(8)}},
			wantType: "untyped", wantValue: 8,
		},
		{
			name: "no value is zero", typ: samplesType(dto.MetricType_COUNTER),
			metric:   &dto.Metric{Counter: &dto.Counter{}},
			wantType: "counter", wantValue: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			samples := metricSamples([]*dto.MetricFamily{samplesFamily("probe_metric", tt.typ, tt.metric)},
				false, false, samplesNow)

			require.Len(t, samples, 1)
			require.Equal(t, tt.wantType, samples[0].Type)
			require.Equal(t, tt.wantValue, samples[0].Value)
		})
	}
}

// TestMetricSamplesLeaveOutGoAndProcessFamiliesUnlessIncluded is the pin's
// SystemMetricsFiltering (:849-969), with the two cross-flag rows added: a go_ family needs
// includeGo and a process_ family needs includeProc, each flag alone; every other family is
// always mapped.
func TestMetricSamplesLeaveOutGoAndProcessFamiliesUnlessIncluded(t *testing.T) {
	tests := []struct {
		name        string
		includeGo   bool
		includeProc bool
		metricName  string
		mapped      bool
	}{
		{name: "go_ left out by default", metricName: "go_goroutines", mapped: false},
		{name: "go_ mapped when included", includeGo: true, metricName: "go_memstats_alloc_bytes", mapped: true},
		{name: "go_ not included by the process flag", includeProc: true, metricName: "go_goroutines", mapped: false},
		{name: "process_ left out by default", metricName: "process_cpu_seconds_total", mapped: false},
		{name: "process_ mapped when included", includeProc: true, metricName: "process_open_fds", mapped: true},
		{name: "process_ not included by the go flag", includeGo: true, metricName: "process_open_fds", mapped: false},
		{name: "framework metric always mapped", metricName: "semengine_service_status", mapped: true},
		{name: "other metric always mapped", metricName: "custom_app_metric", mapped: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			families := []*dto.MetricFamily{samplesFamily(tt.metricName, samplesType(dto.MetricType_GAUGE),
				&dto.Metric{Gauge: &dto.Gauge{Value: samplesFloat(42)}})}

			samples := metricSamples(families, tt.includeGo, tt.includeProc, samplesNow)

			if !tt.mapped {
				require.Empty(t, samples)
				return
			}
			require.Equal(t, []metricSample{{
				Subject: "metrics.system." + tt.metricName, Timestamp: samplesNowMillis, Name: tt.metricName,
				Component: "system", Type: "gauge", Value: 42, Labels: map[string]string{},
			}}, samples)
		})
	}
}

// TestMetricSamplesSkipUnnamedFamiliesAndIncompleteLabels covers the pin's guards
// (metrics_forwarder.go:306-308, :345-347): a nil or unnamed family maps to nothing, a label
// pair missing its name or value is left out, and samples keep the order of the families and
// of the metrics within each.
func TestMetricSamplesSkipUnnamedFamiliesAndIncompleteLabels(t *testing.T) {
	labelName, labelValue := "dangling", "value"
	counter := samplesType(dto.MetricType_COUNTER)
	families := []*dto.MetricFamily{
		nil,
		{Type: counter, Metric: []*dto.Metric{{Counter: &dto.Counter{Value: samplesFloat(1)}}}},
		samplesFamily("first_total", counter,
			&dto.Metric{
				Label: append(samplesLabels("component", "alpha"),
					&dto.LabelPair{Name: &labelName}, &dto.LabelPair{Value: &labelValue}),
				Counter: &dto.Counter{Value: samplesFloat(2)},
			},
			&dto.Metric{Label: samplesLabels("component", "beta"), Counter: &dto.Counter{Value: samplesFloat(3)}},
		),
		samplesFamily("second_total", counter, &dto.Metric{Counter: &dto.Counter{Value: samplesFloat(4)}}),
	}

	require.Equal(t, []metricSample{
		{
			Subject: "metrics.alpha.first_total", Timestamp: samplesNowMillis, Name: "first_total",
			Component: "alpha", Type: "counter", Value: 2, Labels: map[string]string{"component": "alpha"},
		},
		{
			Subject: "metrics.beta.first_total", Timestamp: samplesNowMillis, Name: "first_total",
			Component: "beta", Type: "counter", Value: 3, Labels: map[string]string{"component": "beta"},
		},
		{
			Subject: "metrics.system.second_total", Timestamp: samplesNowMillis, Name: "second_total",
			Component: "system", Type: "counter", Value: 4, Labels: map[string]string{},
		},
	}, metricSamples(families, false, false, samplesNow))
}
