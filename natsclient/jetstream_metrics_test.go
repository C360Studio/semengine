package natsclient

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/c360studio/semengine/metric"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

// Task 3.7d test (2), design D9: two clients' metrics on one registry share the canonical
// collector for each of the 11 metrics, and a value written through the second is gathered. At the
// pin the 8 registered through the removed Register* methods were orphans for the second client.
func TestJetStreamMetricsShareCanonicalCollectorsAcrossOwners(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	first, err := newJetStreamMetrics(registry)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newJetStreamMetrics(registry)
	if err != nil {
		t.Fatal(err)
	}

	if first.policyRequested != second.policyRequested {
		t.Fatal("requested policy collector identity differs across JetStream metrics owners")
	}
	if first.policyEffective != second.policyEffective {
		t.Fatal("effective policy collector identity differs across JetStream metrics owners")
	}
	if first.policyAvailable != second.policyAvailable {
		t.Fatal("availability policy collector identity differs across JetStream metrics owners")
	}

	labels := []string{"worker", "events", "EVENTS", "worker-events", policySourcePort}
	second.policyRequested.WithLabelValues(labels...).Set(7)
	second.policyEffective.WithLabelValues(labels...).Set(5)
	second.policyAvailable.WithLabelValues(labels...).Set(1)

	families, err := registry.PrometheusRegistry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	wantValues := map[string]float64{
		"semstreams_jetstream_consumer_max_ack_pending_requested":             7,
		"semstreams_jetstream_consumer_max_ack_pending_effective":             5,
		"semstreams_jetstream_consumer_max_ack_pending_observation_available": 1,
	}
	gotNames := make([]string, 0, len(wantValues))
	for _, family := range families {
		name := family.GetName()
		if strings.HasPrefix(name, "semstreams_jetstream_") &&
			(strings.Contains(name, "queue") || strings.Contains(name, "drop")) {
			t.Fatalf("unexpected queue/drop metric added with consumer-policy observation: %q", name)
		}
		want, isPolicy := wantValues[name]
		if !isPolicy {
			continue
		}
		gotNames = append(gotNames, name)
		if len(family.Metric) != 1 {
			t.Fatalf("metric family %q has %d series, want 1", name, len(family.Metric))
		}
		if got := family.Metric[0].GetGauge().GetValue(); got != want {
			t.Fatalf("metric %q = %v, want %v", name, got, want)
		}
	}
	sort.Strings(gotNames)
	wantNames := make([]string, 0, len(wantValues))
	for name := range wantValues {
		wantNames = append(wantNames, name)
	}
	sort.Strings(wantNames)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("policy metric names = %v, want exact names %v", gotNames, wantNames)
	}

	// The other 8 collectors. Add, which gauges and counters share, writes through the second owner.
	others := []struct {
		name     string
		same     bool
		write    func()
		wantType dto.MetricType
	}{
		{"semstreams_jetstream_stream_messages", first.streamMessages == second.streamMessages,
			func() { second.streamMessages.WithLabelValues("EVENTS").Add(3) }, dto.MetricType_GAUGE},
		{"semstreams_jetstream_stream_bytes", first.streamBytes == second.streamBytes,
			func() { second.streamBytes.WithLabelValues("EVENTS").Add(3) }, dto.MetricType_GAUGE},
		{"semstreams_jetstream_stream_state", first.streamState == second.streamState,
			func() { second.streamState.WithLabelValues("EVENTS").Add(3) }, dto.MetricType_GAUGE},
		{"semstreams_jetstream_consumer_pending_messages", first.consumerPending == second.consumerPending,
			func() { second.consumerPending.WithLabelValues("EVENTS", "worker").Add(3) }, dto.MetricType_GAUGE},
		{"semstreams_jetstream_consumer_delivered_stream_sequence", first.consumerDelivered == second.consumerDelivered,
			func() { second.consumerDelivered.WithLabelValues("EVENTS", "worker").Add(3) }, dto.MetricType_GAUGE},
		{"semstreams_jetstream_consumer_ack_floor_stream_sequence", first.consumerAcked == second.consumerAcked,
			func() { second.consumerAcked.WithLabelValues("EVENTS", "worker").Add(3) }, dto.MetricType_GAUGE},
		{"semstreams_jetstream_consumer_redelivered_messages", first.consumerRedelivered == second.consumerRedelivered,
			func() { second.consumerRedelivered.WithLabelValues("EVENTS", "worker").Add(3) }, dto.MetricType_GAUGE},
		{"semstreams_jetstream_operation_errors_total", first.errors == second.errors,
			func() { second.errors.WithLabelValues("publish").Add(3) }, dto.MetricType_COUNTER},
	}
	for _, o := range others {
		if !o.same {
			t.Errorf("%s: collector identity differs across JetStream metrics owners", o.name)
		}
		o.write()
	}
	families, err = registry.PrometheusRegistry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]*dto.MetricFamily, len(families))
	for _, family := range families {
		byName[family.GetName()] = family
	}
	for _, o := range others {
		family := byName[o.name]
		if family == nil {
			t.Errorf("%s: not gathered", o.name)
			continue
		}
		if family.GetType() != o.wantType || len(family.Metric) != 1 {
			t.Errorf("%s: type %v with %d series, want %v with 1", o.name, family.GetType(), len(family.Metric), o.wantType)
			continue
		}
		if got := sampleValue(family.Metric[0]); got != 3 {
			t.Errorf("%s = %v, want the 3 written through the second owner", o.name, got)
		}
	}
}

// sampleValue is a gauge's or a counter's gathered value.
func sampleValue(m *dto.Metric) float64 {
	if m.GetGauge() != nil {
		return m.GetGauge().GetValue()
	}
	return m.GetCounter().GetValue()
}

// serverConsumerInfo reports info, the server state the test sets between polls.
type serverConsumerInfo struct {
	jetstream.Consumer
	info *jetstream.ConsumerInfo
}

func (f *serverConsumerInfo) Info(context.Context) (*jetstream.ConsumerInfo, error) {
	return f.info, nil
}

// Task 3.7d test (1), design D9: the three consumer metrics report the server's state, so two polls
// of an unchanged consumer gather the server's values, as gauges under their new names. At the pin
// they were counters that added the server's current values on every poll, so a second poll
// doubled them. A third poll follows the server when its state changes: NumRedelivered counts
// messages redelivered and not yet acknowledged, so it falls on acknowledgement.
func TestJetStreamConsumerMetricsReportServerStateAcrossPolls(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	metrics, err := newJetStreamMetrics(registry)
	if err != nil {
		t.Fatal(err)
	}
	server := &serverConsumerInfo{info: &jetstream.ConsumerInfo{
		Stream: "EVENTS", Name: "worker", NumPending: 1, NumRedelivered: 2,
		Delivered: jetstream.SequenceInfo{Stream: 5}, AckFloor: jetstream.SequenceInfo{Stream: 4},
	}}
	metrics.trackConsumer("EVENTS", "worker", server, nil)
	metrics.updateStats(t.Context())
	metrics.updateStats(t.Context())

	initial := []struct {
		name  string
		value float64
	}{
		{"semstreams_jetstream_consumer_delivered_stream_sequence", 5},
		{"semstreams_jetstream_consumer_ack_floor_stream_sequence", 4},
		{"semstreams_jetstream_consumer_redelivered_messages", 2},
	}
	polled := []float64{
		testutil.ToFloat64(metrics.consumerDelivered.WithLabelValues("EVENTS", "worker")),
		testutil.ToFloat64(metrics.consumerAcked.WithLabelValues("EVENTS", "worker")),
		testutil.ToFloat64(metrics.consumerRedelivered.WithLabelValues("EVENTS", "worker")),
	}
	for i, want := range initial {
		if polled[i] != want.value {
			t.Errorf("%s after two polls = %v, want the server's %v", want.name, polled[i], want.value)
		}
	}

	families, err := registry.PrometheusRegistry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]*dto.MetricFamily, len(families))
	for _, family := range families {
		byName[family.GetName()] = family
	}
	for _, want := range initial {
		family := byName[want.name]
		if family == nil || family.GetType() != dto.MetricType_GAUGE || len(family.Metric) != 1 {
			t.Errorf("%s: gathered %v, want one gauge series", want.name, family)
			continue
		}
		if got := family.Metric[0].GetGauge().GetValue(); got != want.value {
			t.Errorf("%s gathered %v, want the server's %v", want.name, got, want.value)
		}
	}
	for _, old := range []string{"semstreams_jetstream_consumer_delivered_total",
		"semstreams_jetstream_consumer_acked_total", "semstreams_jetstream_consumer_redelivered_total"} {
		if byName[old] != nil {
			t.Errorf("%s is still gathered", old)
		}
	}

	// The server's state changes: one more message delivered, the ack floor caught up, and one of the
	// two redelivered messages acknowledged, so NumRedelivered falls from 2 to 1.
	server.info = &jetstream.ConsumerInfo{
		Stream: "EVENTS", Name: "worker", NumRedelivered: 1,
		Delivered: jetstream.SequenceInfo{Stream: 6}, AckFloor: jetstream.SequenceInfo{Stream: 6},
	}
	metrics.updateStats(t.Context())
	changed := []struct {
		name  string
		got   float64
		value float64
	}{
		{"consumer_delivered_stream_sequence", testutil.ToFloat64(metrics.consumerDelivered.WithLabelValues("EVENTS", "worker")), 6},
		{"consumer_ack_floor_stream_sequence", testutil.ToFloat64(metrics.consumerAcked.WithLabelValues("EVENTS", "worker")), 6},
		{"consumer_redelivered_messages", testutil.ToFloat64(metrics.consumerRedelivered.WithLabelValues("EVENTS", "worker")), 1},
	}
	for _, c := range changed {
		if c.got != c.value {
			t.Errorf("%s after the server's state changed = %v, want the server's %v", c.name, c.got, c.value)
		}
	}
}

type policyInfoSequence struct {
	results []policyInfoResult
	next    int
}

type policyInfoResult struct {
	info *jetstream.ConsumerInfo
	err  error
}

func (s *policyInfoSequence) Info(context.Context) (*jetstream.ConsumerInfo, error) {
	result := s.results[s.next]
	if s.next < len(s.results)-1 {
		s.next++
	}
	return result.info, result.err
}

func TestJetStreamPolicyMetricsRemoveStaleEffectiveAndRecover(t *testing.T) {
	metrics, err := newJetStreamMetrics(metric.NewMetricsRegistry())
	if err != nil {
		t.Fatal(err)
	}
	handle := &policyInfoSequence{results: []policyInfoResult{
		{err: errors.New("unavailable")},
		{info: &jetstream.ConsumerInfo{Config: jetstream.ConsumerConfig{MaxAckPending: 8}}},
	}}
	record := &consumerPolicyRecord{
		component: "worker", port: "events", stream: "EVENTS", consumer: "worker-events",
		policySource: policySourcePort, requested: 8, handle: handle, available: true, active: true,
	}
	key := record.key()
	metrics.trackPolicy(key, record, 8)
	labels := record.labels()

	metrics.updateStats(t.Context())
	if got := testutil.ToFloat64(metrics.policyRequested.WithLabelValues(labels...)); got != 8 {
		t.Fatalf("requested = %v, want 8", got)
	}
	if got := testutil.ToFloat64(metrics.policyAvailable.WithLabelValues(labels...)); got != 0 {
		t.Fatalf("availability after failure = %v, want 0", got)
	}
	if count := testutil.CollectAndCount(metrics.policyEffective); count != 0 {
		t.Fatalf("effective series count after failure = %d, want 0", count)
	}

	metrics.updateStats(t.Context())
	if got := testutil.ToFloat64(metrics.policyAvailable.WithLabelValues(labels...)); got != 1 {
		t.Fatalf("availability after recovery = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.policyEffective.WithLabelValues(labels...)); got != 8 {
		t.Fatalf("effective after recovery = %v, want 8", got)
	}

	metrics.forgetPolicy(key)
	count := testutil.CollectAndCount(metrics.policyRequested) +
		testutil.CollectAndCount(metrics.policyEffective) +
		testutil.CollectAndCount(metrics.policyAvailable)
	if count != 0 {
		t.Fatalf("policy series count after forget = %d, want 0", count)
	}
}

type blockingPolicyInfo struct {
	started chan struct{}
	release chan struct{}
}

type blockingTrackedConsumer struct {
	jetstream.Consumer
	started chan struct{}
	release chan struct{}
}

func (b *blockingTrackedConsumer) Info(context.Context) (*jetstream.ConsumerInfo, error) {
	close(b.started)
	<-b.release
	return &jetstream.ConsumerInfo{Stream: "EVENTS", Name: "internal-events"}, nil
}

func TestJetStreamConsumerClosedCannotBeUndoneByInflightRefresh(t *testing.T) {
	metrics, err := newJetStreamMetrics(metric.NewMetricsRegistry())
	if err != nil {
		t.Fatal(err)
	}
	handle := &blockingTrackedConsumer{started: make(chan struct{}), release: make(chan struct{})}
	metrics.trackConsumer("EVENTS", "internal-events", handle, nil)
	done := make(chan struct{})
	go func() {
		metrics.updateStats(t.Context())
		close(done)
	}()
	<-handle.started
	metrics.forgetConsumer("EVENTS", "internal-events")
	close(handle.release)
	<-done

	count := testutil.CollectAndCount(metrics.consumerPending) +
		testutil.CollectAndCount(metrics.consumerDelivered) +
		testutil.CollectAndCount(metrics.consumerAcked) +
		testutil.CollectAndCount(metrics.consumerRedelivered)
	if count != 0 {
		t.Fatalf("in-flight refresh recreated %d Closed consumer series", count)
	}
}

func (b *blockingPolicyInfo) Info(context.Context) (*jetstream.ConsumerInfo, error) {
	close(b.started)
	<-b.release
	return &jetstream.ConsumerInfo{Config: jetstream.ConsumerConfig{MaxAckPending: 5}}, nil
}

func TestJetStreamPolicyStopCannotBeUndoneByInflightRefresh(t *testing.T) {
	metrics, err := newJetStreamMetrics(metric.NewMetricsRegistry())
	if err != nil {
		t.Fatal(err)
	}
	handle := &blockingPolicyInfo{started: make(chan struct{}), release: make(chan struct{})}
	record := &consumerPolicyRecord{
		component: "worker", port: "events", stream: "EVENTS", consumer: "worker-events",
		policySource: policySourcePort, requested: 5, handle: handle, available: true, active: true,
	}
	key := record.key()
	metrics.trackPolicy(key, record, 5)

	done := make(chan struct{})
	go func() {
		metrics.updateStats(t.Context())
		close(done)
	}()
	<-handle.started
	metrics.forgetPolicy(key)
	close(handle.release)
	<-done

	count := testutil.CollectAndCount(metrics.policyRequested) +
		testutil.CollectAndCount(metrics.policyEffective) +
		testutil.CollectAndCount(metrics.policyAvailable)
	if count != 0 {
		t.Fatalf("in-flight refresh recreated %d stopped policy series", count)
	}
}

func TestJetStreamPolicyReplacementCannotBeUndoneByInflightRefresh(t *testing.T) {
	metrics, err := newJetStreamMetrics(metric.NewMetricsRegistry())
	if err != nil {
		t.Fatal(err)
	}
	oldHandle := &blockingPolicyInfo{started: make(chan struct{}), release: make(chan struct{})}
	oldRecord := &consumerPolicyRecord{
		component: "worker", port: "events", stream: "EVENTS", consumer: "worker-events",
		policySource: policySourcePort, requested: 5, handle: oldHandle, available: true, active: true,
	}
	key := oldRecord.key()
	metrics.trackPolicy(key, oldRecord, 5)

	done := make(chan struct{})
	go func() {
		metrics.updateStats(t.Context())
		close(done)
	}()
	<-oldHandle.started
	newRecord := &consumerPolicyRecord{
		component: "worker", port: "events", stream: "EVENTS", consumer: "worker-events",
		policySource: policySourcePort, requested: 9, available: true, active: true,
		handle: &policyInfoSequence{results: []policyInfoResult{
			{info: &jetstream.ConsumerInfo{Config: jetstream.ConsumerConfig{MaxAckPending: 9}}},
		}},
	}
	metrics.trackPolicy(key, newRecord, 9)
	close(oldHandle.release)
	<-done

	labels := newRecord.labels()
	if got := testutil.ToFloat64(metrics.policyRequested.WithLabelValues(labels...)); got != 9 {
		t.Fatalf("requested after replacement = %v, want 9", got)
	}
	if got := testutil.ToFloat64(metrics.policyEffective.WithLabelValues(labels...)); got != 9 {
		t.Fatalf("effective after replacement = %v, want 9", got)
	}
}

func TestJetStreamPolicyIdentityKeepsSiblingAndReplacesOnlyExactRecord(t *testing.T) {
	metrics, err := newJetStreamMetrics(metric.NewMetricsRegistry())
	if err != nil {
		t.Fatal(err)
	}
	newRecord := func(stream, consumer string, requested int) *consumerPolicyRecord {
		return &consumerPolicyRecord{
			component: "worker", port: "events", stream: stream, consumer: consumer,
			policySource: policySourcePort, requested: requested, available: true, active: true,
			handle: &policyInfoSequence{results: []policyInfoResult{
				{info: &jetstream.ConsumerInfo{Config: jetstream.ConsumerConfig{MaxAckPending: requested}}},
			}},
		}
	}
	first := newRecord("EVENTS_A", "worker-a", 5)
	sibling := newRecord("EVENTS_B", "worker-b", 7)
	metrics.trackPolicy(first.key(), first, 5)
	metrics.trackPolicy(sibling.key(), sibling, 7)
	if len(metrics.policies) != 2 {
		t.Fatalf("policy records = %d, want two coexisting instances", len(metrics.policies))
	}

	replacement := newRecord("EVENTS_A", "worker-a", 9)
	metrics.trackPolicy(replacement.key(), replacement, 9)
	if len(metrics.policies) != 2 {
		t.Fatalf("policy records after exact replacement = %d, want 2", len(metrics.policies))
	}
	if first.active {
		t.Fatal("exact prior record remains active after replacement")
	}
	if !sibling.active {
		t.Fatal("sibling instance was deactivated by another identity replacement")
	}
	if got := testutil.ToFloat64(metrics.policyRequested.WithLabelValues(sibling.labels()...)); got != 7 {
		t.Fatalf("sibling requested = %v, want 7", got)
	}
	if got := testutil.ToFloat64(metrics.policyRequested.WithLabelValues(replacement.labels()...)); got != 9 {
		t.Fatalf("replacement requested = %v, want 9", got)
	}
}

// switchableConsumerInfo reports info, or err when it is set, as the test switches between polls.
type switchableConsumerInfo struct {
	jetstream.Consumer
	info *jetstream.ConsumerInfo
	err  error
}

func (s *switchableConsumerInfo) Info(context.Context) (*jetstream.ConsumerInfo, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.info, nil
}

// TestJetStreamConsumerMetricsDropSeriesWhenInfoFails (PR #48 comment 5985648705, HIGH 1): when a
// tracked consumer's Info fails, its four gauges are deleted rather than left holding the last
// poll's values as if current (each is set from the server's current state, design D9), and the
// failure is counted on the JetStream error metric each poll, as operation consumer_info. A later
// successful poll sets them again.
func TestJetStreamConsumerMetricsDropSeriesWhenInfoFails(t *testing.T) {
	metrics, err := newJetStreamMetrics(metric.NewMetricsRegistry())
	if err != nil {
		t.Fatal(err)
	}
	logs := newRecordingHandler("")
	handle := &switchableConsumerInfo{info: &jetstream.ConsumerInfo{
		Stream: "EVENTS", Name: "worker", NumPending: 3, NumRedelivered: 1,
		Delivered: jetstream.SequenceInfo{Stream: 7}, AckFloor: jetstream.SequenceInfo{Stream: 6},
	}}
	metrics.trackConsumer("EVENTS", "worker", handle, slog.New(logs))
	gauges := map[string]*prometheus.GaugeVec{
		"pending": metrics.consumerPending, "delivered": metrics.consumerDelivered,
		"acked": metrics.consumerAcked, "redelivered": metrics.consumerRedelivered,
	}
	series := func() map[string]int {
		out := map[string]int{}
		for name, g := range gauges {
			out[name] = testutil.CollectAndCount(g)
		}
		return out
	}
	metrics.updateStats(t.Context())
	for name, n := range series() {
		if n != 1 {
			t.Fatalf("%s series after a good poll = %d, want 1", name, n)
		}
	}

	handle.err = errors.New("consumer info unavailable")
	metrics.updateStats(t.Context())
	metrics.updateStats(t.Context())
	for name, n := range series() {
		if n != 0 {
			t.Errorf("%s series after Info failed = %d, want 0 (a stale value reads as current)", name, n)
		}
	}
	if got := testutil.ToFloat64(metrics.errors.WithLabelValues("consumer_info")); got != 2 {
		t.Errorf("consumer_info errors after two failed polls = %v, want 2", got)
	}
	warned := logs.find(func(r loggedRecord) bool {
		return r.level == slog.LevelWarn && r.attrs["consumer"] == "worker" && r.attrs["stream"] == "EVENTS"
	})
	if len(warned) != 1 {
		t.Errorf("warnings for the unavailable consumer = %d, want one per transition", len(warned))
	}

	handle.err = nil
	metrics.updateStats(t.Context())
	if got := testutil.ToFloat64(metrics.consumerDelivered.WithLabelValues("EVENTS", "worker")); got != 7 {
		t.Errorf("delivered after recovery = %v, want the server's 7", got)
	}
}
