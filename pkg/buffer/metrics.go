package buffer

import (
	"github.com/c360studio/semengine/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// bufferMetrics holds Prometheus metrics for buffer operations.
type bufferMetrics struct {
	// Counter metrics - directly incremented without stats duplication
	writes    prometheus.Counter
	reads     prometheus.Counter
	peeks     prometheus.Counter
	overflows prometheus.Counter
	drops     prometheus.Counter

	// Gauge metrics - updated on operations
	size        prometheus.Gauge
	utilization prometheus.Gauge
}

// newBufferMetrics creates and registers buffer metrics with the provided registry.
func newBufferMetrics(registry *metric.MetricsRegistry, prefix string) (*bufferMetrics, error) {
	m := &bufferMetrics{
		writes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   "semengine",
			Subsystem:   "buffer",
			Name:        "writes_total",
			ConstLabels: prometheus.Labels{"component": prefix},
			Help:        "Total number of buffer write operations",
		}),
		reads: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   "semengine",
			Subsystem:   "buffer",
			Name:        "reads_total",
			ConstLabels: prometheus.Labels{"component": prefix},
			Help:        "Total number of buffer read operations",
		}),
		peeks: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   "semengine",
			Subsystem:   "buffer",
			Name:        "peeks_total",
			ConstLabels: prometheus.Labels{"component": prefix},
			Help:        "Total number of buffer peek operations",
		}),
		overflows: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   "semengine",
			Subsystem:   "buffer",
			Name:        "overflows_total",
			ConstLabels: prometheus.Labels{"component": prefix},
			Help:        "Total number of buffer overflow events",
		}),
		drops: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   "semengine",
			Subsystem:   "buffer",
			Name:        "drops_total",
			ConstLabels: prometheus.Labels{"component": prefix},
			Help:        "Total number of items dropped due to overflow",
		}),
		size: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   "semengine",
			Subsystem:   "buffer",
			Name:        "size",
			ConstLabels: prometheus.Labels{"component": prefix},
			Help:        "Current number of items in buffer",
		}),
		utilization: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   "semengine",
			Subsystem:   "buffer",
			Name:        "utilization",
			ConstLabels: prometheus.Labels{"component": prefix},
			Help:        "Buffer utilization as a percentage (0.0 to 1.0)",
		}),
	}

	// Register through RegisterOrGet and keep the collectors it returns: a second buffer under
	// the same prefix shares the first buffer's collectors, so both buffers' writes are gathered.
	// The collectors are interface-typed; RegisterOrGet compares the concrete type.
	var err error
	if m.writes, err = metric.RegisterOrGet(registry, prefix, "buffer_writes", m.writes); err != nil {
		return nil, err
	}
	if m.reads, err = metric.RegisterOrGet(registry, prefix, "buffer_reads", m.reads); err != nil {
		return nil, err
	}
	if m.peeks, err = metric.RegisterOrGet(registry, prefix, "buffer_peeks", m.peeks); err != nil {
		return nil, err
	}
	if m.overflows, err = metric.RegisterOrGet(registry, prefix, "buffer_overflows", m.overflows); err != nil {
		return nil, err
	}
	if m.drops, err = metric.RegisterOrGet(registry, prefix, "buffer_drops", m.drops); err != nil {
		return nil, err
	}
	if m.size, err = metric.RegisterOrGet(registry, prefix, "buffer_size", m.size); err != nil {
		return nil, err
	}
	if m.utilization, err = metric.RegisterOrGet(registry, prefix, "buffer_utilization", m.utilization); err != nil {
		return nil, err
	}

	return m, nil
}

// recordWrite increments the write counter and updates size/utilization.
func (m *bufferMetrics) recordWrite(size, capacity int) {
	m.writes.Inc()
	m.size.Set(float64(size))
	m.utilization.Set(float64(size) / float64(capacity))
}

// recordRead increments the read counter and updates size/utilization.
func (m *bufferMetrics) recordRead(size, capacity int) {
	m.reads.Inc()
	m.size.Set(float64(size))
	m.utilization.Set(float64(size) / float64(capacity))
}

// recordReads adds n to the read counter and updates size/utilization once.
func (m *bufferMetrics) recordReads(n, size, capacity int) {
	m.reads.Add(float64(n))
	m.size.Set(float64(size))
	m.utilization.Set(float64(size) / float64(capacity))
}

// recordPeek increments the peek counter.
func (m *bufferMetrics) recordPeek() {
	m.peeks.Inc()
}

// recordOverflow increments the overflow counter.
func (m *bufferMetrics) recordOverflow() {
	m.overflows.Inc()
}

// recordDrop increments the drop counter.
func (m *bufferMetrics) recordDrop() {
	m.drops.Inc()
}

// updateSize sets the current buffer size and utilization.
func (m *bufferMetrics) updateSize(size, capacity int) {
	m.size.Set(float64(size))
	m.utilization.Set(float64(size) / float64(capacity))
}
