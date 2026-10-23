//go:build integration

package service

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
)

// The pin ran these as MetricsRuntimeSuite on its ServiceSuite, whose NATS client no test here
// used, beside three KVTestHelper tests that exercised only that helper; PR 1 carried neither
// ServiceSuite nor KVTestHelper (archived design setup-04a-03-boot-service-base, D6). The two
// tests of NewMetrics are carried as plain tests.

func TestMetrics_DefaultConfiguration(t *testing.T) {
	// Test that metrics uses proper defaults when config is empty
	svc, err := NewMetrics(json.RawMessage(`{}`), &Dependencies{Logger: slog.Default()})
	require.NoError(t, err)

	metrics := svc.(*Metrics)

	// Verify constructor defaults.
	require.Equal(t, 9090, metrics.config.Port)
	require.Equal(t, "/metrics", metrics.config.Path)
}

func TestMetrics_ConfigValidation(t *testing.T) {
	// Test that invalid configs are rejected
	t.Run("invalid port range", func(t *testing.T) {
		_, err := NewMetrics(json.RawMessage(`{"port": 99999}`), &Dependencies{Logger: slog.Default()})
		require.ErrorContains(t, err, "invalid port")
	})

	t.Run("negative port", func(t *testing.T) {
		_, err := NewMetrics(json.RawMessage(`{"port": -1}`), &Dependencies{Logger: slog.Default()})
		require.ErrorContains(t, err, "invalid port")
	})

	t.Run("retired enabled field", func(t *testing.T) {
		_, err := NewMetrics(json.RawMessage(`{"enabled": true}`), &Dependencies{Logger: slog.Default()})
		require.ErrorContains(t, err, "unknown field")
	})

	t.Run("empty path gets default", func(t *testing.T) {
		m, err := NewMetrics(json.RawMessage(`{"path": ""}`), &Dependencies{
			Logger:          slog.Default(),
			MetricsRegistry: metric.NewMetricsRegistry(),
		})
		require.NoError(t, err)
		require.NotNil(t, m)

		// Empty path should get default "/metrics"
		metrics := m.(*Metrics)
		require.Equal(t, "/metrics", metrics.config.Path)
	})
}
