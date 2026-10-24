//go:build integration

package natsfixture

import (
	"context"
	"errors"
	"net"
	neturl "net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/network"
	"github.com/nats-io/nats.go"
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
		// Every address: plays the #209 Mac program, which holds ports on all addresses.
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

// Requirement: nats-fixture/Phased start in one container; Scenario: Host port taken before Docker binds it
// When another process takes the host port freePort just released before Docker binds it (#345),
// Docker fails ContainerStart with "address already in use". With a live caller context, Start
// removes the first unstarted container, takes a fresh host port, and starts a second container;
// when both attempts hit a held port, or when the caller context is cancelled, Start fails in the
// start phase and leaves no container behind.
func TestStartRetriesOnceOnPortInUse(t *testing.T) {
	// Every address: held on all addresses so Docker's bind of this port fails (#345).
	held1, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen :0: %v", err)
	}
	t.Cleanup(func() { _ = held1.Close() })
	heldPort1 := strconv.Itoa(held1.Addr().(*net.TCPAddr).Port)

	// Every address: held on all addresses so Docker's bind of this port fails (#345).
	held2, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen :0: %v", err)
	}
	t.Cleanup(func() { _ = held2.Close() })
	heldPort2 := strconv.Itoa(held2.Addr().(*net.TCPAddr).Port)

	t.Run("recovers_on_second_attempt", func(t *testing.T) {
		orig := pickPort
		t.Cleanup(func() { pickPort = orig })
		calls := 0
		pickPort = func() (string, error) {
			calls++
			if calls == 1 {
				return heldPort1, nil
			}
			return freePort()
		}

		const limit = 4096
		// Own server: tests Start's port-in-use retry on a fresh container.
		f := New(t, WithMaxPayload(limit))
		if err := f.Start(t.Context()); err != nil {
			t.Fatalf("Start after one held port (%s) = %v, want nil on the second attempt", heldPort1, err)
		}
		if calls != 2 {
			t.Fatalf("pickPort calls = %d, want 2", calls)
		}
		assertExplicitPortBinding(t, f)
		if _, err := f.JetStream().AccountInfo(t.Context()); err != nil {
			t.Fatalf("JetStream AccountInfo on %s: %v", f.URL(), err)
		}
		nc, err := nats.Connect(f.URL(), nats.MaxReconnects(0))
		if err != nil {
			t.Fatalf("dial %s: %v", f.URL(), err)
		}
		defer nc.Close()
		if got := nc.MaxPayload(); got != limit {
			t.Fatalf("broker announces max_payload %d after retry, want %d", got, limit)
		}
		if err := nc.Publish("payload.bound", make([]byte, limit+1)); !errors.Is(err, nats.ErrMaxPayload) {
			t.Fatalf("publish of %d bytes after retry = %v, want nats.ErrMaxPayload", limit+1, err)
		}
		if got := processContainers(t); !slices.Equal(got, []string{f.containerID}) {
			t.Fatalf("containers of this process = %v, want only %s (first attempt container not removed)", got, f.containerID)
		}
	})

	t.Run("fails_after_two_attempts", func(t *testing.T) {
		orig := pickPort
		t.Cleanup(func() { pickPort = orig })
		calls := 0
		pickPort = func() (string, error) {
			calls++
			if calls == 1 {
				return heldPort1, nil
			}
			return heldPort2, nil
		}

		// Own server: tests Start's two-attempt bound when both ports are in use.
		f := New(t)
		err := f.Start(t.Context())
		if err == nil {
			t.Fatal("Start succeeded when both attempts used a held port")
		}
		if calls != 2 {
			t.Fatalf("pickPort calls = %d, want 2", calls)
		}
		for _, want := range []string{"phase start", "(parent context live)", "address already in use", heldPort1, heldPort2} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Start error %q lacks %q", err, want)
			}
		}
		if strings.Contains(err.Error(), "cleanup:") {
			t.Errorf("Start error %q reports a failed cleanup", err)
		}
		if got := processContainers(t); len(got) != 0 {
			t.Fatalf("containers of this process after failed Start = %v, want none", got)
		}
		if !stopHoldsNothing(t, f) {
			t.Fatalf("after failed Start the fixture still holds %v", f.remaining())
		}
	})

	t.Run("no_retry_when_context_cancelled", func(t *testing.T) {
		orig := pickPort
		t.Cleanup(func() { pickPort = orig })
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		calls := 0
		pickPort = func() (string, error) {
			calls++
			cancel()
			return heldPort1, nil
		}

		// Own server: tests that a cancelled context stops Start after the first attempt.
		f := New(t)
		err := f.Start(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Start = %v, want context.Canceled", err)
		}
		if calls != 1 {
			t.Fatalf("pickPort calls = %d, want 1 (no retry after cancellation)", calls)
		}
		if got := processContainers(t); len(got) != 0 {
			t.Fatalf("containers of this process after cancelled Start = %v, want none", got)
		}
		if !stopHoldsNothing(t, f) {
			t.Fatalf("after cancelled Start the fixture still holds %v", f.remaining())
		}
	})

	t.Run("no_retry_on_non_port_in_use_error", func(t *testing.T) {
		orig := pickPort
		t.Cleanup(func() { pickPort = orig })
		calls := 0
		pickPort = func() (string, error) {
			calls++
			return "99999", nil
		}

		// Own server: tests that a Docker start failure other than address-in-use does not retry.
		f := New(t)
		err := f.Start(t.Context())
		if err == nil || !strings.Contains(err.Error(), "phase start") {
			t.Fatalf("Start with invalid port = %v, want a start-phase error", err)
		}
		if calls != 1 {
			t.Fatalf("pickPort calls = %d, want 1 (no retry on non-port-in-use error)", calls)
		}
		if got := processContainers(t); len(got) != 0 {
			t.Fatalf("containers of this process after failed Start = %v, want none", got)
		}
		if !stopHoldsNothing(t, f) {
			t.Fatalf("after failed Start the fixture still holds %v", f.remaining())
		}
	})
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
