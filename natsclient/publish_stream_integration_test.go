//go:build integration

package natsclient

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/errs"
)

// gatheredCount reads one series of a counter from the registry by its Prometheus name and one
// label, so the oracle does not go through the client's own collector. A series never written
// reads 0.
func gatheredCount(t *testing.T, registry *metric.MetricsRegistry, name, label, value string) float64 {
	t.Helper()
	families, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, m := range family.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == label && l.GetValue() == value {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// Design D3: the stream publish calls keep their errors per call. A publish no stream captures
// fails: PublishToStream returns nats.go's error unclassified and counts nothing, while
// PublishToStreamWithAck counts publish_to_stream and classifies the error transient. On a client
// that is not connected, PublishToStreamWithAck returns ErrNotConnected itself and counts nothing.
func TestIntegration_PublishToStreamErrorsPerCall(t *testing.T) {
	const (
		series    = "semengine_jetstream_operation_errors_total"
		operation = "publish_to_stream"
		subject   = "nostream.publish"
	)
	transient := func(err error) bool {
		var ce *errs.ClassifiedError
		return errors.As(err, &ce) && ce.Class == errs.ErrorTransient
	}

	t.Run("no stream captures the subject", func(t *testing.T) {
		registry := metric.NewMetricsRegistry()
		client, err := NewClient(startFixture(t).URL(), WithMetrics(registry))
		require.NoError(t, err)
		require.NoError(t, client.Connect(t.Context()))
		defer closeClient(t, client)
		ctx, cancel := context.WithTimeout(t.Context(), failureBound)
		defer cancel()

		err = client.PublishToStream(ctx, subject, []byte("x"))
		require.Error(t, err)
		var ce *errs.ClassifiedError
		require.False(t, errors.As(err, &ce), "PublishToStream classified its error: %v", err)
		require.Zero(t, gatheredCount(t, registry, series, "operation", operation),
			"publish_to_stream count after PublishToStream")

		ack, err := client.PublishToStreamWithAck(ctx, subject, []byte("x"))
		require.Nil(t, ack)
		require.True(t, transient(err), "PublishToStreamWithAck error %v is not classified transient", err)
		require.Equal(t, 1.0, gatheredCount(t, registry, series, "operation", operation),
			"publish_to_stream count after PublishToStreamWithAck")
	})

	t.Run("not connected", func(t *testing.T) {
		registry := metric.NewMetricsRegistry()
		client, err := NewClient("nats://unused", WithMetrics(registry))
		require.NoError(t, err)

		ack, err := client.PublishToStreamWithAck(t.Context(), subject, []byte("x"))
		require.Nil(t, ack)
		require.Equal(t, ErrNotConnected, err)
		require.Zero(t, gatheredCount(t, registry, series, "operation", operation),
			"publish_to_stream count after a refused PublishToStreamWithAck")
	})
}
