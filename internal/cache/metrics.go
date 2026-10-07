package cache

import (
	"github.com/c360studio/semengine/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// cacheMetrics holds Prometheus metrics for cache operations.
type cacheMetrics struct {
	// Counter metrics - directly incremented without stats duplication
	hits      prometheus.Counter
	misses    prometheus.Counter
	sets      prometheus.Counter
	deletes   prometheus.Counter
	evictions prometheus.Counter

	// Gauge metrics - updated on operations
	size prometheus.Gauge
}

// newCacheMetrics creates and registers cache metrics with the provided registry.
func newCacheMetrics(registry *metric.MetricsRegistry, prefix string) (*cacheMetrics, error) {
	m := &cacheMetrics{
		hits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   "semengine",
			Subsystem:   "cache",
			Name:        "hits_total",
			ConstLabels: prometheus.Labels{"component": prefix},
			Help:        "Total number of cache hits",
		}),
		misses: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   "semengine",
			Subsystem:   "cache",
			Name:        "misses_total",
			ConstLabels: prometheus.Labels{"component": prefix},
			Help:        "Total number of cache misses",
		}),
		sets: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   "semengine",
			Subsystem:   "cache",
			Name:        "sets_total",
			ConstLabels: prometheus.Labels{"component": prefix},
			Help:        "Total number of cache set operations",
		}),
		deletes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   "semengine",
			Subsystem:   "cache",
			Name:        "deletes_total",
			ConstLabels: prometheus.Labels{"component": prefix},
			Help:        "Total number of cache delete operations",
		}),
		evictions: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   "semengine",
			Subsystem:   "cache",
			Name:        "evictions_total",
			ConstLabels: prometheus.Labels{"component": prefix},
			Help:        "Total number of cache evictions",
		}),
		size: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   "semengine",
			Subsystem:   "cache",
			Name:        "size",
			ConstLabels: prometheus.Labels{"component": prefix},
			Help:        "Current number of entries in cache",
		}),
	}

	// Register through RegisterOrGet and keep the collectors it returns: a second cache under
	// the same prefix shares the first cache's collectors, so both caches' writes are gathered
	// (design D9). The collectors are interface-typed; RegisterOrGet compares the concrete type.
	var err error
	if m.hits, err = metric.RegisterOrGet(registry, prefix, "cache_hits", m.hits); err != nil {
		return nil, err
	}
	if m.misses, err = metric.RegisterOrGet(registry, prefix, "cache_misses", m.misses); err != nil {
		return nil, err
	}
	if m.sets, err = metric.RegisterOrGet(registry, prefix, "cache_sets", m.sets); err != nil {
		return nil, err
	}
	if m.deletes, err = metric.RegisterOrGet(registry, prefix, "cache_deletes", m.deletes); err != nil {
		return nil, err
	}
	if m.evictions, err = metric.RegisterOrGet(registry, prefix, "cache_evictions", m.evictions); err != nil {
		return nil, err
	}
	if m.size, err = metric.RegisterOrGet(registry, prefix, "cache_size", m.size); err != nil {
		return nil, err
	}

	return m, nil
}

// recordHit increments the hit counter.
func (m *cacheMetrics) recordHit() {
	m.hits.Inc()
}

// recordMiss increments the miss counter.
func (m *cacheMetrics) recordMiss() {
	m.misses.Inc()
}

// recordSet increments the set counter.
func (m *cacheMetrics) recordSet() {
	m.sets.Inc()
}

// recordDelete increments the delete counter.
func (m *cacheMetrics) recordDelete() {
	m.deletes.Inc()
}

// recordEviction increments the eviction counter.
func (m *cacheMetrics) recordEviction() {
	m.evictions.Inc()
}

// updateSize sets the current cache size.
func (m *cacheMetrics) updateSize(size int) {
	m.size.Set(float64(size))
}
