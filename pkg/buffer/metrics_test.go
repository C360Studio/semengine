package buffer

import (
	"testing"

	"github.com/c360studio/semengine/metric"
	cerrs "github.com/c360studio/semengine/pkg/errs"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetricsOnInjectedRegistry: the buffer's collectors are registered through
// metric.RegisterOrGet on the injected registry, under the semengine namespace with the
// prefix as the component label; a second buffer under one prefix shares the first one's
// collectors; and a registry that already holds another collector type under one of the
// buffer's keys refuses construction (design R2).
//
// Oracle: the operation counts below are fixed by the script each buffer runs, counted by
// hand: capacity 2 under DropOldest, writes 1, 2, 3 (3 evicts 1: one overflow, one drop),
// one read, one peek, leaving one item (utilization 1/2).
func TestMetricsOnInjectedRegistry(t *testing.T) {
	registry := metric.NewMetricsRegistry()

	for range 2 {
		buf, err := NewCircularBuffer[int](2, WithMetrics[int](registry, "shared"))
		require.NoError(t, err)
		for _, v := range []int{1, 2, 3} {
			require.NoError(t, buf.Write(v))
		}
		got, ok := buf.Read()
		require.True(t, ok)
		require.Equal(t, 2, got, "1 was evicted")
		_, ok = buf.Peek()
		require.True(t, ok)
	}

	families, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)
	byName := make(map[string]*dto.MetricFamily)
	for _, mf := range families {
		byName[mf.GetName()] = mf
	}

	counters := map[string]float64{
		"semengine_buffer_writes_total":    6,
		"semengine_buffer_reads_total":     2,
		"semengine_buffer_peeks_total":     2,
		"semengine_buffer_overflows_total": 2,
		"semengine_buffer_drops_total":     2,
	}
	for name, want := range counters {
		mf := byName[name]
		require.NotNil(t, mf, "%s gathered", name)
		require.Len(t, mf.GetMetric(), 1, "%s: the two buffers share one series", name)
		assert.Equal(t, want, mf.GetMetric()[0].GetCounter().GetValue(), name)
		assertComponentLabel(t, mf.GetMetric()[0], "shared")
	}
	gauges := map[string]float64{
		"semengine_buffer_size":        1,
		"semengine_buffer_utilization": 0.5,
	}
	for name, want := range gauges {
		mf := byName[name]
		require.NotNil(t, mf, "%s gathered", name)
		require.Len(t, mf.GetMetric(), 1, "%s: the two buffers share one series", name)
		assert.Equal(t, want, mf.GetMetric()[0].GetGauge().GetValue(), name)
		assertComponentLabel(t, mf.GetMetric()[0], "shared")
	}

	t.Run("another collector type under a key refuses construction", func(t *testing.T) {
		registry := metric.NewMetricsRegistry()
		_, err := metric.RegisterOrGet(registry, "squatted", "buffer_writes",
			prometheus.NewGauge(prometheus.GaugeOpts{Name: "squatter", Help: "owns the key first"}))
		require.NoError(t, err)

		_, err = NewCircularBuffer[int](2, WithMetrics[int](registry, "squatted"))
		require.Error(t, err, "a refused collector must fail the constructor")
		assert.True(t, cerrs.IsTransient(err), "classified Transient (design O7): %v", err)
	})
}

func assertComponentLabel(t *testing.T, m *dto.Metric, want string) {
	t.Helper()
	for _, l := range m.GetLabel() {
		if l.GetName() == "component" {
			assert.Equal(t, want, l.GetValue())
			return
		}
	}
	t.Errorf("no component label on %v", m)
}

// TestNewCircularBufferRefusedReturnsNil: when construction fails, NewCircularBuffer returns a
// nil Buffer[T] with its error, so a caller's nil check agrees with the error. Oracle: the
// interface compared to nil directly (reflection-based assertions treat a nil pointer inside a
// non-nil interface as nil, so they are not used).
func TestNewCircularBufferRefusedReturnsNil(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	_, err := metric.RegisterOrGet(registry, "squatted", "buffer_writes",
		prometheus.NewGauge(prometheus.GaugeOpts{Name: "squatter", Help: "owns the key first"}))
	require.NoError(t, err)

	buf, err := NewCircularBuffer[int](2, WithMetrics[int](registry, "squatted"))
	require.Error(t, err)
	if buf != nil {
		t.Errorf("NewCircularBuffer returned a non-nil Buffer[int] (%T) with its error", buf)
	}
}

// TestReadBatchCountsReadMetric: semengine_buffer_reads_total counts every item removed,
// whether by Read or ReadBatch (#589). Two buffers share one prefix, so the gathered series
// is the sum of both buffers' reads.
//
// Oracle: the script each buffer runs, counted by hand: capacity 4, writes 1, 2, 3, 4; Read
// removes 1; ReadBatch(0) removes nothing; ReadBatch(2) removes 2 and 3; ReadBatch(10), larger
// than the 1 item left, removes 4; ReadBatch(5) on the empty buffer removes nothing. That is
// 4 reads per buffer and 8 on the shared series, with size 0 and utilization 0 at the end.
func TestReadBatchCountsReadMetric(t *testing.T) {
	registry := metric.NewMetricsRegistry()

	for range 2 {
		buf, err := NewCircularBuffer[int](4, WithMetrics[int](registry, "batch"))
		require.NoError(t, err)
		for _, v := range []int{1, 2, 3, 4} {
			require.NoError(t, buf.Write(v))
		}
		got, ok := buf.Read()
		require.True(t, ok)
		require.Equal(t, 1, got)
		assert.Empty(t, buf.ReadBatch(0), "maxN 0 removes nothing")
		assert.Equal(t, []int{2, 3}, buf.ReadBatch(2))
		assert.Equal(t, []int{4}, buf.ReadBatch(10), "an oversized batch removes what is there")
		assert.Empty(t, buf.ReadBatch(5), "a batch from an empty buffer removes nothing")
		assert.Equal(t, int64(4), buf.Stats().Reads())
	}

	families, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)
	byName := make(map[string]*dto.MetricFamily)
	for _, mf := range families {
		byName[mf.GetName()] = mf
	}

	reads := byName["semengine_buffer_reads_total"]
	require.NotNil(t, reads, "reads_total gathered")
	require.Len(t, reads.GetMetric(), 1, "the two buffers share one series")
	assert.Equal(t, 8.0, reads.GetMetric()[0].GetCounter().GetValue(), "items removed by Read and ReadBatch")
	assertComponentLabel(t, reads.GetMetric()[0], "batch")

	for _, name := range []string{"semengine_buffer_size", "semengine_buffer_utilization"} {
		mf := byName[name]
		require.NotNil(t, mf, "%s gathered", name)
		require.Len(t, mf.GetMetric(), 1, name)
		assert.Equal(t, 0.0, mf.GetMetric()[0].GetGauge().GetValue(), name)
	}
}
