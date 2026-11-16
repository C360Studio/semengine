package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/errs"
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

// TestMetricsStopWithEndedContextFencesServerClaim: any non-nil Stop (including an ended context)
// fences lifecycleCalled so neither a later manager server claim nor Start is admitted.
func TestMetricsStopWithEndedContextFencesServerClaim(t *testing.T) {
	svc, err := NewMetrics(json.RawMessage(`{"port":9090,"path":"/metrics"}`),
		&Dependencies{MetricsRegistry: metric.NewMetricsRegistry()})
	require.NoError(t, err)
	m := svc.(*Metrics)

	ended, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, m.Stop(ended))
	_, err = m.claimManagerServer()
	require.Error(t, err, "claimManagerServer must be refused after Stop(endedCtx)")
	require.ErrorIs(t, m.Start(t.Context()), errs.ErrAlreadyStarted)
}
