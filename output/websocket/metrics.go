package websocket

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semengine/metric"
)

// The reasons a client's connection ends, the client_disconnections_total label.
// The first to happen is the one counted.
const (
	// disconnectReadTimeout: the client answered no ping before its read deadline.
	disconnectReadTimeout = "read_timeout"
	// disconnectClientClosed: the client's connection failed a read for any other reason: the
	// client sent a close frame, closed its connection, or broke the protocol.
	disconnectClientClosed = "client_closed"
	// disconnectWriteFailed: a frame or ping could not be written to the client.
	disconnectWriteFailed = "write_failed"
	// disconnectStopped: Stop closed the client with 1001.
	disconnectStopped = "stopped"
	// disconnectWatchLost: the applied-state view lost its watch and closed the client with 1013.
	disconnectWatchLost = "watch_lost"
)

// The kinds of server error, the errors_total type label.
const (
	// errorUpgrade: a request to the path could not be upgraded and was answered with an HTTP error.
	errorUpgrade = "upgrade"
	// errorServe: the server stopped serving before Stop.
	errorServe = "serve"
)

// messageSizeBuckets are the message_size_bytes bucket bounds, in bytes.
var messageSizeBuckets = []float64{100, 500, 1000, 2000, 5000, 10000, 25000}

// outputMetrics are one output's semengine_websocket_* collectors. Every one is
// labelled listener, so instances on one registry stay apart; the single-label ones are resolved
// for this output's listener at construction.
type outputMetrics struct {
	received    prometheus.Counter
	sent        prometheus.Counter
	bytesSent   prometheus.Counter
	sizes       prometheus.Observer
	connected   prometheus.Gauge
	connections prometheus.Counter
	// coalesced is changes_coalesced_total: applied-state changes merged into a later one for a
	// client that was behind (graphview's Hooks.OnFanOut overwritten count).
	coalesced prometheus.Counter
	// disconnections is labelled listener and reason, errors listener and type, dropped listener
	// and reason.
	disconnections *prometheus.CounterVec
	errors         *prometheus.CounterVec
	dropped        *prometheus.CounterVec
}

// newOutputMetrics builds the collectors and registers each on registry, the injected
// component.Dependencies.MetricsRegistry, through metric.RegisterOrGet, keeping the collector it
// returns, so instances on one registry share it. With a nil registry they are registered nowhere.
func newOutputMetrics(registry *metric.MetricsRegistry, listener string) (*outputMetrics, error) {
	r := registrar{registry: registry}
	m := &outputMetrics{
		received: r.counter("messages_received_total",
			"Messages the input received, whether or not they were sent: subject messages, or applied-state writes").WithLabelValues(listener),
		sent: r.counter("messages_sent_total",
			"Data frames written to a client, once per client").WithLabelValues(listener),
		bytesSent: r.counter("bytes_sent_total",
			"Bytes of the data frames written to clients").WithLabelValues(listener),
		sizes: register(&r, "message_size_bytes", prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "semengine", Subsystem: key, Name: "message_size_bytes",
			Help:    "Size of each data frame written to a client, in bytes",
			Buckets: messageSizeBuckets,
		}, []string{"listener"})).WithLabelValues(listener),
		connected: register(&r, "clients_connected", prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "semengine", Subsystem: key, Name: "clients_connected",
			Help: "Clients connected now",
		}, []string{"listener"})).WithLabelValues(listener),
		connections: r.counter("client_connections_total",
			"Requests upgraded to a WebSocket client").WithLabelValues(listener),
		coalesced: r.counter("changes_coalesced_total",
			"Applied-state changes merged into a later change of the same key because a client was behind").WithLabelValues(listener),
		disconnections: r.counter("client_disconnections_total",
			"Clients whose connection ended, by reason: read_timeout, client_closed, write_failed, stopped, watch_lost", "reason"),
		errors: r.counter("errors_total",
			"Server errors, by type: upgrade (a request that could not be upgraded), serve (the server stopped before Stop)", "type"),
		dropped: r.counter("messages_dropped_total",
			"Frames not sent, by reason: not_json (a subject message that is not JSON), client_queue_full (a client's oldest queued frame)", "reason"),
	}
	if r.err != nil {
		return nil, r.err
	}
	return m, nil
}

// registrar registers collectors on one registry, keeping the first error.
type registrar struct {
	registry *metric.MetricsRegistry
	err      error
}

// counter builds and registers the counter vector semengine_websocket_<name>, labelled listener and
// then labels.
func (r *registrar) counter(name, help string, labels ...string) *prometheus.CounterVec {
	return register(r, name, prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine", Subsystem: key, Name: name, Help: help,
	}, append([]string{"listener"}, labels...)))
}

// register registers candidate as name and returns the collector to use: the registry's, or
// candidate when there is no registry or registration failed, the failure kept in r.err.
func register[C prometheus.Collector](r *registrar, name string, candidate C) C {
	if r.registry == nil || r.err != nil {
		return candidate
	}
	registered, err := metric.RegisterOrGet(r.registry, key, name, candidate)
	if err != nil {
		r.err = err
		return candidate
	}
	return registered
}
