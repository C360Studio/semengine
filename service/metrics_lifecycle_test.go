package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
)

// TestMetricsStartReportsServerBindFailureBeforeReturning: Start given the port of a listener the
// test holds returns the bind failure, and the service holds no server and stays stopped.
//
// spec: service-lifecycle / The metrics service serves on its own or through the manager
func TestMetricsStartReportsServerBindFailureBeforeReturning(t *testing.T) {
	raw := json.RawMessage(fmt.Sprintf(`{"port":%d,"path":"/metrics"}`, heldPort(t)))
	svc, err := NewMetrics(raw, &Dependencies{MetricsRegistry: metric.NewMetricsRegistry()})
	require.NoError(t, err)
	m := svc.(*Metrics)

	err = m.Start(t.Context())
	require.Error(t, err)
	require.ErrorContains(t, err, "start metrics server")
	require.ErrorContains(t, err, "address already in use")
	require.Nil(t, m.server, "a failed bind left the service holding a server")
	require.Equal(t, StatusStopped, m.Status())
	require.NoError(t, m.Stop(stopContext(t)))
}
