package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/c360studio/semengine/natsclient/storagereport"
)

// The storage report route and the example alert rules (design D5 and Q1, U-e), carried from the
// pin's storage_observability_http_test.go. The response shape is proven through
// storageReportResponseFrom and the JSON encoding; the route through RegisterHTTPHandlers on a
// service built by NewStorageObservabilityService (design O4: no reader field a test could swap).

// --- fakes (pin :24-85) ---------------------------------------------------------------------

// fakeReportEntry is one KV entry as a watch delivers it.
type fakeReportEntry struct {
	key   string
	value []byte
	op    jetstream.KeyValueOp
}

func (e fakeReportEntry) Bucket() string                  { return "STORAGE_REPORT_FAKE" }
func (e fakeReportEntry) Key() string                     { return e.key }
func (e fakeReportEntry) Value() []byte                   { return e.value }
func (e fakeReportEntry) Revision() uint64                { return 1 }
func (e fakeReportEntry) Created() time.Time              { return time.Now() }
func (e fakeReportEntry) Delta() uint64                   { return 0 }
func (e fakeReportEntry) Operation() jetstream.KeyValueOp { return e.op }

// fakeReportWatcher reproduces one KV watch, including the single nil entry nats.go sends once
// every current value has been delivered.
type fakeReportWatcher struct {
	updates chan jetstream.KeyValueEntry
}

func (w *fakeReportWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w *fakeReportWatcher) Stop() error                             { return nil }

func (w *fakeReportWatcher) put(t *testing.T, key string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	w.updates <- fakeReportEntry{key: key, value: encoded, op: jetstream.KeyValuePut}
}

func (w *fakeReportWatcher) synced() { w.updates <- nil }

// fakeReportWatchStore hands the consumer one watcher whose entries the test queued before the
// consumer runs, so no wait is needed for the watch to be established.
type fakeReportWatchStore struct {
	watcher *fakeReportWatcher
}

func (s *fakeReportWatchStore) WatchAll(context.Context, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return s.watcher, nil
}

// observedSignal forwards every call to the metrics surface and closes done once it has seen
// want resources. The consumer marks its view synced before it calls the observer, so after done
// both surfaces hold the whole publication. Called only from the consumer's goroutine.
type observedSignal struct {
	storagereport.Observer
	want int
	seen int
	done chan struct{}
}

func (o *observedSignal) ObserveResource(row storagereport.ResourceReport) {
	o.Observer.ObserveResource(row)
	o.seen++
	if o.seen == o.want {
		close(o.done)
	}
}

// --- helpers --------------------------------------------------------------------------------

// wireReport encodes the response storageReportResponseFrom builds from snapshot and decodes it
// twice: as the typed response and as the generic document an operator's tooling sees.
func wireReport(t *testing.T, snapshot storagereport.Snapshot) (StorageReportResponse, map[string]any) {
	t.Helper()
	body, err := json.Marshal(storageReportResponseFrom(snapshot))
	require.NoError(t, err)
	var response StorageReportResponse
	require.NoError(t, json.Unmarshal(body, &response))
	var document map[string]any
	require.NoError(t, json.Unmarshal(body, &document))
	return response, document
}

// reportRoute builds a service over a client that never connects, so its consumer's view is the
// unread report, and mounts its routes under prefix.
func reportRoute(t *testing.T, prefix string) (*StorageObservabilityService, *http.ServeMux) {
	t.Helper()
	s, _ := newTestStorageObservability(t, newUnconnectedClient(t), "{}", nil)
	mux := http.NewServeMux()
	s.RegisterHTTPHandlers(prefix, mux)
	return s, mux
}

func serve(mux *http.ServeMux, method, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

// --- the response shape (pin :128-310) ------------------------------------------------------

// TestStorageReportHTTP_ResponseRoundTrips pins the wire shape an operator builds a CLI or a
// dashboard against.
func TestStorageReportHTTP_ResponseRoundTrips(t *testing.T) {
	collected := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	updated := collected.Add(time.Second)

	bounded := withPressure(resourceRow("LOGS", storagereport.TierFile), storagereport.PressureHigh)
	bounded.CollectedAt = collected
	unbounded := unboundedRow("EVENTS", storagereport.TierMemory)
	unbounded.CollectedAt = collected

	response, document := wireReport(t, storagereport.Snapshot{
		Resources:      []storagereport.ResourceReport{unbounded, bounded},
		Account:        storagereport.AccountReport{ProducedBy: "unit-test", CollectedAt: collected},
		AccountKnown:   true,
		Synced:         true,
		UpdatedAt:      updated,
		PressureCounts: map[storagereport.PressureState]int{storagereport.PressureHigh: 1},
		NotEvaluated:   1,
		WorstPressure:  storagereport.PressureHigh,
	})

	assert.Equal(t, true, document["report_only"], "the response states the report-only guarantee on its face")
	assert.Equal(t, true, document["synced"])
	assert.Equal(t, updated.Format(time.RFC3339Nano), document["updated_at"])

	summary, ok := document["summary"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(2), summary["resources"])
	assert.Equal(t, float64(1), summary["not_evaluated"])
	assert.Equal(t, "high", summary["worst_pressure"])
	pressure, ok := summary["pressure"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), pressure["high"])

	resources, ok := document["resources"].([]any)
	require.True(t, ok)
	require.Len(t, resources, 2)

	// The rows are the published rows verbatim, so the HTTP shape cannot drift from the bucket's.
	assert.Equal(t, []storagereport.ResourceReport{unbounded, bounded}, response.Resources)
	require.NotNil(t, response.Account)
	assert.Equal(t, "unit-test", response.Account.ProducedBy)
}

// TestStorageReportHTTP_UnknownAccountIsAbsentNotZero keeps an unread account from publishing a
// zero-valued comparison that reads as "no tiers, nothing over-committed".
func TestStorageReportHTTP_UnknownAccountIsAbsentNotZero(t *testing.T) {
	response, document := wireReport(t, storagereport.Snapshot{Synced: true})

	_, present := document["account"]
	assert.False(t, present, "an unread account publishes no comparison, never an empty one")
	assert.Nil(t, response.Account)
}

// TestStorageReportHTTP_UnreadReportSaysSo distinguishes "not read yet" from "the account holds
// nothing". The route is a real service's over a client that never connects, whose consumer has
// read nothing: the unread report, served as a 200.
func TestStorageReportHTTP_UnreadReportSaysSo(t *testing.T) {
	_, mux := reportRoute(t, "/storage-observability")
	recorder := serve(mux, http.MethodGet, "/storage-observability/report")
	require.Equal(t, http.StatusOK, recorder.Code, "an unread report is still a 200: this surface fails no gate")

	var response StorageReportResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Synced)
	assert.Empty(t, response.Resources)

	var document map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &document))
	_, present := document["updated_at"]
	assert.False(t, present, "nothing was ever applied, so no update time is claimed")
	assert.NotNil(t, document["resources"], "the list is empty, never null")
	summary, ok := document["summary"].(map[string]any)
	require.True(t, ok)
	assert.NotNil(t, summary["pressure"], "the pressure tally is empty, never null")
}

// TestStorageReportHTTP_NotEvaluatedRowsStayVisible: a row carrying no pressure state is served
// and counted, never filtered out or folded into normal.
func TestStorageReportHTTP_NotEvaluatedRowsStayVisible(t *testing.T) {
	normal := withPressure(resourceRow("QUIET", storagereport.TierFile), storagereport.PressureNormal)
	unbounded := unboundedRow("UNBOUNDED", storagereport.TierFile)
	unknown := unknownRow("OFFLINE")

	response, _ := wireReport(t, storagereport.Snapshot{
		Resources:      []storagereport.ResourceReport{normal, unbounded, unknown},
		Synced:         true,
		PressureCounts: map[storagereport.PressureState]int{storagereport.PressureNormal: 1},
		NotEvaluated:   2,
	})

	names := make(map[string]storagereport.ResourceReport, len(response.Resources))
	for _, row := range response.Resources {
		names[row.Resource.Name] = row
	}
	require.Contains(t, names, "UNBOUNDED", "an unbounded resource must be named, not filtered out")
	require.Contains(t, names, "OFFLINE", "a resource with unknown capacity must be named too")
	assert.Equal(t, 2, response.Summary.NotEvaluated,
		"the rows carrying no state at all are counted rather than folded into normal")

	// Never represented as having headroom.
	for _, name := range []string{"UNBOUNDED", "OFFLINE"} {
		row := names[name]
		assert.False(t, row.Pressure.Evaluated, "%s carries no pressure state", name)
		assert.Nil(t, row.Projection.HeadroomBytes, "%s must not report headroom bytes", name)
		assert.Nil(t, row.Projection.HeadroomFraction, "%s must not report a headroom fraction", name)
		assert.Nil(t, row.Projection.TimeToThreshold, "%s must not project exhaustion", name)
		assert.NotEmpty(t, row.Projection.HeadroomUnavailable, "%s says why it has no headroom", name)
	}

	// And the two stay distinguishable from each other on the wire.
	assert.Equal(t, storagereport.CapacityUnbounded, names["UNBOUNDED"].Resource.Bytes.State)
	assert.Equal(t, storagereport.CapacityUnknown, names["OFFLINE"].Resource.Bytes.State)

	assert.Len(t, response.Resources, 3, "every published row is served; the route does not filter")
}

// TestStorageReportHTTP_WorstPressureIsEmptyWhenNothingIsEvaluated keeps the summary from
// asserting normal over an account whose rows all declined to evaluate.
func TestStorageReportHTTP_WorstPressureIsEmptyWhenNothingIsEvaluated(t *testing.T) {
	_, document := wireReport(t, storagereport.Snapshot{
		Resources:    []storagereport.ResourceReport{unboundedRow("UNBOUNDED", storagereport.TierFile)},
		Synced:       true,
		NotEvaluated: 1,
	})

	summary, ok := document["summary"].(map[string]any)
	require.True(t, ok)
	_, present := summary["worst_pressure"]
	assert.False(t, present, "no row was evaluated, so no verdict is published")
	assert.Equal(t, float64(1), summary["not_evaluated"])
}

// TestStorageReportHTTP_CriticalPressureFailsNothing: critical pressure is a fact in the body of
// a report that still says report-only. The route answering 200 on a critical row is
// TestStorageObservabilityCriticalPressureKeepsReadiness (U-f), over a real account.
func TestStorageReportHTTP_CriticalPressureFailsNothing(t *testing.T) {
	response, _ := wireReport(t, storagereport.Snapshot{
		Resources: []storagereport.ResourceReport{
			withPressure(resourceRow("FULL", storagereport.TierFile), storagereport.PressureCritical),
		},
		Synced:         true,
		PressureCounts: map[storagereport.PressureState]int{storagereport.PressureCritical: 1},
		WorstPressure:  storagereport.PressureCritical,
	})

	assert.True(t, response.ReportOnly)
	assert.Equal(t, storagereport.PressureCritical, response.Summary.WorstPressure)
	assert.Len(t, response.Resources, 1)
}

// --- the route (pin :311-345) ---------------------------------------------------------------

// TestStorageReportHTTP_RejectsNonGET keeps the route read-only.
func TestStorageReportHTTP_RejectsNonGET(t *testing.T) {
	_, mux := reportRoute(t, "/storage-observability")
	assert.Equal(t, http.StatusMethodNotAllowed,
		serve(mux, http.MethodPost, "/storage-observability/report").Code)
}

// TestStorageReportHTTP_PrefixWithOrWithoutTrailingSlash follows the registration shape the other
// services use, where the prefix may or may not carry one.
func TestStorageReportHTTP_PrefixWithOrWithoutTrailingSlash(t *testing.T) {
	for _, prefix := range []string{"/storage-observability", "/storage-observability/"} {
		_, mux := reportRoute(t, prefix)
		assert.Equal(t, http.StatusOK, serve(mux, http.MethodGet, "/storage-observability/report").Code,
			"prefix %q", prefix)
	}
}

// TestStorageReportHTTP_AnswersGETAndHEAD: the route is a GET pattern, so ServeMux answers HEAD as
// GET and refuses every other method with 405 and an Allow header naming GET (design D5).
func TestStorageReportHTTP_AnswersGETAndHEAD(t *testing.T) {
	_, mux := reportRoute(t, "/storage-observability")

	get := serve(mux, http.MethodGet, "/storage-observability/report")
	require.Equal(t, http.StatusOK, get.Code)
	assert.Equal(t, "application/json", get.Header().Get("Content-Type"))
	var response StorageReportResponse
	require.NoError(t, json.Unmarshal(get.Body.Bytes(), &response))
	assert.True(t, response.ReportOnly)

	head := serve(mux, http.MethodHead, "/storage-observability/report")
	assert.Equal(t, http.StatusOK, head.Code)
	assert.Equal(t, "application/json", head.Header().Get("Content-Type"))

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		refused := serve(mux, method, "/storage-observability/report")
		assert.Equal(t, http.StatusMethodNotAllowed, refused.Code, method)
		assert.Contains(t, refused.Header().Get("Allow"), http.MethodGet, method)
	}
}

// TestStorageObservabilityRoutesAfterStop: GET /report reads the consumer's snapshot, which does
// no I/O, so it answers before Start, while running and after Stop; after Stop the health message
// carries the base service's verdict (design, "Reads during and after Stop").
func TestStorageObservabilityRoutesAfterStop(t *testing.T) {
	s, mux := reportRoute(t, "/storage-observability")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cancel()
		assert.NoError(t, s.Stop(ctx))
	})

	answers := func(when string) {
		t.Helper()
		recorder := serve(mux, http.MethodGet, "/storage-observability/report")
		require.Equal(t, http.StatusOK, recorder.Code, when)
		var response StorageReportResponse
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response), when)
		assert.True(t, response.ReportOnly, when)
	}

	answers("before Start")
	require.NoError(t, s.Start(t.Context()))
	answers("while running")

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	answers("after Stop")

	// Health keeps the base service's verdict and appends the storage picture (design D5).
	base, health := s.BaseService.Health(), s.Health()
	assert.False(t, health.Healthy, "a stopped service is not healthy")
	assert.Equal(t, base.Status, health.Status)
	assert.True(t, strings.HasPrefix(health.Message, base.Message),
		"health after Stop carries the base verdict %q, got %q", base.Message, health.Message)
}

// --- one produced truth (pin :346-445) ------------------------------------------------------

// TestStorageReportHTTP_CannotDivergeFromTheMetricsSurface: one consumer, fed by one watch, fans
// out to both surfaces, so the route's rows and the published series agree about every resource.
// A test that hand-built a snapshot for the route and fed rows to the metrics separately would
// prove only that two fixtures matched.
func TestStorageReportHTTP_CannotDivergeFromTheMetricsSurface(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	// One publication, four resources spanning every state that matters, queued with the watch's
	// sync marker before the consumer runs.
	rows := []storagereport.ResourceReport{
		withPressure(resourceRow("CRITICAL", storagereport.TierFile), storagereport.PressureCritical),
		withPressure(resourceRow("NORMAL", storagereport.TierFile), storagereport.PressureNormal),
		unboundedRow("UNBOUNDED", storagereport.TierMemory),
		unknownRow("OFFLINE"),
	}
	watcher := &fakeReportWatcher{updates: make(chan jetstream.KeyValueEntry, len(rows)+1)}
	for _, row := range rows {
		watcher.put(t, row.Resource.Name, row)
	}
	watcher.synced()

	observer := &observedSignal{Observer: metrics, want: len(rows), done: make(chan struct{})}
	consumer, err := storagereport.NewConsumer(&fakeReportWatchStore{watcher: watcher},
		storagereport.ConsumerConfig{Observer: observer, RetryBackoff: time.Millisecond})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		consumer.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the consumer did not stop when its context ended")
		}
	})

	select {
	case <-observer.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the consumer did not deliver every published row to the metrics surface")
	}

	response, _ := wireReport(t, consumer.Snapshot())
	require.True(t, response.Synced)
	require.Len(t, response.Resources, len(rows))

	// The numeric severity the alert rules read (configs/prometheus/rules/storage-pressure.yml:
	// 0=normal, 1=warning, 2=high, 3=critical).
	severity := map[storagereport.PressureState]float64{
		storagereport.PressureNormal:   0,
		storagereport.PressureWarning:  1,
		storagereport.PressureHigh:     2,
		storagereport.PressureCritical: 3,
	}
	for _, row := range response.Resources {
		name := row.Resource.Name
		labels := map[string]string{"resource": name}

		// Pressure: a series exists exactly when the row carries a verdict, and says the same one.
		value, published := gaugeValue(t, registry, "semengine_storage_resource_pressure", labels)
		assert.Equal(t, row.Pressure.Evaluated, published,
			"%s: a pressure series exists exactly when the row was evaluated", name)
		if row.Pressure.Evaluated {
			want, known := severity[row.Pressure.State]
			require.True(t, known, "%s: unexpected pressure state %q", name, row.Pressure.State)
			assert.InDelta(t, want, value, 1e-9, "%s: both surfaces report the same pressure", name)
		}

		// Usage: absent on both surfaces for an unreadable resource.
		used, hasUsed := gaugeValue(t, registry, "semengine_storage_resource_used_bytes", labels)
		rowUsed, rowHasUsed := row.Resource.Bytes.Usage()
		assert.Equal(t, rowHasUsed, hasUsed, "%s: usage is present on both surfaces or neither", name)
		if rowHasUsed {
			assert.InDelta(t, float64(rowUsed), used, 1e-9, "%s: both surfaces report the same usage", name)
		}

		// Limit: absent for unbounded and unknown, so neither reads as having headroom.
		limit, hasLimit := gaugeValue(t, registry, "semengine_storage_resource_limit_bytes", labels)
		rowLimit, rowHasLimit := row.Resource.Bytes.Limit()
		assert.Equal(t, rowHasLimit, hasLimit, "%s: a limit is present on both surfaces or neither", name)
		if rowHasLimit {
			assert.InDelta(t, float64(rowLimit), limit, 1e-9, "%s: both surfaces report the same bound", name)
		}
	}

	// The aggregate an alert rule reads equals the summary the route serves.
	notEvaluated := 0.0
	for _, tier := range []string{"file", "memory", "unknown"} {
		value, _ := gaugeValue(t, registry, "semengine_storage_pressure_resources",
			map[string]string{"tier": tier, "pressure_state": pressureStateNotEvaluated})
		notEvaluated += value
	}
	assert.InDelta(t, float64(response.Summary.NotEvaluated), notEvaluated, 1e-9,
		"both surfaces count the same not-evaluated rows")
	assert.Equal(t, 2, response.Summary.NotEvaluated)
}

// --- the example alert rules (pin :448-624; design Q1) --------------------------------------

// storageAlertRuleFile is the example rule set operators copy; the engine does not load it.
const storageAlertRuleFile = "../configs/prometheus/rules/storage-pressure.yml"

type promRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

type promRuleFile struct {
	Groups []struct {
		Name  string     `yaml:"name"`
		Rules []promRule `yaml:"rules"`
	} `yaml:"groups"`
}

// loadStorageAlertRules parses the example rule set and flattens it.
func loadStorageAlertRules(t *testing.T) []promRule {
	t.Helper()
	raw, err := os.ReadFile(storageAlertRuleFile)
	require.NoError(t, err, "the example alert rules must exist: a gauge with nothing downstream looks like "+
		"coverage and is the opposite")

	var parsed promRuleFile
	require.NoError(t, yaml.Unmarshal(raw, &parsed), "the rule file must be valid YAML")
	require.NotEmpty(t, parsed.Groups)

	var rules []promRule
	for _, group := range parsed.Groups {
		rules = append(rules, group.Rules...)
	}
	require.NotEmpty(t, rules)
	return rules
}

// TestStorageAlertRules_ReferenceOnlyPublishedMetrics: a rule naming a metric the service never
// publishes is silently never satisfied, which looks exactly like a calm account.
func TestStorageAlertRules_ReferenceOnlyPublishedMetrics(t *testing.T) {
	metrics, registry := newTestStorageMetrics(t)

	// Every shape the surface can publish. The critical row carries a measured rate and a
	// projection, because those series exist only for a growing resource.
	metrics.ObserveResource(withGrowth(
		withPressure(resourceRow("CRITICAL", storagereport.TierFile), storagereport.PressureCritical),
		1024, 30*time.Minute))
	metrics.ObserveResource(unboundedRow("UNBOUNDED", storagereport.TierFile))
	metrics.ObserveResource(unknownRow("OFFLINE"))
	// The file tier carries its own growth, projection and pressure as well as the
	// over-commitment verdict: the series every unbounded resource's state is inherited from.
	fileTier := fillingTier(storagereport.TierFile, storagereport.PressureHigh)
	fileTier.State = storagereport.OvercommitmentOver
	metrics.ObserveAccount(storagereport.AccountReport{
		Tiers: []storagereport.TierComparison{
			fileTier,
			{
				Tier:  storagereport.TierMemory,
				Limit: storagereport.NewCapacity(0, 0, true),
				State: storagereport.OvercommitmentNotApplicable,
			},
		},
	})

	families, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)
	published := make(map[string]struct{}, len(families))
	for _, family := range families {
		published[family.GetName()] = struct{}{}
	}

	metricName := regexp.MustCompile(`semengine_storage_[a-z_]+`)
	referenced := 0
	for _, rule := range loadStorageAlertRules(t) {
		for _, name := range metricName.FindAllString(rule.Expr, -1) {
			referenced++
			assert.Contains(t, published, name, "alert %s reads %s, which the service never publishes", rule.Alert, name)
		}
	}
	assert.Greater(t, referenced, 0, "the rule set must actually read the storage metrics")
}

// TestStorageAlertRules_KeyNothingOnRowDisappearance: reclamation is eventually consistent, so a
// row may transiently vanish and return; an alert keyed on its absence would fire on a skew.
func TestStorageAlertRules_KeyNothingOnRowDisappearance(t *testing.T) {
	for _, rule := range loadStorageAlertRules(t) {
		assert.NotContains(t, rule.Expr, "absent(",
			"alert %s keys on absence; a row may transiently vanish and return", rule.Alert)
		assert.NotContains(t, rule.Expr, "absent_over_time(",
			"alert %s keys on absence over a window, which a transiently reclaimed row would satisfy", rule.Alert)
	}
}

// TestStorageAlertRules_MakeUnboundedResourcesVisible: an unbounded resource has no headroom
// series, so a rule names it by its usage series, and another counts the rows carrying no state.
func TestStorageAlertRules_MakeUnboundedResourcesVisible(t *testing.T) {
	var namesUnbounded, countsNotEvaluated bool
	for _, rule := range loadStorageAlertRules(t) {
		expr := rule.Expr
		if strings.Contains(expr, "semengine_storage_resource_used_bytes") &&
			strings.Contains(expr, "unless") &&
			strings.Contains(expr, "semengine_storage_resource_limit_bytes") {
			namesUnbounded = true
		}
		if strings.Contains(expr, pressureStateNotEvaluated) || strings.Contains(expr, `capacity_state="unknown"`) {
			countsNotEvaluated = true
		}
	}
	assert.True(t, namesUnbounded, "a rule must name the resources carrying no bound")
	assert.True(t, countsNotEvaluated, "a rule must surface the rows carrying no pressure state at all")
}

// TestStorageAlertRules_AreAnnotatedAndReportOnly keeps the example from implying enforcement:
// nothing rejects, throttles or degrades because of pressure.
func TestStorageAlertRules_AreAnnotatedAndReportOnly(t *testing.T) {
	reportOnly := 0
	for _, rule := range loadStorageAlertRules(t) {
		require.NotEmpty(t, rule.Alert)
		require.NotEmpty(t, rule.Annotations["summary"], "alert %s needs a summary", rule.Alert)
		require.NotEmpty(t, rule.Annotations["description"], "alert %s needs a description", rule.Alert)
		require.NotEmpty(t, rule.For, "alert %s needs a `for` window; a zero-duration alert flaps on one sample",
			rule.Alert)
		if strings.Contains(rule.Annotations["description"], "report-only") {
			reportOnly++
		}
	}
	assert.Greater(t, reportOnly, 0, "at least one alert must state that pressure rejects nothing")
}
