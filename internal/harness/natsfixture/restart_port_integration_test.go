//go:build integration

package natsfixture

import (
	"context"
	"net"
	neturl "net/url"
	"strconv"
	"testing"

	"github.com/nats-io/nats.go"
)

// Docker Desktop on macOS hands out host ports in sequence, and when a Mac process listens on all
// addresses on the port it picks, the container's port list comes back empty (#180; PR #200,
// comment 6128094108). This test holds the port after the fixture's own, so the restart's first
// pick is taken, then restarts and dials URL(). On Linux, dockerd binds the host port itself and
// the held port plays no part.
func TestRestartPastAHeldPort(t *testing.T) {
	f := startFixture(t)
	u, err := neturl.Parse(f.URL())
	if err != nil {
		t.Fatalf("URL %q: %v", f.URL(), err)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("port of %q: %v", f.URL(), err)
	}
	next := strconv.Itoa(p + 1)
	held, err := net.Listen("tcp", net.JoinHostPort("", next))
	if err != nil {
		t.Logf("port %s not held by this test (%v): this run may not reach the held-port path", next, err)
	} else {
		defer func() { _ = held.Close() }()
	}
	old := f.URL()
	restart(t, f)
	// Two stop-container calls mean the restart's first port read failed and it tried once more.
	t.Logf("binding %s -> %s, port %s held, stop-container calls %d", old, f.URL(), next, f.callCounts()["stopContainer"])

	nc, err := nats.Connect(f.URL(), nats.MaxReconnects(0))
	if err != nil {
		t.Fatalf("dial %s after restart: %v", f.URL(), err)
	}
	defer nc.Close()
	flushCtx, cancel := context.WithTimeout(t.Context(), stopBound) // nats.go demands a deadline here
	defer cancel()
	if err := nc.FlushWithContext(flushCtx); err != nil {
		t.Fatalf("round trip on the new binding: %v", err)
	}
}
