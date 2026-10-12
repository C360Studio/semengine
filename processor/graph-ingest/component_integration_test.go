//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/pkg/errs"
)

// The six tests in this file skipped in the pin's component_test.go ("requires real
// NATS connection - move to integration tests", :578-596, :638-650, :662-677,
// :679-690, :701-715, :944-961). They run here against a broker of their own, with no
// skip (harness-boundaries, "No skipped or hidden tests"). Each of the pin's two sleeps
// is replaced by a wait on what the test observes, and each Stop runs under a bounded
// context (task cleanup-roots:check).

// componentTestTimeout bounds each test's operations against the broker.
const componentTestTimeout = 30 * time.Second

// startableComponent stands in for the pin's createTestComponent here: graph-ingest with
// its default configuration, built on a fixture broker that holds the ENTITY stream,
// and not yet initialized.
func startableComponent(t *testing.T) *Component {
	t.Helper()
	natsClient := newFixtureClient(t, entityStream)
	configJSON, err := json.Marshal(DefaultConfig())
	require.NoError(t, err)
	comp, err := CreateGraphIngest(configJSON, testDependencies(t, natsClient))
	require.NoError(t, err)
	return comp.(*Component)
}

func TestComponent_Health_Running(t *testing.T) {
	comp := startableComponent(t)
	ctx, cancel := context.WithTimeout(t.Context(), componentTestTimeout)
	defer cancel()
	owner := newGraphIngestTestOwner(comp)
	defer owner.finish(ctx, t)

	require.NoError(t, comp.Initialize())
	require.NoError(t, comp.Start(owner.startContext(ctx)))

	// The pin slept 100ms here to "allow time for component to become healthy". The
	// test waits for that instead: the first Health that reports healthy, with uptime
	// past Start.
	health, err := probe.Await(ctx,
		func(context.Context) (component.HealthStatus, error) { return comp.Health(), nil },
		func(h component.HealthStatus) bool { return h.Healthy && h.Uptime > 0 })
	require.NoError(t, err, "component should be healthy, with uptime, when running")

	assert.True(t, health.Healthy, "component should be healthy when running")
	assert.Greater(t, health.Uptime, time.Duration(0))
}

func TestComponent_Start_Success(t *testing.T) {
	comp := startableComponent(t)
	ctx, cancel := context.WithTimeout(t.Context(), componentTestTimeout)
	defer cancel()
	owner := newGraphIngestTestOwner(comp)
	defer owner.finish(ctx, t)

	require.NoError(t, comp.Initialize())
	err := comp.Start(owner.startContext(ctx))

	assert.NoError(t, err)
}

func TestComponent_Start_AlreadyStarted(t *testing.T) {
	comp := startableComponent(t)
	ctx, cancel := context.WithTimeout(t.Context(), componentTestTimeout)
	defer cancel()
	owner := newGraphIngestTestOwner(comp)
	defer owner.finish(ctx, t)

	require.NoError(t, comp.Initialize())
	require.NoError(t, comp.Start(owner.startContext(ctx)))

	// Start is one-shot, even while the first generation is running.
	err := comp.Start(ctx)

	assert.ErrorIs(t, err, errs.ErrAlreadyStarted)
}

func TestComponent_Stop_Success(t *testing.T) {
	comp := startableComponent(t)

	require.NoError(t, comp.Initialize())
	require.NoError(t, comp.Start(t.Context()))

	ctx, cancel := context.WithTimeout(t.Context(), componentTestTimeout)
	defer cancel()
	err := comp.Stop(ctx)

	assert.NoError(t, err)
}

// TestComponent_Stop_Timeout: the pin stopped under context.Background(), beside a
// comment promising a very short timeout, and asserted nothing ("should either succeed
// quickly or timeout gracefully"). Here the timeout is one already past. Such a Stop
// returns its context's error and releases nothing, and the next Stop, given time,
// completes the shutdown (design D13; internal/lifecycleguard's Stop).
func TestComponent_Stop_Timeout(t *testing.T) {
	comp := startableComponent(t)

	require.NoError(t, comp.Initialize())
	require.NoError(t, comp.Start(t.Context()))

	expired, cancelExpired := context.WithDeadline(t.Context(), time.Unix(1, 0))
	defer cancelExpired()
	err := comp.Stop(expired)
	require.ErrorIs(t, err, context.DeadlineExceeded, "a Stop out of time reports its deadline")
	assert.Equal(t, "running", comp.Health().Status, "a Stop out of time releases nothing")

	ctx, cancel := context.WithTimeout(t.Context(), componentTestTimeout)
	defer cancel()
	require.NoError(t, comp.Stop(ctx), "the next Stop, given time, completes the shutdown")
	assert.Equal(t, "stopped", comp.Health().Status)
}

// TestComponent_RespectsContext_Cancellation: the pin built this component on a mock KV
// bucket, which Start's initStorage replaces with the broker's; here it is built on the
// broker alone.
func TestComponent_RespectsContext_Cancellation(t *testing.T) {
	comp := startableComponent(t)
	startCtx, cancelStart := context.WithCancel(t.Context())

	require.NoError(t, comp.Initialize())
	require.NoError(t, comp.Start(startCtx))

	// Cancel context
	cancelStart()

	// The pin slept 100ms here to "allow time for cancellation to propagate". The test
	// waits for it instead: the status loop Start runs under its context returns once
	// that context is cancelled.
	comp.handlesMu.Lock()
	statusDone := comp.statusDone
	comp.handlesMu.Unlock()
	require.NotNil(t, statusDone, "Start must establish a status producer")
	ctx, cancel := context.WithTimeout(t.Context(), componentTestTimeout)
	defer cancel()
	select {
	case <-statusDone:
	case <-ctx.Done():
		t.Fatalf("the status loop had not returned %s after Start's context was cancelled", componentTestTimeout)
	}

	// Component should handle cancellation gracefully
	err := comp.Stop(ctx)
	assert.NoError(t, err)
}
