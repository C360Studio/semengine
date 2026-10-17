//go:build integration

package natsclient

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
)

// TestHealthMonitorTickReturnsStatusAndClose proves that a health tick writes the status and that
// Close then returns: the tick calls Status() while it holds mu (client.go, startHealthMonitoring),
// so a Status() that took mu would hang the tick and every later Close (design D2). The test marks
// a healthy connection Disconnected itself, as kv_key_contract_integration_test.go stores private
// state; only a tick writes Connected back on a healthy connection.
//
// Connect, the wait and both Close calls each run beside a deadline of the test's own, so a call
// hung on mu fails the test on that deadline instead of hanging the run.
func TestHealthMonitorTickReturnsStatusAndClose(t *testing.T) {
	url := shared.Reset(t)
	client, err := NewClient(url,
		WithTimeout(fixtureClientTimeout),
		WithMaxReconnects(0),
		WithHealthInterval(5*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	closed := false
	t.Cleanup(func() {
		if closed {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
		defer cancel()
		if err := withinDeadline(ctx, func() error { return client.Close(ctx) }); err != nil {
			t.Errorf("cleanup Close: %v", err)
		}
	})
	connectCtx, cancelConnect := context.WithTimeout(t.Context(), fixtureClientTimeout)
	defer cancelConnect()
	if err := withinDeadline(connectCtx, func() error { return client.Connect(connectCtx) }); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	client.status.Store(int32(StatusDisconnected))

	waitCtx, cancelWait := context.WithTimeout(t.Context(), failureBound)
	defer cancelWait()
	if err := withinDeadline(waitCtx, func() error {
		_, err := probe.Await(waitCtx,
			func(context.Context) (ConnectionStatus, error) { return client.Status(), nil },
			func(s ConnectionStatus) bool { return s == StatusConnected })
		return err
	}); err != nil {
		t.Fatalf("status did not return to Connected through a health tick: %v", err)
	}

	closeCtx, cancelClose := context.WithTimeout(t.Context(), closeBudget)
	defer cancelClose()
	if err := withinDeadline(closeCtx, func() error { return client.Close(closeCtx) }); err != nil {
		t.Fatalf("Close after a health tick: %v", err)
	}
	closed = true
}

// withinDeadline runs f on its own goroutine and returns f's error, or ctx's error once ctx ends
// first. A call that blocks on a lock its context cannot reach still fails the test on time; its
// goroutine is left blocked, which only a hung call reaches.
func withinDeadline(ctx context.Context, f func() error) error {
	done := make(chan error, 1)
	go func() { done <- f() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}
