package cache

import (
	"testing"

	"github.com/c360studio/semengine/metric"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheMetricsIntegration(t *testing.T) {
	// Create metrics registry
	metricsRegistry := metric.NewMetricsRegistry()

	// Create cache with metrics enabled
	cache, err := NewLRU[string](10, WithMetrics[string](metricsRegistry, "test_cache"))
	require.NoError(t, err)

	// Perform cache operations
	_, _ = cache.Set("key1", "value1")
	_, _ = cache.Set("key2", "value2")

	// Access key1 (hit)
	val, found := cache.Get("key1")
	assert.True(t, found)
	assert.Equal(t, "value1", val)

	// Access non-existent key (miss)
	_, found = cache.Get("key3")
	assert.False(t, found)

	// Delete a key
	deleted, _ := cache.Delete("key2")
	assert.True(t, deleted)

	// Gather metrics from registry
	metricFamilies, err := metricsRegistry.PrometheusRegistry().Gather()
	require.NoError(t, err)

	// Verify cache metrics exist and have correct values
	metricsByName := make(map[string]*dto.MetricFamily)
	for _, mf := range metricFamilies {
		metricsByName[*mf.Name] = mf
	}

	// Check hits metric
	hitsMetric := metricsByName["semengine_cache_hits_total"]
	require.NotNil(t, hitsMetric, "hits metric should exist")
	assert.Equal(t, float64(1), *hitsMetric.Metric[0].Counter.Value, "should have 1 hit")

	// Check misses metric
	missesMetric := metricsByName["semengine_cache_misses_total"]
	require.NotNil(t, missesMetric, "misses metric should exist")
	assert.Equal(t, float64(1), *missesMetric.Metric[0].Counter.Value, "should have 1 miss")

	// Check sets metric
	setsMetric := metricsByName["semengine_cache_sets_total"]
	require.NotNil(t, setsMetric, "sets metric should exist")
	assert.Equal(t, float64(2), *setsMetric.Metric[0].Counter.Value, "should have 2 sets")

	// Check deletes metric
	deletesMetric := metricsByName["semengine_cache_deletes_total"]
	require.NotNil(t, deletesMetric, "deletes metric should exist")
	assert.Equal(t, float64(1), *deletesMetric.Metric[0].Counter.Value, "should have 1 delete")

	// Check size metric
	sizeMetric := metricsByName["semengine_cache_size"]
	require.NotNil(t, sizeMetric, "size metric should exist")
	assert.Equal(t, float64(1), *sizeMetric.Metric[0].Gauge.Value, "should have 1 item remaining")

	// Check component label
	assert.Equal(t, "test_cache", *hitsMetric.Metric[0].Label[0].Value, "should have correct component label")
}

func TestCacheWithoutMetrics(t *testing.T) {
	// Create cache without metrics registry
	cache, err := NewLRU[string](10)
	require.NoError(t, err)

	// Perform cache operations
	_, _ = cache.Set("key1", "value1")
	val, found := cache.Get("key1")
	assert.True(t, found)
	assert.Equal(t, "value1", val)

	// Should work without errors even though no metrics are configured
}

func TestCachePreferMetricsOverStats(t *testing.T) {
	// Create metrics registry
	metricsRegistry := metric.NewMetricsRegistry()

	// Create cache with both metrics and stats enabled
	// Note: EnableStats is deprecated and ignored - stats are always enabled
	// Only metrics need to be explicitly enabled

	cache, err := NewLRU[string](10, WithMetrics[string](metricsRegistry, "test_cache"))
	require.NoError(t, err)
	lruCache := cache.(*lruCache[string])

	// Both metrics and stats should be enabled (stats are always on)
	assert.NotNil(t, lruCache.metrics, "metrics should be enabled")
	assert.NotNil(t, lruCache.stats, "stats should always be enabled")
}

// TestCacheMetricsTwoCachesOnePrefixGatherTheSum holds design D9 for pkg/cache: two caches built
// with the same prefix on one registry share one collector per metric, so every operation either
// cache performs is gathered. The oracle is the operations this test performs, counted by hand:
// each cache has room for one entry and does two sets (one eviction), one hit, one miss and one
// delete. At the pin the second cache's registrations returned nil without registering its
// collectors, so its writes were never gathered.
func TestCacheMetricsTwoCachesOnePrefixGatherTheSum(t *testing.T) {
	registry := metric.NewMetricsRegistry()

	for range 2 {
		c, err := NewLRU[string](1, WithMetrics[string](registry, "shared"))
		require.NoError(t, err)
		_, _ = c.Set("a", "1")
		_, _ = c.Set("b", "2") // evicts a
		_, found := c.Get("b")
		require.True(t, found)
		_, found = c.Get("a")
		require.False(t, found)
		deleted, err := c.Delete("b")
		require.NoError(t, err)
		require.True(t, deleted)
	}

	families, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)
	byName := make(map[string]*dto.MetricFamily)
	for _, mf := range families {
		byName[mf.GetName()] = mf
	}
	want := map[string]float64{
		"semengine_cache_sets_total":      4,
		"semengine_cache_evictions_total": 2,
		"semengine_cache_hits_total":      2,
		"semengine_cache_misses_total":    2,
		"semengine_cache_deletes_total":   2,
	}
	for name, value := range want {
		mf := byName[name]
		require.NotNil(t, mf, "%s gathered", name)
		require.Len(t, mf.GetMetric(), 1, "%s has one series", name)
		assert.Equal(t, value, mf.GetMetric()[0].GetCounter().GetValue(), name)
	}
}
