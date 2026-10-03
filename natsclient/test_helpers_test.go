package natsclient

import "time"

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
