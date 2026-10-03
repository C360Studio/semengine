package natsclient

import (
	"net"
	"testing"
	"time"
)

// This file is the test-side helper file that replaces the pin's test_client.go, which is not
// ported (its fixture role is internal/harness/natsfixture; admission ledger row
// natsclient/test_client.go). It holds only what a ported test reads.

// Test-stream bounds, from test_client.go:912-913 at the pin, read by client_test.go. A test
// stream declares its bounds rather than taking an exemption from the bounds requirement, so the
// production contract stays exercised by the suite. The values are small on purpose: a runaway
// test fails loudly instead of filling the account.
const (
	testStreamMaxAge   = time.Hour
	testStreamMaxBytes = 64 << 20
)

// closeBudget bounds a test's Close (harness-boundaries, "Bounded cleanup roots"): the pin closed
// under context.Background() at client_connect_test.go:70, :98 and client_test.go:700. It is a
// failure bound, never reached by a correct Close (design D8 R1b).
const closeBudget = 10 * time.Second

// refusedNATSURL returns a nats:// URL on a loopback port that was bound with port 0 and then
// released, so a dial is refused at once: no fixed address and no DNS lookup (design D8 R5). The
// pin dialled nats://invalid-host:4222 at client_test.go:332. A test that never dials uses
// "nats://unused", the pin's own spelling (client_connect_test.go:14).
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
