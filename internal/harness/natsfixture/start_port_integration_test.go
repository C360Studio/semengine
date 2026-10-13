//go:build integration

package natsfixture

import (
	"net"
	neturl "net/url"
	"strconv"
	"testing"
)

// heldPorts is how many host ports after a running fixture's own TestStartPastHeldPorts holds: one
// for each of Start's two containers, and two for containers other runs start meanwhile.
const heldPorts = 4

// Left to pick, Docker Desktop on macOS publishes a container's port on the next host port of a
// counter of its own, which does not see the Mac's listeners; when a Mac process listens on all
// addresses on that port, the container runs with an empty port list, and both of Start's
// containers can land on held ports (#209; the probe on PR #210, comment 6151736554). This test
// holds the ports after a running fixture's own on all addresses, then starts a second fixture.
// When Docker picked the first fixture's port, the held ports are the ones it picks next; when the
// fixture picked it, they are the ones the Mac hands out next. Start returning nil is the oracle:
// it returns only once JetStream answers on the URL it reports. Measured on macOS only.
func TestStartPastHeldPorts(t *testing.T) {
	u, err := neturl.Parse(startFixture(t).URL())
	if err != nil {
		t.Fatal(err)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	for next := p + 1; next <= p+heldPorts; next++ {
		held, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(next)))
		if err != nil {
			t.Logf("port %d not held by this test: %v", next, err)
			continue
		}
		t.Cleanup(func() { _ = held.Close() })
	}
	t.Logf("held ports %d-%d; second fixture at %s", p+1, p+heldPorts, startFixture(t).URL())
}
