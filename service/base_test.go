package service

import (
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
)

// The pin's base_test.go ran every test against a shared NATS client. The tests here need no NATS,
// so they are unit tests without WithNATS; the ones that need a client are in
// base_integration_test.go (design D6).

// serviceStatusGauge reads the service-status gauge the registry holds for name.
func serviceStatusGauge(registry *metric.MetricsRegistry, name string) float64 {
	return testutil.ToFloat64(registry.CoreMetrics().ServiceStatus.WithLabelValues(name))
}

// Test service creation and initialization
func TestService_Creation(t *testing.T) {
	service := NewBaseServiceWithOptions("test-service", WithMetrics(metric.NewMetricsRegistry()))

	assert.NotNil(t, service)
	assert.Equal(t, "test-service", service.Name())
	assert.Equal(t, StatusStopped, service.Status())
	assert.False(t, service.IsHealthy())
}

// Test service lifecycle. The status is also recorded on the injected registry, so WithMetrics is
// shown to be read (metric-registry spec; design D1).
func TestService_Lifecycle(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	service := NewBaseServiceWithOptions("test-service", WithMetrics(registry))
	assert.Equal(t, float64(StatusStopped), serviceStatusGauge(registry, "test-service"))

	err := service.Start(t.Context())
	require.NoError(t, err)
	assert.Equal(t, StatusRunning, service.Status())
	assert.Equal(t, float64(StatusRunning), serviceStatusGauge(registry, "test-service"))

	err = service.Stop(stopContext(t))
	require.NoError(t, err)
	assert.Equal(t, StatusStopped, service.Status())
	assert.Equal(t, float64(StatusStopped), serviceStatusGauge(registry, "test-service"))
}

// Test graceful shutdown
func TestService_GracefulShutdown(t *testing.T) {
	service := NewBaseServiceWithOptions("test-service", WithMetrics(metric.NewMetricsRegistry()))

	err := service.Start(t.Context())
	require.NoError(t, err)

	err = service.Stop(stopContext(t))
	require.NoError(t, err)
	assert.Equal(t, StatusStopped, service.Status())
}

// Test status collection
func TestService_GetStatus(t *testing.T) {
	service := NewBaseServiceWithOptions("test-service", WithMetrics(metric.NewMetricsRegistry()))

	info := service.GetStatus()
	assert.Equal(t, "test-service", info.Name)
	assert.Equal(t, StatusStopped, info.Status)
	assert.Equal(t, int64(0), info.Uptime.Milliseconds())
	assert.Equal(t, int64(0), info.MessagesProcessed)
}

// Test custom health check: the check set by SetHealthCheck runs at Start, and a passing check
// with no NATS client makes the service healthy.
func TestService_CustomHealthCheck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		service := NewBaseServiceWithOptions("test-service")

		var healthCheckCalled atomic.Int64
		service.SetHealthCheck(func() error {
			healthCheckCalled.Add(1)
			return nil
		})

		require.NoError(t, service.Start(t.Context()))
		synctest.Wait()
		assert.Equal(t, int64(1), healthCheckCalled.Load(), "custom health check should be called")
		assert.True(t, service.IsHealthy())
		assert.True(t, service.Health().Healthy)
		assert.Equal(t, int64(1), service.GetStatus().HealthChecks)

		require.NoError(t, service.Stop(stopContext(t)))
	})
}

// TestFailingHealthCheckMakesTheServiceUnhealthy is the pin's TestService_FailingHealthCheck
// without its sleep (design D2). The check then passes, so the test also shows the service
// becoming healthy again.
//
// spec: service-lifecycle / A failed health check makes the service unhealthy
func TestFailingHealthCheckMakesTheServiceUnhealthy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int64
		var failing atomic.Bool
		failing.Store(true)
		service := NewBaseServiceWithOptions("test-service")
		service.SetHealthCheck(countingCheck(&calls, &failing))

		require.NoError(t, service.Start(t.Context()))
		synctest.Wait()
		require.Equal(t, int64(1), calls.Load())
		assert.False(t, service.IsHealthy())
		health := service.Health()
		assert.False(t, health.Healthy)
		assert.Equal(t, "unhealthy", health.Status)
		assert.Equal(t, int64(1), service.GetStatus().FailedHealthChecks)

		failing.Store(false)
		<-time.After(30 * time.Second) // the health interval (spec: every 30 seconds)
		synctest.Wait()
		require.Equal(t, int64(2), calls.Load())
		assert.True(t, service.IsHealthy(), "a passing check after a failed one makes the service healthy")
		assert.True(t, service.Health().Healthy)

		require.NoError(t, service.Stop(stopContext(t)))
	})
}

// Test fresh-instance restart
func TestService_FreshInstanceRestart(t *testing.T) {
	service := NewBaseServiceWithOptions("test-service", WithMetrics(metric.NewMetricsRegistry()))

	err := service.Start(t.Context())
	require.NoError(t, err)
	assert.Equal(t, StatusRunning, service.Status())

	err = service.Stop(stopContext(t))
	require.NoError(t, err)
	assert.Equal(t, StatusStopped, service.Status())

	// The stopped instance is terminal; process composition creates a fresh one.
	err = service.Start(t.Context())
	require.Error(t, err)

	restarted := NewBaseServiceWithOptions("test-service", WithMetrics(metric.NewMetricsRegistry()))
	err = restarted.Start(t.Context())
	require.NoError(t, err)
	assert.Equal(t, StatusRunning, restarted.Status())

	err = restarted.Stop(stopContext(t))
	require.NoError(t, err)
}

// Test status transitions
func TestService_StatusTransitions(t *testing.T) {
	tests := []struct {
		name         string
		initial      Status
		action       func(*BaseService, *testing.T) error
		expectedNext Status
	}{
		{
			name:         "stopped to running",
			initial:      StatusStopped,
			action:       func(s *BaseService, t *testing.T) error { return s.Start(t.Context()) },
			expectedNext: StatusRunning,
		},
		{
			name:         "running to stopped",
			initial:      StatusRunning,
			action:       func(s *BaseService, t *testing.T) error { return s.Stop(stopContext(t)) },
			expectedNext: StatusStopped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewBaseServiceWithOptions("test-service", WithMetrics(metric.NewMetricsRegistry()))

			// Set initial state if needed
			if tt.initial == StatusRunning {
				require.NoError(t, service.Start(t.Context()))
			}

			// Perform action
			require.NoError(t, tt.action(service, t))
			assert.Equal(t, tt.expectedNext, service.Status())

			// Cleanup
			require.NoError(t, service.Stop(stopContext(t)))
		})
	}
}

// Test functional options pattern. The subtests that pass a NATS client are in
// base_integration_test.go; the options the port drops (WithHealthCheck, WithHealthInterval,
// OnHealthChange) lost their subtests (design D1, D6).
func TestService_FunctionalOptions(t *testing.T) {
	t.Run("service with no dependencies", func(t *testing.T) {
		service := NewBaseServiceWithOptions("test-service")

		assert.NotNil(t, service)
		assert.Equal(t, "test-service", service.Name())
		assert.Equal(t, StatusStopped, service.Status())
		assert.Nil(t, service.nats)
		assert.Nil(t, service.metricsRegistry)
	})

	t.Run("service with metrics only", func(t *testing.T) {
		metricsRegistry := metric.NewMetricsRegistry()

		service := NewBaseServiceWithOptions("test-service", WithMetrics(metricsRegistry))

		assert.NotNil(t, service)
		assert.Nil(t, service.nats)
		assert.Equal(t, metricsRegistry, service.metricsRegistry)
	})

	t.Run("service with logger", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)

		service := NewBaseServiceWithOptions("test-service", WithLogger(logger))
		assert.Same(t, logger, service.logger)

		// A nil logger keeps the default.
		service = NewBaseServiceWithOptions("test-service", WithLogger(nil))
		assert.NotNil(t, service.logger)
	})
}
