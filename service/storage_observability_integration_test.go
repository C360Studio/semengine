//go:build integration

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/health"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/natsclient/storagereport"
)

// The storage observability service against the package's shared NATS server (design D7, U-f;
// pin storage_observability_integration_test.go:33-316). Every wait reads what an operator reads,
// GET /storage-observability/report and the metrics registry, and is bounded by
// storageReportWait: the 1 s collection interval is a real-clock window (design § Harness rules).

// storageReportWait bounds every wait in this file for a collection to reach the route.
const storageReportWait = 30 * time.Second

// routedStorageReport is GET <prefix>/report decoded by the wire names design D5 lists, not by
// the service's own response type, so a renamed or dropped field fails here.
type routedStorageReport struct {
	ReportOnly bool       `json:"report_only"`
	Synced     bool       `json:"synced"`
	UpdatedAt  *time.Time `json:"updated_at"`
	Summary    struct {
		Resources    int                                 `json:"resources"`
		Pressure     map[storagereport.PressureState]int `json:"pressure"`
		NotEvaluated int                                 `json:"not_evaluated"`
	} `json:"summary"`
	Account   *storagereport.AccountReport   `json:"account"`
	Resources []storagereport.ResourceReport `json:"resources"`
}

// row returns the report's row for the resource named name.
func (r routedStorageReport) row(name string) (storagereport.ResourceReport, bool) {
	for _, row := range r.Resources {
		if row.Resource.Name == name {
			return row, true
		}
	}
	return storagereport.ResourceReport{}, false
}

// awaitStorageReport requests url until the report it answers satisfies until, and returns that
// report. It fails the test when storageReportWait passes first, naming what it waited for and
// the last answer.
func awaitStorageReport(
	t *testing.T, url, what string, until func(routedStorageReport) bool,
) routedStorageReport {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), storageReportWait)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	last := "no answer yet"
	for {
		status, body, err := httpGet(ctx, url)
		switch {
		case err != nil:
			if ctx.Err() == nil { // keep the last answer, not the request the deadline cut short
				last = err.Error()
			}
		case status != http.StatusOK:
			last = fmt.Sprintf("status %d: %s", status, body)
		default:
			var report routedStorageReport
			require.NoError(t, json.Unmarshal([]byte(body), &report), "GET %s answered %s", url, body)
			if until(report) {
				return report
			}
			last = body
		}
		select {
		case <-ctx.Done():
			t.Fatalf("GET %s: %s not seen within %v; last answer: %s", url, what, storageReportWait, last)
		case <-ticker.C:
		}
	}
}

// serveStorageReport mounts s's routes at the prefix the service manager computes for the
// service's name, on a test server, and returns the report route's URL.
func serveStorageReport(t *testing.T, s *StorageObservabilityService) string {
	t.Helper()
	prefix := "/" + serviceNameToPrefix(StorageObservabilityServiceName)
	require.Equal(t, "/storage-observability", prefix)
	mux := http.NewServeMux()
	s.RegisterHTTPHandlers(prefix, mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server.URL + prefix + "/report"
}

// startStorageObservability starts s and stops it when the test ends under a bounded detached
// context.
func startStorageObservability(t *testing.T, ctx context.Context, s *StorageObservabilityService) {
	t.Helper()
	require.NoError(t, s.Start(ctx))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		require.NoError(t, s.Stop(stopCtx))
	})
}

// TestStorageObservabilityCollectsPublishesAndServes drives the service's whole production path
// against a real server: the account listing, the report bucket acquired through the catalog,
// the publication, the watch that feeds the route, health and the metrics. The streams exist
// before the service and are never touched through it, so the inventory is the account's, not
// a view of the service's own handles (pin :33-316).
func TestStorageObservabilityCollectsPublishesAndServes(t *testing.T) {
	client := fixtureClient(t)
	js, err := client.JetStream()
	require.NoError(t, err)

	const bounded = "STORAGE_OBS_INTEGRATION_BOUNDED"
	const boundedLimit = int64(16 << 20)
	_, err = js.CreateStream(t.Context(), jetstream.StreamConfig{
		Name:     bounded,
		Subjects: []string{"storage.obs.integration.bounded.>"},
		Storage:  jetstream.FileStorage,
		MaxBytes: boundedLimit,
	})
	require.NoError(t, err)

	const unbounded = "STORAGE_OBS_INTEGRATION_UNBOUNDED"
	_, err = js.CreateStream(t.Context(), jetstream.StreamConfig{
		Name:     unbounded,
		Subjects: []string{"storage.obs.integration.unbounded.>"},
		Storage:  jetstream.MemoryStorage,
	})
	require.NoError(t, err)

	storage, registry := newTestStorageObservability(t, client, `{"interval":"1s","timeout":"1s"}`, nil)
	url := serveStorageReport(t, storage)
	startStorageObservability(t, t.Context(), storage)

	// The report bucket is acquired on the first publication, so it does not exist while the
	// first collection lists the account: its own stream appears from the second collection
	// on. Waiting for it as well keeps the reads below from racing that.
	reportStream := natsclient.KVStreamPrefix + graph.BucketStorageReport
	report := awaitStorageReport(t, url, "the bounded stream and the report bucket's own stream",
		func(r routedStorageReport) bool {
			_, sawBounded := r.row(bounded)
			_, sawSelf := r.row(reportStream)
			return r.Synced && r.Account != nil && sawBounded && sawSelf
		})

	t.Run("the route serves the consumed report", func(t *testing.T) {
		assert.True(t, report.ReportOnly)
		assert.NotNil(t, report.UpdatedAt, "a change was applied, so updated_at is set")
		assert.Equal(t, len(report.Resources), report.Summary.Resources)
		assert.NotNil(t, report.Summary.Pressure, "pressure is never null")
	})

	t.Run("the bounded stream appears with its real capacity", func(t *testing.T) {
		row, ok := report.row(bounded)
		require.True(t, ok)
		require.Equal(t, storagereport.CapacityBounded, row.Resource.Bytes.State)
		limit, has := row.Resource.Bytes.Limit()
		require.True(t, has)
		assert.Equal(t, boundedLimit, limit)
		assert.True(t, row.Pressure.Evaluated)
		assert.Equal(t, storagereport.PressureNormal, row.Pressure.State)
	})

	t.Run("an unbounded stream stays visible with no headroom", func(t *testing.T) {
		row, ok := report.row(unbounded)
		require.True(t, ok, "an unbounded resource must stay visible, not be filtered out")
		assert.Equal(t, storagereport.CapacityUnbounded, row.Resource.Bytes.State)
		assert.False(t, row.Pressure.Evaluated, "no band has an input, so there is no state")
		assert.Nil(t, row.Projection.HeadroomBytes)
		assert.Nil(t, row.Projection.HeadroomFraction)
		assert.Nil(t, row.Projection.TimeToThreshold)
		assert.GreaterOrEqual(t, report.Summary.NotEvaluated, 1,
			"rows carrying no pressure state are counted rather than folded into normal")
	})

	t.Run("the report bucket observes itself as the catalog declares it", func(t *testing.T) {
		row, ok := report.row(reportStream)
		require.True(t, ok, "the report bucket's own backing stream must be inventoried")
		assert.Equal(t, storagereport.AttributionAttributed, row.Resource.Attribution)
		declared, ok := kvcatalog.SpecFor(graph.BucketStorageReport)
		require.True(t, ok)
		assert.Equal(t, declared.Owner, row.Resource.Owner)
	})

	t.Run("an unbounded account limit is not applicable, never within", func(t *testing.T) {
		// A stock server reports no account limit for either tier, so this is the default path.
		for _, tier := range []storagereport.StorageTier{storagereport.TierFile, storagereport.TierMemory} {
			comparison, ok := report.Account.TierFor(tier)
			require.True(t, ok, "tier %s must appear in the account report", tier)
			require.Equal(t, storagereport.CapacityUnbounded, comparison.Limit.State)
			assert.Equal(t, storagereport.OvercommitmentNotApplicable, comparison.State)
			assert.Equal(t, storagereport.OvercommitmentUnavailableUnboundedLimit, comparison.Unavailable)
		}
		// The declared sums stay per tier, never merged.
		file, _ := report.Account.TierFor(storagereport.TierFile)
		memory, _ := report.Account.TierFor(storagereport.TierMemory)
		assert.GreaterOrEqual(t, file.DeclaredBytes, boundedLimit,
			"the bounded file stream's declaration is in the file tier")
		assert.Equal(t, 0, memory.BoundedResources,
			"the memory stream declared no bound, so it is counted rather than summed")
		assert.GreaterOrEqual(t, memory.UnboundedResources, 1)
	})

	t.Run("the metrics carry the same rows", func(t *testing.T) {
		// Compared on facts that do not move between two reads, a configured bound and whether a
		// series exists, not on live byte counts, which the next collection advances.
		labels := map[string]string{"resource": bounded, "tier": "file", "kind": "stream"}
		_, ok := gaugeValue(t, registry, "semengine_storage_resource_used_bytes", labels)
		require.True(t, ok, "the route served the bounded row, so its usage series exists")
		assert.InDelta(t, float64(boundedLimit),
			requireGauge(t, registry, "semengine_storage_resource_limit_bytes", labels), 1e-9)
		assert.InDelta(t, 0, requireGauge(t, registry, "semengine_storage_resource_pressure", labels), 1e-9,
			"normal pressure is severity 0")

		unboundedLabels := map[string]string{"resource": unbounded}
		_, ok = gaugeValue(t, registry, "semengine_storage_resource_used_bytes", unboundedLabels)
		assert.True(t, ok)
		_, ok = gaugeValue(t, registry, "semengine_storage_resource_pressure", unboundedLabels)
		assert.False(t, ok, "a pressure series exists only for an evaluated row")

		assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_account_limit_state",
			map[string]string{"tier": "file", "state": "unbounded"}), 1e-9)
		assert.InDelta(t, 1, requireGauge(t, registry, "semengine_storage_account_overcommitment",
			map[string]string{"tier": "file", "state": "not-applicable"}), 1e-9)
		assert.InDelta(t, 0, requireGauge(t, registry, "semengine_storage_account_overcommitment",
			map[string]string{"tier": "file", "state": "within-limit"}), 1e-9)
		_, ok = gaugeValue(t, registry, "semengine_storage_account_limit_bytes",
			map[string]string{"tier": "file"})
		assert.False(t, ok, "no limit number exists, so no limit series is published")
	})

	t.Run("health reports the picture and keeps the base verdict", func(t *testing.T) {
		status := storage.Health()
		assert.True(t, status.Healthy)
		assert.Contains(t, status.Message, "storage pressure (report-only)")
		assert.Contains(t, status.Message, "collected")
	})

	t.Run("a growth rate appears once two observations exist", func(t *testing.T) {
		// The rate is the byte change over the time between successive published observations,
		// at least 5 s apart, so this waits on elapsed time rather than on a state change.
		awaitStorageReport(t, url, "a known growth rate on the bounded stream", func(r routedStorageReport) bool {
			row, ok := r.row(bounded)
			return ok && row.Growth.State == storagereport.GrowthKnown
		})
		_, ok := gaugeValue(t, registry, "semengine_storage_resource_growth_bytes_per_second",
			map[string]string{"resource": bounded})
		assert.True(t, ok, "a measured rate reaches the metrics surface")
	})
}

// TestStorageObservabilityKeepsCollectingAfterStartContextEnds: ending the context Start was
// given, after Start returned, stops nothing. The stream is created only after that context
// ended, so only a collection made after it can put the stream's row on the route (design D2).
//
// spec: service-lifecycle / The storage observability service's Start bounds Start and Stop is the only stop
func TestStorageObservabilityKeepsCollectingAfterStartContextEnds(t *testing.T) {
	client := fixtureClient(t)
	js, err := client.JetStream()
	require.NoError(t, err)

	storage, _ := newTestStorageObservability(t, client, `{"interval":"1s","timeout":"1s"}`, nil)
	url := serveStorageReport(t, storage)
	startCtx, cancelStart := context.WithCancel(t.Context())
	startStorageObservability(t, startCtx, storage)
	cancelStart()

	const late = "STORAGE_OBS_AFTER_START_CONTEXT"
	_, err = js.CreateStream(t.Context(), jetstream.StreamConfig{
		Name:     late,
		Subjects: []string{"storage.obs.after.start.>"},
		Storage:  jetstream.MemoryStorage,
	})
	require.NoError(t, err)

	awaitStorageReport(t, url, "the row of a stream created after the Start context ended",
		func(r routedStorageReport) bool {
			_, ok := r.row(late)
			return ok
		})
}

// TestStorageObservabilityCriticalPressureKeepsReadiness: critical pressure is report-only. A
// manager composes the service over the shared server with thresholds that make a bounded stream
// holding a little data critical; once the route serves the critical row, /readyz still answers
// READY and /health 200, with the count in the service's message (design D5; pin
// storage_observability_test.go:284-344, here on the production path).
func TestStorageObservabilityCriticalPressureKeepsReadiness(t *testing.T) {
	client := fixtureClient(t)
	js, err := client.JetStream()
	require.NoError(t, err)

	// 8 KiB in a 64 KiB stream leaves a free fraction below 0.88, at or below critical_headroom.
	const pressed = "STORAGE_OBS_CRITICAL"
	_, err = js.CreateStream(t.Context(), jetstream.StreamConfig{
		Name:     pressed,
		Subjects: []string{"storage.obs.critical.>"},
		Storage:  jetstream.FileStorage,
		MaxBytes: 64 << 10,
	})
	require.NoError(t, err)
	_, err = js.Publish(t.Context(), "storage.obs.critical.fill", make([]byte, 8<<10))
	require.NoError(t, err)

	storage, _ := newTestStorageObservability(t, client, `{"interval":"1s","timeout":"1s",`+
		`"pressure_thresholds":{"warning_headroom":0.99,"high_headroom":0.98,"critical_headroom":0.97}}`, nil)

	deps := createTestServiceDependencies()
	deps.NATSClient = client
	manager := createTestServiceManager(ManagerConfig{}, deps)
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	require.NoError(t, manager.RegisterInstance(StorageObservabilityServiceName, storage))
	require.NoError(t, manager.StartAll(t.Context()))
	stopManagerAtEnd(t, manager)
	base := observedManagerHTTPURL(t, manager)

	awaitStorageReport(t, base+"/storage-observability/report", "the stream's critical row",
		func(r routedStorageReport) bool {
			row, ok := r.row(pressed)
			return ok && row.Pressure.Evaluated && row.Pressure.State == storagereport.PressureCritical
		})

	requireAnswer(t, base+"/readyz", http.StatusOK, "READY")

	status, body, err := httpGet(t.Context(), base+"/health")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, body)
	var system health.Status
	require.NoError(t, json.Unmarshal([]byte(body), &system))
	assert.True(t, system.Healthy, body)
	var own *health.Status
	for i := range system.SubStatuses {
		if system.SubStatuses[i].Component == StorageObservabilityServiceName {
			own = &system.SubStatuses[i]
		}
	}
	require.NotNil(t, own, "/health carries no %s status: %s", StorageObservabilityServiceName, body)
	assert.True(t, own.Healthy, own.Message)
	assert.True(t, strings.Contains(own.Message, "critical=1 high="),
		"the service's message must count the one critical row: %q", own.Message)
}
