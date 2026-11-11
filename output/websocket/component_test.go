package websocket

import (
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/natsclient"
)

// freePort picks a loopback port for the output and releases it, as the smoke test's freePort
// does; the output then binds 127.0.0.1 on it.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick a port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release port %d: %v", port, err)
	}
	return port
}

// subjectConfig is a configuration streaming subject on 127.0.0.1:port at /ws, pinging every
// interval.
func subjectConfig(t *testing.T, subject string, port int, interval string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"ping_interval": interval,
		"ports": component.PortConfig{
			Inputs:  []component.PortDefinition{{Name: "frames", Config: component.NATSPort{Subject: subject}}},
			Outputs: []component.PortDefinition{server(port)},
		},
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return string(raw)
}

// TestStartFailureReleasesPort: a Start that fails after binding its port (here the subscribe, on a
// NATS client that never connected) releases the port and holds nothing (design D3; output-websocket
// spec, "Start bounds start; Stop is the only stop").
func TestStartFailureReleasesPort(t *testing.T) {
	port := freePort(t)
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	built, err := create(json.RawMessage(subjectConfig(t, "boids.frames", port, "30s")), component.Dependencies{NATSClient: client})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	c := built.(*output)
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Start(t.Context()); !errors.Is(err, natsclient.ErrNotConnected) {
		t.Fatalf("Start = %v; want the subscribe's ErrNotConnected", err)
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	if conn, err := net.Dial("tcp4", address); err == nil {
		_ = conn.Close()
		t.Fatalf("dial %s after a failed Start connected; want it refused", address)
	}
	if held := (suiteOwner{c}).Observe().Unresolved; len(held) != 0 {
		t.Fatalf("after a failed Start the component holds %v; want nothing", held)
	}
}

// TestClientQueueDropsOldest: a client's queue holds the newest 256 frames; each frame queued while
// it is full drops the oldest, reported to the caller, and the writer takes the rest oldest first
// (design D2, the stalled-client row).
func TestClientQueueDropsOldest(t *testing.T) {
	cl := newClient(nil)
	const queued = 300
	dropped := 0
	for i := range queued {
		if cl.enqueue([]byte(strconv.Itoa(i))) {
			dropped++
		}
	}
	if dropped != queued-256 {
		t.Fatalf("enqueue reported %d drops; want %d", dropped, queued-256)
	}
	for want := queued - 256; want < queued; want++ {
		frame, ok := cl.next()
		if !ok || string(frame) != strconv.Itoa(want) {
			t.Fatalf("next = %q, %v; want frame %d", frame, ok, want)
		}
	}
	if frame, ok := cl.next(); ok {
		t.Fatalf("next after the queue emptied = %q; want none", frame)
	}
}
