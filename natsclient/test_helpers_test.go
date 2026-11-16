package natsclient

import (
	"net"
	"testing"
	"time"
)

// This file holds the unit tests' shared helpers; the broker fixture is
// internal/harness/natsfixture. It holds only what a test here reads.

// Test-stream bounds, read by client_test.go. A test stream declares its bounds rather than taking
// an exemption from the bounds requirement, so the production contract stays exercised by the
// suite. The values are small on purpose: a runaway test fails loudly instead of filling the
// account.
const (
	testStreamMaxAge   = time.Hour
	testStreamMaxBytes = 64 << 20
)

// closeBudget bounds a test's Close (harness-boundaries, "Bounded cleanup roots"): a Close under
// context.Background() could hang cleanup forever. It is a failure bound, never reached by a
// correct Close.
const closeBudget = 10 * time.Second

// refusedNATSURL returns a nats:// URL on a loopback port that was bound with port 0 and then
// released, so a dial is refused at once: no fixed address and no DNS lookup. A test that never
// dials uses "nats://unused".
func refusedNATSURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "nats://" + listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return url
}
