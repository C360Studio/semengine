//go:build integration

package natsfixture

import (
	"net"
	neturl "net/url"
	"testing"

	"github.com/moby/moby/api/types/network"
)

// heldPorts is how many wildcard listeners TestStartPastHeldPorts holds between the two fixtures.
const heldPorts = 4

// Left to pick, Docker Desktop on macOS publishes a container's port on the next host port of a
// counter of its own, which does not see the Mac's listeners; when a Mac process listens on all
// addresses on that port, the container runs with an empty port list (#209; the probe on PR #210,
// comment 6151736554). This test starts one fixture, holds the next wildcard ports the operating
// system hands out via ":0" (never binding p+1..p+4 by number, which can steal a port the kernel
// handed to a concurrent run while the first container was starting, #254), and then starts a
// second fixture. Each fixture's HostConfig.PortBindings must carry the explicit host port of its
// URL rather than Docker's "0", and the second Start must return nil with JetStream answering on
// its URL().
func TestStartPastHeldPorts(t *testing.T) {
	first := startFixture(t)
	assertExplicitPortBinding(t, first)

	var heldAddrs []string
	for range heldPorts {
		held, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("listen :0: %v", err)
		}
		t.Cleanup(func() { _ = held.Close() })
		heldAddrs = append(heldAddrs, held.Addr().String())
	}

	second := startFixture(t)
	assertExplicitPortBinding(t, second)
	t.Logf("first fixture at %s; held %v; second fixture at %s", first.URL(), heldAddrs, second.URL())
}

func assertExplicitPortBinding(t *testing.T, f *Fixture) {
	t.Helper()
	u, err := neturl.Parse(f.URL())
	if err != nil {
		t.Fatalf("parse URL %q: %v", f.URL(), err)
	}
	ins, err := f.container.Inspect(t.Context())
	if err != nil {
		t.Fatalf("inspect container %s: %v", shortID(f.containerID), err)
	}
	bindings := ins.HostConfig.PortBindings[network.MustParsePort(clientPort)]
	if len(bindings) != 1 || bindings[0].HostPort != u.Port() {
		t.Fatalf("container %s HostConfig.PortBindings[%s] = %+v, want explicit HostPort %q",
			shortID(f.containerID), clientPort, bindings, u.Port())
	}
}
