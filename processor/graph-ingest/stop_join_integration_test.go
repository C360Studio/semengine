//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
)

// TestStopJoinsCallbackAfterConnectionLoss holds graph-ingest-recovery, "Stop returns only after every
// input callback has returned" (#441): a Stop never returns nil while a consume callback still runs,
// even after the broker went away for good while the callback was in it. The callback is held by
// pausing graph-ingest's logger on the warning deliver logs for a message it cannot decode. The
// client does not reconnect (openFixtureClient), so the broker's restart closes its connection, and
// the consume loop's native Closed() then reports the end while the callback is still held
// (natsclient/owned_delivery.go:217-225), which the test checks before it stops. A Stop whose context
// ends first returns that error and keeps the inputs; once the callback returns, a later Stop
// returns nil and holds nothing (lifecycle-suite, retained handles).
func TestStopJoinsCallbackAfterConnectionLoss(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	// Own server: the test takes the broker away under a running Component.
	f := natsfixture.New(t)
	require.NoError(t, f.Start(ctx))
	_, err := natsfixture.CreateStream(ctx, f.JetStream(), jetstream.FileStorage, entityStream.name, entityStream.subjects...)
	require.NoError(t, err)
	client := natsfixture.Open(t, f.URL(), openFixtureClient)

	pause := newPauseOnRecord("graph-ingest: poison message; terminating")
	defer pause.Release()
	cfgJSON, err := json.Marshal(DefaultConfig())
	require.NoError(t, err)
	deps := testDependencies(t, client, withAuthority("c360", "test"))
	deps.Logger = slog.New(pause)
	created, err := CreateGraphIngest(cfgJSON, deps)
	require.NoError(t, err)
	c := created.(*Component)
	owner := newSuiteOwner(c)
	require.NoError(t, c.Initialize())
	require.NoError(t, c.Start(ctx))
	defer func() {
		pause.Release()
		stopCtx, stopCancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer stopCancel()
		if err := c.Stop(stopCtx); err != nil {
			t.Errorf("cleanup Stop: %v", err)
		}
	}()

	// One message the callback cannot decode: it logs the poison warning and is held there.
	require.NoError(t, client.PublishToStream(ctx, "entity.poison", []byte("not a message")))
	pause.awaitEntered(ctx, t, 30*time.Second)

	closedConn := client.GetConnection().StatusChanged(nats.CLOSED)
	require.NoError(t, f.Restart(ctx))
	select {
	case <-closedConn:
	case <-ctx.Done():
		t.Fatal("the client's connection never closed after the broker went away")
	}
	// The trigger: every consume loop reports its end while the callback is still held.
	for _, binding := range c.consumers {
		select {
		case <-binding.handle.Closed():
		case <-ctx.Done():
			t.Fatal("a consume loop's Closed() never fired; the scenario needs it to")
		}
	}

	// The callback is held until pause.Release, so this Stop can only end by its context.
	stopCtx, stopCancel := context.WithTimeout(ctx, time.Second)
	err = c.Stop(stopCtx)
	stopCancel()
	if err == nil {
		t.Fatal("Stop returned nil while a consume callback is still running")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop error = %v, want the expired context's", err)
	}
	if held := owner.Observe().Unresolved; len(held) == 0 {
		t.Fatal("a Stop whose context ended released the inputs; cleanup must stay pending for a later Stop")
	}

	pause.Release()
	retryCtx, retryCancel := context.WithTimeout(ctx, 30*time.Second)
	defer retryCancel()
	if err := c.Stop(retryCtx); err != nil {
		t.Fatalf("Stop after the callback returned: %v", err)
	}
	if held := owner.Observe().Unresolved; len(held) != 0 {
		t.Fatalf("after a Stop returned nil the Component still holds %v", held)
	}
}
