package cache

import (
	"github.com/c360studio/semengine/metric"
)

// Option configures cache behavior using the functional options pattern.
// This provides a clean, extensible API for configuring caches.
type Option[V any] func(*cacheOptions[V])

// cacheOptions holds internal configuration for cache instances.
// Stats are ALWAYS collected - they are not optional.
// Metrics are optional and exposed via WithMetrics().
type cacheOptions[V any] struct {
	// metricsReg is optional - if provided, cache stats are also exposed as Prometheus metrics
	metricsReg *metric.MetricsRegistry

	// metricsPrefix is used as the component label for Prometheus metrics
	metricsPrefix string
}

// WithMetrics enables Prometheus metrics export for cache statistics.
// If registry is nil, this option is ignored.
// Registry should not be nil in normal usage - this handles edge cases gracefully.
func WithMetrics[V any](registry *metric.MetricsRegistry, prefix string) Option[V] {
	return func(opts *cacheOptions[V]) {
		if registry != nil && prefix != "" {
			opts.metricsReg = registry
			opts.metricsPrefix = prefix
		}
	}
}

// applyOptions applies functional options to create final cache configuration.
// This is an internal helper used by cache constructors.
func applyOptions[V any](options ...Option[V]) *cacheOptions[V] {
	opts := &cacheOptions[V]{}

	for _, opt := range options {
		if opt != nil {
			opt(opts)
		}
	}

	return opts
}
