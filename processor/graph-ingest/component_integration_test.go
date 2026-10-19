//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/message"
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

// TestComponent_StartContextEndStopsNothing replaces the pin's
// TestComponent_RespectsContext_Cancellation, which asserted the old rule: that ending
// Start's context ends the component's work. Under ADR-113 the Start context bounds Start
// only. After Start returns and its context is cancelled, the component still ingests an
// entity from its input stream into ENTITY_STATES and answers an entity query; only Stop
// ends its work, and Stop joins the status loop. The oracle is the bucket itself, read
// through the test's own JetStream handle.
func TestComponent_StartContextEndStopsNothing(t *testing.T) {
	comp := startableComponent(t)
	ctx, cancel := context.WithTimeout(t.Context(), componentTestTimeout)
	defer cancel()
	startCtx, cancelStart := context.WithCancel(ctx)
	defer cancelStart()

	require.NoError(t, comp.Initialize())
	registerMergeTestPayload(t, comp) // decoder before Start: no consumer race
	require.NoError(t, comp.Start(startCtx))
	stopped := false
	defer func() {
		if !stopped {
			stopCtx, cancelStop := context.WithTimeout(context.WithoutCancel(ctx), componentTestTimeout)
			defer cancelStop()
			assert.NoError(t, comp.Stop(stopCtx), "terminal Stop")
		}
	}()
	statusDone := comp.statusDone // written by Start, which has returned
	require.NotNil(t, statusDone, "Start must establish a status producer")

	cancelStart()

	const entityID = "c360.platform.robotics.mav1.drone.startctx001"
	payload := &mergeTestGraphable{entityID: entityID, triples: []message.Triple{{
		Subject: entityID, Predicate: "test.startctx.value", Object: "v", Source: "start-context-test",
		Timestamp: time.Now(), Confidence: 1.0,
	}}}
	data, err := json.Marshal(message.NewBaseMessage(payload.Schema(), payload, "start-context-test"))
	require.NoError(t, err)
	require.NoError(t, comp.natsClient.PublishToStream(ctx, "entity."+entityID, data))

	js, err := comp.natsClient.JetStream()
	require.NoError(t, err)
	entities, err := js.KeyValue(ctx, graph.BucketEntityStates)
	require.NoError(t, err)
	_, err = probe.Await(ctx,
		func(ctx context.Context) (jetstream.KeyValueEntry, error) { return entities.Get(ctx, entityID) },
		func(entry jetstream.KeyValueEntry) bool { return entry != nil })
	require.NoError(t, err, "the entity published after the Start context ended never reached ENTITY_STATES")

	queryData, err := json.Marshal(map[string]string{"id": entityID})
	require.NoError(t, err)
	_, err = comp.natsClient.RequestClassified(ctx, "graph.ingest.query.entity", queryData, wireRequestTimeout)
	require.NoError(t, err, "an entity query after the Start context ended")

	select {
	case <-statusDone:
		t.Fatal("the status loop returned when the Start context ended")
	default:
	}

	stopped = true
	require.NoError(t, comp.Stop(ctx), "Stop")
	select {
	case <-statusDone:
	default:
		t.Fatal("Stop returned nil while the status loop still ran")
	}
}
