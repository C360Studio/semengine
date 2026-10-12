package natsfixture

import (
	"context"
	"testing"
	"time"
)

const (
	// openBudget bounds the caller's open function: a connect and its first round trip to a broker
	// that is already answering. The value is natsclient's fixture-client timeout, which the pin's
	// NewTestClient family used.
	openBudget = 15 * time.Second
	// closeBudget bounds the close Open registers. The test's own context is already cancelled
	// when cleanups run, so the close gets fresh finite authority.
	closeBudget = 10 * time.Second
)

// Open gives a package's tests a value connected to a started fixture, such as a client built
// with that package's own options (nats-fixture › "Connected value for a package's tests"). It
// calls open with the fixture's URL under a bounded context derived from the test's, and returns
// the value. When open fails, Open fails the test naming the URL and the error and registers
// nothing; open releases whatever it acquired before failing. On success it registers close on
// the test's cleanup under a fresh bounded context, reporting a close error as a test error.
//
// Cleanups run last-registered first, so close runs before the Stop that New registered: call
// Open after New. Open starts no goroutine and imports nothing of this module outside
// internal/harness, so a package under test can use it without an import cycle.
func Open[T any](t testing.TB, f *Fixture, open func(ctx context.Context, url string) (T, func(context.Context) error, error)) T {
	t.Helper()
	var zero T
	url := f.URL()
	if url == "" {
		t.Fatalf("natsfixture: Open on a fixture that is not started")
		return zero
	}
	ctx, cancel := context.WithTimeout(t.Context(), openBudget)
	defer cancel()
	value, closeFn, err := open(ctx, url)
	if err != nil {
		t.Fatalf("natsfixture: open against %s: %v", url, err)
		return zero
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
		defer cancel()
		if err := closeFn(ctx); err != nil {
			t.Errorf("natsfixture: close of the value opened against %s: %v", url, err)
		}
	})
	return value
}
