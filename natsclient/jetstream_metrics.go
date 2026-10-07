package natsclient

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/c360studio/semengine/metric"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
)

// jetstreamMetrics holds Prometheus metrics for JetStream operations.
// Tracks only streams and consumers that are created/accessed through this client.
type jetstreamMetrics struct {
	// Stream state metrics
	streamMessages *prometheus.GaugeVec // Current message count by stream
	streamBytes    *prometheus.GaugeVec // Storage bytes by stream
	streamState    *prometheus.GaugeVec // Stream state (1=active, 0=inactive)

	// Consumer state metrics
	// Each gauge is set from the server's current consumer state on each poll (design D9), never
	// added to: some of these values fall (pending, and redelivered messages once acknowledged).
	consumerPending     *prometheus.GaugeVec // Pending messages by consumer
	consumerDelivered   *prometheus.GaugeVec // Stream sequence last delivered (Delivered.Stream)
	consumerAcked       *prometheus.GaugeVec // Stream sequence of the ack floor (AckFloor.Stream)
	consumerRedelivered *prometheus.GaugeVec // Messages redelivered (NumRedelivered)
	policyRequested     *prometheus.GaugeVec
	policyEffective     *prometheus.GaugeVec
	policyAvailable     *prometheus.GaugeVec

	// Operation errors
	errors *prometheus.CounterVec // JetStream operation errors

	// Tracked resources (only what we create/use)
	mu        sync.RWMutex
	streams   map[string]jetstream.Stream // Streams we've created/accessed
	consumers map[string]*trackedConsumer // Consumers we've created
	policies  map[consumerPolicyKey]*consumerPolicyRecord
}

type trackedConsumer struct {
	handle        jetstream.Consumer
	stream, name  string
	logger        *slog.Logger
	infoAvailable bool // the last poll's Info succeeded; guarded by jetstreamMetrics.mu
}

// newJetStreamMetrics creates and registers JetStream metrics with the provided registry.
func newJetStreamMetrics(registry *metric.MetricsRegistry) (*jetstreamMetrics, error) {
	if registry == nil {
		return nil, nil // Metrics disabled
	}

	m := &jetstreamMetrics{
		// Stream metrics
		streamMessages: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "semengine",
			Subsystem: "jetstream",
			Name:      "stream_messages",
			Help:      "Current number of messages in stream",
		}, []string{"stream"}),

		streamBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "semengine",
			Subsystem: "jetstream",
			Name:      "stream_bytes",
			Help:      "Storage bytes used by stream",
		}, []string{"stream"}),

		streamState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "semengine",
			Subsystem: "jetstream",
			Name:      "stream_state",
			Help:      "Stream state (1=active, 0=inactive)",
		}, []string{"stream"}),

		// Consumer metrics
		consumerPending: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "semengine",
			Subsystem: "jetstream",
			Name:      "consumer_pending_messages",
			Help:      "Number of pending messages for consumer",
		}, []string{"stream", "consumer"}),

		consumerDelivered: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "semengine",
			Subsystem: "jetstream",
			Name:      "consumer_delivered_stream_sequence",
			Help:      "Stream sequence of the last message delivered to consumer, as the server reports it",
		}, []string{"stream", "consumer"}),

		consumerAcked: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "semengine",
			Subsystem: "jetstream",
			Name:      "consumer_ack_floor_stream_sequence",
			Help:      "Stream sequence of consumer's ack floor, as the server reports it",
		}, []string{"stream", "consumer"}),

		consumerRedelivered: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "semengine",
			Subsystem: "jetstream",
			Name:      "consumer_redelivered_messages",
			Help:      "Messages redelivered to consumer and not yet acknowledged, as the server reports it; falls on acknowledgement",
		}, []string{"stream", "consumer"}),
		policyRequested: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "semengine", Subsystem: "jetstream", Name: "consumer_max_ack_pending_requested",
			Help: "Final requested MaxAckPending for a port-backed consumer",
		}, []string{"component", "port", "stream", "consumer", "policy_source"}),
		policyEffective: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "semengine", Subsystem: "jetstream", Name: "consumer_max_ack_pending_effective",
			Help: "Observed effective MaxAckPending for a port-backed consumer",
		}, []string{"component", "port", "stream", "consumer", "policy_source"}),
		policyAvailable: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "semengine", Subsystem: "jetstream", Name: "consumer_max_ack_pending_observation_available",
			Help: "Whether effective MaxAckPending observation is currently available",
		}, []string{"component", "port", "stream", "consumer", "policy_source"}),

		// Error counters
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "semengine",
			Subsystem: "jetstream",
			Name:      "operation_errors_total",
			Help:      "Total number of JetStream operation errors",
		}, []string{"operation"}),

		streams:   make(map[string]jetstream.Stream),
		consumers: make(map[string]*trackedConsumer),
		policies:  make(map[consumerPolicyKey]*consumerPolicyRecord),
	}

	// Register all metrics through metric.RegisterOrGet and keep the collector it returns: a
	// second client on the same registry then writes to the canonical collector, not an orphan
	// (design D9).
	var err error
	// The third argument is the registry key, the handle Unregister takes; it is not the metric
	// name, which is the collector's own (Namespace_Subsystem_Name above).
	if m.streamMessages, err = metric.RegisterOrGet(registry, "jetstream", "stream_messages", m.streamMessages); err != nil {
		return nil, err
	}
	if m.streamBytes, err = metric.RegisterOrGet(registry, "jetstream", "stream_bytes", m.streamBytes); err != nil {
		return nil, err
	}
	if m.streamState, err = metric.RegisterOrGet(registry, "jetstream", "stream_state", m.streamState); err != nil {
		return nil, err
	}
	if m.consumerPending, err = metric.RegisterOrGet(registry, "jetstream", "consumer_pending", m.consumerPending); err != nil {
		return nil, err
	}
	if m.consumerDelivered, err = metric.RegisterOrGet(registry, "jetstream", "consumer_delivered", m.consumerDelivered); err != nil {
		return nil, err
	}
	if m.consumerAcked, err = metric.RegisterOrGet(registry, "jetstream", "consumer_acked", m.consumerAcked); err != nil {
		return nil, err
	}
	if m.consumerRedelivered, err = metric.RegisterOrGet(registry, "jetstream", "consumer_redelivered", m.consumerRedelivered); err != nil {
		return nil, err
	}
	if m.policyRequested, err = metric.RegisterOrGet(registry, "jetstream", "consumer_max_ack_pending_requested", m.policyRequested); err != nil {
		return nil, err
	}
	if m.policyEffective, err = metric.RegisterOrGet(registry, "jetstream", "consumer_max_ack_pending_effective", m.policyEffective); err != nil {
		return nil, err
	}
	if m.policyAvailable, err = metric.RegisterOrGet(registry, "jetstream", "consumer_max_ack_pending_observation_available", m.policyAvailable); err != nil {
		return nil, err
	}
	if m.errors, err = metric.RegisterOrGet(registry, "jetstream", "errors", m.errors); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *jetstreamMetrics) trackPolicy(key consumerPolicyKey, record *consumerPolicyRecord, effective int) {
	if m == nil || key == (consumerPolicyKey{}) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.policies[key]; ok {
		m.deactivatePolicy(existing)
		delete(m.policies, key)
	}
	record.mu.Lock()
	record.active = true
	record.mu.Unlock()
	m.policies[key] = record
	labels := record.labels()
	m.policyRequested.WithLabelValues(labels...).Set(float64(record.requested))
	m.policyEffective.WithLabelValues(labels...).Set(float64(effective))
	m.policyAvailable.WithLabelValues(labels...).Set(1)
}

func (m *jetstreamMetrics) forgetPolicy(key consumerPolicyKey) {
	if m == nil || key == (consumerPolicyKey{}) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if record, ok := m.policies[key]; ok {
		m.deactivatePolicy(record)
		delete(m.policies, key)
	}
}

func (m *jetstreamMetrics) deactivatePolicy(record *consumerPolicyRecord) {
	record.mu.Lock()
	defer record.mu.Unlock()
	record.active = false
	labels := record.labels()
	m.policyRequested.DeleteLabelValues(labels...)
	m.policyEffective.DeleteLabelValues(labels...)
	m.policyAvailable.DeleteLabelValues(labels...)
}

// trackStream adds a stream to the tracking list for metrics collection.
func (m *jetstreamMetrics) trackStream(name string, stream jetstream.Stream) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streams[name] = stream
	m.streamState.WithLabelValues(name).Set(1) // Mark as active
}

// trackConsumer adds a consumer to the tracking list for metrics collection.
func (m *jetstreamMetrics) trackConsumer(streamName, consumerName string, consumer jetstream.Consumer, logger *slog.Logger) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := streamName + ":" + consumerName
	m.consumers[key] = &trackedConsumer{
		handle: consumer, stream: streamName, name: consumerName, logger: logger, infoAvailable: true,
	}
}

// forgetConsumer removes generic observation only after the exact native
// ConsumeContext reports Closed. It has no lifecycle authority over the handle.
func (m *jetstreamMetrics) forgetConsumer(streamName, consumerName string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.consumers, streamName+":"+consumerName)
	m.consumerPending.DeleteLabelValues(streamName, consumerName)
	m.consumerDelivered.DeleteLabelValues(streamName, consumerName)
	m.consumerAcked.DeleteLabelValues(streamName, consumerName)
	m.consumerRedelivered.DeleteLabelValues(streamName, consumerName)
}

// recordError records a JetStream operation error.
func (m *jetstreamMetrics) recordError(operation string) {
	if m != nil {
		m.errors.WithLabelValues(operation).Inc()
	}
}

// updateStats updates all tracked stream and consumer statistics.
// Called periodically by the background poller. Fails gracefully if stats unavailable.
func (m *jetstreamMetrics) updateStats(ctx context.Context) {
	if m == nil {
		return
	}

	m.mu.RLock()
	streams := make(map[string]jetstream.Stream, len(m.streams))
	consumers := make(map[string]*trackedConsumer, len(m.consumers))
	policies := make(map[consumerPolicyKey]*consumerPolicyRecord, len(m.policies))
	for k, v := range m.streams {
		streams[k] = v
	}
	for k, v := range m.consumers {
		consumers[k] = v
	}
	for k, v := range m.policies {
		policies[k] = v
	}
	m.mu.RUnlock()

	// Update stream stats
	for name, stream := range streams {
		info, err := stream.Info(ctx)
		if err != nil {
			// Stream might be deleted or unavailable - fail gracefully
			m.streamState.WithLabelValues(name).Set(0)
			continue
		}

		m.streamMessages.WithLabelValues(name).Set(float64(info.State.Msgs))
		m.streamBytes.WithLabelValues(name).Set(float64(info.State.Bytes))
		m.streamState.WithLabelValues(name).Set(1)
	}

	// Update consumer stats
	for key, consumer := range consumers {
		info, err := consumer.handle.Info(ctx)
		if err != nil {
			// The consumer may be deleted or unreachable. Its gauges are set from the server's current
			// state (design D9), so they are deleted rather than left holding the last poll's values
			// as if current; the failure is counted each poll and logged once per transition, as the
			// policy loop below does. A later successful poll sets them again.
			m.mu.Lock()
			if m.consumers[key] == consumer {
				m.consumerPending.DeleteLabelValues(consumer.stream, consumer.name)
				m.consumerDelivered.DeleteLabelValues(consumer.stream, consumer.name)
				m.consumerAcked.DeleteLabelValues(consumer.stream, consumer.name)
				m.consumerRedelivered.DeleteLabelValues(consumer.stream, consumer.name)
				m.errors.WithLabelValues("consumer_info").Inc()
				if consumer.infoAvailable && consumer.logger != nil {
					consumer.logger.Warn("JetStream consumer state unavailable; its gauges are removed until it answers",
						"stream", consumer.stream, "consumer", consumer.name, "error", err)
				}
				consumer.infoAvailable = false
			}
			m.mu.Unlock()
			continue
		}

		streamName := info.Stream
		consumerName := info.Name

		m.mu.Lock()
		if m.consumers[key] != consumer {
			m.mu.Unlock()
			continue
		}
		m.consumerPending.WithLabelValues(streamName, consumerName).Set(float64(info.NumPending))
		m.consumerDelivered.WithLabelValues(streamName, consumerName).Set(float64(info.Delivered.Stream))
		m.consumerAcked.WithLabelValues(streamName, consumerName).Set(float64(info.AckFloor.Stream))
		m.consumerRedelivered.WithLabelValues(streamName, consumerName).Set(float64(info.NumRedelivered))
		consumer.infoAvailable = true
		m.mu.Unlock()
	}
	for _, record := range policies {
		info, err := record.handle.Info(ctx)
		record.mu.Lock()
		if !record.active {
			record.mu.Unlock()
			continue
		}
		labels := record.labels()
		if err != nil {
			m.policyEffective.DeleteLabelValues(labels...)
			m.policyAvailable.WithLabelValues(labels...).Set(0)
			if record.available && record.logger != nil {
				record.logger.Warn("JetStream consumer acknowledgement policy observation unavailable",
					"component", record.component, "port", record.port, "stream", record.stream, "consumer", record.consumer)
			}
			record.available = false
			record.mu.Unlock()
			continue
		}
		m.policyEffective.WithLabelValues(labels...).Set(float64(info.Config.MaxAckPending))
		m.policyAvailable.WithLabelValues(labels...).Set(1)
		record.available = true
		record.mu.Unlock()
	}
}

// runPoller polls JetStream stats every interval until ctx ends. The client
// runs it on a goroutine it starts through startBackground and joins in Close.
func (m *jetstreamMetrics) runPoller(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Update stats, but don't let errors crash the poller
			m.updateStats(ctx)
		case <-ctx.Done():
			return
		}
	}
}
