//go:build integration

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
)

// healthBound bounds the wait for the first health check's result.
const healthBound = 10 * time.Second

// Test health monitoring: with no custom check, a connected NATS client makes the service healthy,
// and a client that is not connected makes it unhealthy.
//
// spec: service-lifecycle / A failed health check makes the service unhealthy
func TestService_HealthMonitoring(t *testing.T) {
	t.Run("connected client", func(t *testing.T) {
		natsClient := fixtureClient(t)
		service := NewBaseServiceWithOptions("test-service",
			WithNATS(natsClient),
			WithMetrics(metric.NewMetricsRegistry()))

		require.NoError(t, service.Start(t.Context()))
		t.Cleanup(func() { require.NoError(t, service.Stop(stopContext(t))) })

		ctx, cancel := context.WithTimeout(t.Context(), healthBound)
		defer cancel()
		_, err := probe.Await(ctx,
			func(context.Context) (bool, error) { return service.IsHealthy(), nil },
			func(healthy bool) bool { return healthy })
		require.NoError(t, err, "service should become healthy")
		assert.True(t, service.Health().Healthy)
	})

	t.Run("client not connected", func(t *testing.T) {
		// Built from the shared server's URL and never connected, so its status is not connected.
		natsClient, err := natsclient.NewClient(shared.Reset(t))
		require.NoError(t, err)
		service := NewBaseServiceWithOptions("test-service", WithNATS(natsClient))

		require.NoError(t, service.Start(t.Context()))
		t.Cleanup(func() { require.NoError(t, service.Stop(stopContext(t))) })

		ctx, cancel := context.WithTimeout(t.Context(), healthBound)
		defer cancel()
		_, err = probe.Await(ctx,
			func(context.Context) (int64, error) { return service.GetStatus().FailedHealthChecks, nil },
			func(failed int64) bool { return failed > 0 })
		require.NoError(t, err, "the first health check never failed")
		assert.False(t, service.IsHealthy())
		assert.False(t, service.Health().Healthy)
	})
}

// Test functional options with a NATS client: the subtests of the pin's TestService_FunctionalOptions
// that pass one (design D6), without the dropped options.
func TestService_FunctionalOptionsWithNATS(t *testing.T) {
	t.Run("service with NATS only", func(t *testing.T) {
		natsClient := fixtureClient(t)

		service := NewBaseServiceWithOptions("test-service", WithNATS(natsClient))

		assert.NotNil(t, service)
		assert.Equal(t, natsClient, service.nats)
		assert.Nil(t, service.metricsRegistry)
	})

	t.Run("service with both NATS and metrics", func(t *testing.T) {
		natsClient := fixtureClient(t)
		metricsRegistry := metric.NewMetricsRegistry()

		service := NewBaseServiceWithOptions("test-service",
			WithNATS(natsClient),
			WithMetrics(metricsRegistry))

		assert.NotNil(t, service)
		assert.Equal(t, natsClient, service.nats)
		assert.Equal(t, metricsRegistry, service.metricsRegistry)
	})
}
