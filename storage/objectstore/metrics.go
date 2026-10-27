package objectstore

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semengine/metric"
)

// metricService is the service half of every metric key the Store registers.
const metricService = "objectstore"

// storeMetrics holds the collectors one Store records to. Every Store on a
// registry shares them; the bucket label tells the Stores' buckets apart. A nil
// *storeMetrics records nothing.
type storeMetrics struct {
	bucket string

	readOps   *prometheus.CounterVec // bucket, operation (get, open)
	writeOps  *prometheus.CounterVec // bucket, operation (put, store_content)
	deleteOps *prometheus.CounterVec // bucket
	listOps   *prometheus.CounterVec // bucket

	readLatency   *prometheus.HistogramVec // bucket, operation
	writeLatency  *prometheus.HistogramVec // bucket, operation
	deleteLatency *prometheus.HistogramVec // bucket
	listLatency   *prometheus.HistogramVec // bucket

	// errors counts each call that returned an error, by operation. The Store
	// retries nothing, so a count is an outcome.
	errors *prometheus.CounterVec // bucket, operation
}

var (
	opLatencyBuckets   = []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 2.0}
	listLatencyBuckets = []float64{0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0}
)

func newCounterVec(name, help string, labels ...string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine", Subsystem: metricService, Name: name, Help: help,
	}, append([]string{"bucket"}, labels...))
}

func newHistogramVec(name, help string, buckets []float64, labels ...string) *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "semengine", Subsystem: metricService, Name: name, Help: help, Buckets: buckets,
	}, append([]string{"bucket"}, labels...))
}

// newStoreMetrics registers the Store's collectors on registry through
// metric.RegisterOrGet and keeps the collectors it returns, so a second Store on
// the same registry records to the first one's. With a nil registry it returns
// nil and nothing is registered anywhere.
func newStoreMetrics(registry *metric.MetricsRegistry, bucket string) (*storeMetrics, error) {
	if registry == nil {
		return nil, nil
	}
	m := &storeMetrics{bucket: bucket}
	counters := []struct {
		dst  **prometheus.CounterVec
		name string
		vec  *prometheus.CounterVec
	}{
		{&m.readOps, "read_operations_total", newCounterVec("read_operations_total", "Read operations by bucket and operation.", "operation")},
		{&m.writeOps, "write_operations_total", newCounterVec("write_operations_total", "Write operations by bucket and operation.", "operation")},
		{&m.deleteOps, "delete_operations_total", newCounterVec("delete_operations_total", "Delete operations by bucket.")},
		{&m.listOps, "list_operations_total", newCounterVec("list_operations_total", "List operations by bucket.")},
		{&m.errors, "operation_errors_total", newCounterVec("operation_errors_total", "Calls that returned an error, by bucket and operation.", "operation")},
	}
	for _, c := range counters {
		got, err := metric.RegisterOrGet(registry, metricService, c.name, c.vec)
		if err != nil {
			return nil, err
		}
		*c.dst = got
	}
	histograms := []struct {
		dst  **prometheus.HistogramVec
		name string
		vec  *prometheus.HistogramVec
	}{
		{&m.readLatency, "read_duration_seconds", newHistogramVec("read_duration_seconds", "Successful read duration in seconds.", opLatencyBuckets, "operation")},
		{&m.writeLatency, "write_duration_seconds", newHistogramVec("write_duration_seconds", "Successful write duration in seconds.", opLatencyBuckets, "operation")},
		{&m.deleteLatency, "delete_duration_seconds", newHistogramVec("delete_duration_seconds", "Successful delete duration in seconds.", opLatencyBuckets)},
		{&m.listLatency, "list_duration_seconds", newHistogramVec("list_duration_seconds", "Successful list duration in seconds.", listLatencyBuckets)},
	}
	for _, h := range histograms {
		got, err := metric.RegisterOrGet(registry, metricService, h.name, h.vec)
		if err != nil {
			return nil, err
		}
		*h.dst = got
	}
	return m, nil
}

func (m *storeMetrics) recordReadOp(operation string) {
	if m != nil {
		m.readOps.WithLabelValues(m.bucket, operation).Inc()
	}
}

func (m *storeMetrics) recordWriteOp(operation string) {
	if m != nil {
		m.writeOps.WithLabelValues(m.bucket, operation).Inc()
	}
}

func (m *storeMetrics) recordDeleteOp() {
	if m != nil {
		m.deleteOps.WithLabelValues(m.bucket).Inc()
	}
}

func (m *storeMetrics) recordListOp() {
	if m != nil {
		m.listOps.WithLabelValues(m.bucket).Inc()
	}
}

func (m *storeMetrics) recordReadLatency(operation string, start time.Time) {
	if m != nil {
		m.readLatency.WithLabelValues(m.bucket, operation).Observe(time.Since(start).Seconds())
	}
}

func (m *storeMetrics) recordWriteLatency(operation string, start time.Time) {
	if m != nil {
		m.writeLatency.WithLabelValues(m.bucket, operation).Observe(time.Since(start).Seconds())
	}
}

func (m *storeMetrics) recordDeleteLatency(start time.Time) {
	if m != nil {
		m.deleteLatency.WithLabelValues(m.bucket).Observe(time.Since(start).Seconds())
	}
}

func (m *storeMetrics) recordListLatency(start time.Time) {
	if m != nil {
		m.listLatency.WithLabelValues(m.bucket).Observe(time.Since(start).Seconds())
	}
}

func (m *storeMetrics) recordError(operation string) {
	if m != nil {
		m.errors.WithLabelValues(m.bucket, operation).Inc()
	}
}
