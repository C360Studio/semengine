package metric

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetricsRegistry(t *testing.T) {
	registry := NewMetricsRegistry()

	assert.NotNil(t, registry)
	assert.NotNil(t, registry.PrometheusRegistry())
}

func TestMetricsRegistry_UnregisterMetric(t *testing.T) {
	registry := NewMetricsRegistry()

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "unregister_counter",
		Help: "A counter to unregister",
	})

	// Register the counter
	_, err := RegisterOrGet(registry, "test-service", "unregister_counter", counter)
	require.NoError(t, err)

	// Verify it's registered
	metricFamilies, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "unregister_counter" {
			found = true
			break
		}
	}
	assert.True(t, found)

	// Unregister the counter
	success := registry.Unregister("test-service", "unregister_counter")
	assert.True(t, success)

	// Verify it's no longer registered
	metricFamilies, err = registry.PrometheusRegistry().Gather()
	require.NoError(t, err)

	found = false
	for _, mf := range metricFamilies {
		if mf.GetName() == "unregister_counter" {
			found = true
			break
		}
	}
	assert.False(t, found)
}

func TestMetricsRegistry_ThreadSafety(t *testing.T) {
	registry := NewMetricsRegistry()

	var wg sync.WaitGroup
	numGoroutines := 10

	// Register metrics concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			counter := prometheus.NewCounter(prometheus.CounterOpts{
				Name: fmt.Sprintf("concurrent_counter_%d", id),
				Help: "A concurrent counter",
			})

			_, err := RegisterOrGet(registry, "concurrent-service",
				fmt.Sprintf("concurrent_counter_%d", id), counter)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify all metrics were registered
	metricFamilies, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)

	counterCount := 0
	for _, mf := range metricFamilies {
		if contains(mf.GetName(), "concurrent_counter_") {
			counterCount++
		}
	}

	assert.Equal(t, numGoroutines, counterCount,
		"All concurrent counters should be registered")
}

func TestMetricsRegistry_CoreMetricsInitialization(t *testing.T) {
	registry := NewMetricsRegistry()

	// Vector metrics don't appear in Gather() until they have at least one value set
	// So we need to use the core metrics to set some values first
	coreMetrics := registry.CoreMetrics()

	// Set some values so the metrics show up in Gather()
	coreMetrics.RecordServiceStatus("test-service", 2)
	coreMetrics.RecordMessageReceived("test-service", "drifter")
	coreMetrics.RecordMessageProcessed("test-service", "drifter", "success")
	coreMetrics.RecordMessagePublished("test-service", "ocean.data")
	coreMetrics.RecordProcessingDuration("test-service", "read", 100*time.Millisecond)
	coreMetrics.RecordError("test-service", "connection")
	coreMetrics.RecordHealthStatus("test-service", true)

	// Verify core platform metrics are initialized
	metricFamilies, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)

	expectedCoreMetrics := []string{
		"semengine_service_status",
		"semengine_messages_received_total",
		"semengine_messages_processed_total",
		"semengine_messages_published_total",
		"semengine_processing_duration_seconds",
		"semengine_errors_total",
		"semengine_health_status",
		"semengine_nats_connected",
		"semengine_nats_rtt_seconds",
		"semengine_nats_reconnects_total",
		"semengine_nats_circuit_breaker",
	}

	foundMetrics := make(map[string]bool)
	for _, mf := range metricFamilies {
		foundMetrics[mf.GetName()] = true
	}

	for _, expectedMetric := range expectedCoreMetrics {
		assert.True(t, foundMetrics[expectedMetric],
			"core metric %s should be initialized", expectedMetric)
	}
}

func TestMetricsRegistry_NoCoreBusinessMetrics(t *testing.T) {
	registry := NewMetricsRegistry()

	metricFamilies, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)

	// These business metrics should NOT be in core registry
	businessMetrics := []string{
		"semengine_business_drifters_tracked",
		"semengine_business_convergence_zones_total",
		"semengine_business_files_processed_total",
		"semengine_business_catalog_size",
	}

	foundMetrics := make(map[string]bool)
	for _, mf := range metricFamilies {
		foundMetrics[mf.GetName()] = true
	}

	for _, businessMetric := range businessMetrics {
		assert.False(t, foundMetrics[businessMetric],
			"Business metric %s should NOT be in core registry", businessMetric)
	}
}

func TestMetricsRegistry_GetCoreMetrics(t *testing.T) {
	registry := NewMetricsRegistry()

	coreMetrics := registry.CoreMetrics()
	assert.NotNil(t, coreMetrics)

	// Verify core metrics are accessible
	assert.NotNil(t, coreMetrics.ServiceStatus)
	assert.NotNil(t, coreMetrics.MessagesReceived)
	assert.NotNil(t, coreMetrics.MessagesProcessed)
	assert.NotNil(t, coreMetrics.MessagesPublished)
	assert.NotNil(t, coreMetrics.ProcessingDuration)
	assert.NotNil(t, coreMetrics.ErrorsTotal)
	assert.NotNil(t, coreMetrics.HealthCheckStatus)
	assert.NotNil(t, coreMetrics.NATSConnected)
	assert.NotNil(t, coreMetrics.NATSRTT)
	assert.NotNil(t, coreMetrics.NATSReconnects)
	assert.NotNil(t, coreMetrics.NATSCircuitBreaker)
}

func TestCoreMetrics_RecordMethods(t *testing.T) {
	registry := NewMetricsRegistry()
	coreMetrics := registry.CoreMetrics()

	// Test service status recording
	coreMetrics.RecordServiceStatus("test-service", 2)

	// Test message recording
	coreMetrics.RecordMessageReceived("test-service", "drifter")
	coreMetrics.RecordMessageProcessed("test-service", "drifter", "success")
	coreMetrics.RecordMessagePublished("test-service", "ocean.data")

	// Test processing duration
	coreMetrics.RecordProcessingDuration("test-service", "read", 100*time.Millisecond)

	// Test error recording
	coreMetrics.RecordError("test-service", "connection")

	// Test health status
	coreMetrics.RecordHealthStatus("test-service", true)

	// Test NATS metrics
	coreMetrics.RecordNATSStatus(true)
	coreMetrics.RecordNATSRTT(50 * time.Millisecond)
	coreMetrics.RecordNATSReconnect()
	coreMetrics.RecordCircuitBreakerState(0)

	// Verify metrics have values > 0
	metricFamilies, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)

	// Check that we have metrics data
	assert.Greater(t, len(metricFamilies), 0, "Should have recorded metrics")
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}
