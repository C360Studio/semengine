//go:build integration

package websocket

import (
	"net/http"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/metric"
)

// TestMetricsCountTraffic: two outputs on one registry count their own traffic apart by listener.
// Output a serves two clients and gets three JSON messages, one that is not JSON and a request that
// is not an upgrade; output b serves two clients, one of which leaves, and gets one JSON message.
// Every semengine_websocket_* metric but changes_coalesced_total (the applied-state input's) is
// checked against that traffic: bytes against the frames the clients read.
func TestMetricsCountTraffic(t *testing.T) {
	client := fixtureClient(t)
	registry := metric.NewMetricsRegistry()
	deps := component.Dependencies{NATSClient: client, MetricsRegistry: registry}
	a := started(t, newOutputWith(t, "ws.metrics.a", "30s", deps))
	b := started(t, newOutputWith(t, "ws.metrics.b", "30s", deps))

	aClients := []*wsClient{dial(t, a.url(), nil, true), dial(t, a.url(), nil, true)}
	bLive := dial(t, b.url(), nil, true)
	bLeaving := dialConn(t, websocket.DefaultDialer, b.url(), nil)
	awaitClients(t, a.c, 2)
	awaitClients(t, b.c, 2)
	// attach sets clients_connected under the lock awaitClients reads the clients under.
	requireValue(t, a, 2, "clients_connected")
	requireValue(t, b, 2, "clients_connected")

	if err := bLeaving.Close(); err != nil {
		t.Fatalf("close b's leaving client: %v", err)
	}
	// detach counts the disconnection and sets clients_connected before it removes the client.
	awaitClients(t, b.c, 1)
	requireValue(t, b, 1, "clients_connected")
	requireValue(t, b, 1, "client_disconnections_total", "reason", "client_closed")

	aBytes, bBytes := 0, 0
	publish(t, client, "ws.metrics.a", []byte("not json"))
	for _, payload := range []string{`{"n":1}`, `{"n":22}`, `{"n":"three"}`} {
		publish(t, client, "ws.metrics.a", []byte(payload))
		for _, ws := range aClients {
			frame := ws.next(t)
			if got := decodeData(t, frame); string(got.Payload) != payload {
				t.Fatalf("a's frame payload %s; want %s", got.Payload, payload)
			}
			aBytes += len(frame)
		}
	}
	publish(t, client, "ws.metrics.b", []byte(`{"b":true}`))
	bBytes += len(bLive.next(t))

	notUpgrade, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+a.listener+"/ws", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	notUpgrade.Close = true
	resp, err := http.DefaultClient.Do(notUpgrade)
	if err != nil {
		t.Fatalf("GET %s: %v", notUpgrade.URL, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET without upgrade = %d; want 400", resp.StatusCode)
	}

	// Stop drains the subscription and joins every client's handler and writer, so every count
	// below has been made once it returns.
	stopOutput(t, a.c)
	stopOutput(t, b.c)
	for _, want := range []struct {
		o      instance
		value  float64
		name   string
		labels []string
	}{
		{a, 4, "messages_received_total", nil},
		{b, 1, "messages_received_total", nil},
		{a, 6, "messages_sent_total", nil},
		{b, 1, "messages_sent_total", nil},
		{a, float64(aBytes), "bytes_sent_total", nil},
		{b, float64(bBytes), "bytes_sent_total", nil},
		{a, 0, "clients_connected", nil},
		{b, 0, "clients_connected", nil},
		{a, 2, "client_connections_total", nil},
		{b, 2, "client_connections_total", nil},
		{a, 2, "client_disconnections_total", []string{"reason", "stopped"}},
		{b, 1, "client_disconnections_total", []string{"reason", "stopped"}},
		{a, 0, "client_disconnections_total", []string{"reason", "client_closed"}},
		{b, 1, "client_disconnections_total", []string{"reason", "client_closed"}},
		{a, 1, "errors_total", []string{"type", "upgrade"}},
		{b, 0, "errors_total", []string{"type", "upgrade"}},
		{a, 1, "messages_dropped_total", []string{"reason", reasonNotJSON}},
		{b, 0, "messages_dropped_total", []string{"reason", reasonNotJSON}},
	} {
		requireValue(t, want.o, want.value, want.name, want.labels...)
	}
	requireSizes(t, a, 6, aBytes)
	requireSizes(t, b, 1, bBytes)
}

// requireValue fails the test unless o's name sample with labels has value want.
func requireValue(t *testing.T, o instance, want float64, name string, labels ...string) {
	t.Helper()
	if got := value(t, o, name, labels...); got != want {
		t.Fatalf("%s%v for %s = %v; want %v", name, labels, o.listener, got, want)
	}
}

// requireSizes fails the test unless o's message_size_bytes holds count frames of bytes in total.
func requireSizes(t *testing.T, o instance, count uint64, bytes int) {
	t.Helper()
	m, err := sample(o, "message_size_bytes")
	if err != nil || m == nil {
		t.Fatalf("message_size_bytes for %s = %v, %v; want a sample", o.listener, m, err)
	}
	if h := m.GetHistogram(); h.GetSampleCount() != count || h.GetSampleSum() != float64(bytes) {
		t.Fatalf("message_size_bytes for %s: %d frames, %v bytes; want %d, %d",
			o.listener, h.GetSampleCount(), h.GetSampleSum(), count, bytes)
	}
}
