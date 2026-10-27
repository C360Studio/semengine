//go:build integration

package graphingest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
)

// awaitHealthAfterTick publishes a status tick now and reads Health, until done accepts it or
// ten seconds pass. Each observation runs its own tick: the running status loop can land a
// compute it began earlier after the test's call (design D3), and the next observation's tick
// then overwrites it, so the wait never depends on the loop's cadence.
func awaitHealthAfterTick(ctx context.Context, t *testing.T, c *Component, done func(component.HealthStatus) bool) (component.HealthStatus, error) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return probe.Await(waitCtx, func(ctx context.Context) (component.HealthStatus, error) {
		c.refreshReadinessStatus(ctx)
		return c.Health(), nil
	}, done)
}

// boundIngestConsumer returns the stream and durable name of graph-ingest's one bound
// consumer and that consumer's configuration as the server holds it.
func boundIngestConsumer(ctx context.Context, t *testing.T, c *Component) (jetstream.JetStream, boundConsumer, jetstream.ConsumerConfig) {
	t.Helper()
	bound := c.boundConsumerSnapshot()
	require.Len(t, bound, 1, "the default config binds one stream consumer")
	js, err := c.natsClient.JetStream()
	require.NoError(t, err)
	consumer, err := js.Consumer(ctx, bound[0].stream, bound[0].name)
	require.NoError(t, err)
	info, err := consumer.Info(ctx)
	require.NoError(t, err)
	return js, bound[0], info.Config
}

// healthNamesBacklog reports whether a Health result is unhealthy and names the backlog
// observation of consumer.
func healthNamesBacklog(h component.HealthStatus, consumer string) bool {
	return !h.Healthy && strings.Contains(h.LastError, "backlog") && strings.Contains(h.LastError, consumer)
}

// TestIntegration_HealthReadsLostConsumer is #216's test (owner, #216 comment 6249597351;
// design D3): with graph-ingest running, its durable consumer is deleted through the
// JetStream API, and after the next status tick Health is unhealthy and names the backlog
// observation. It asserts unhealthy only: the consumer is not rebound (#216 stays open).
func TestIntegration_HealthReadsLostConsumer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	_, c, owner := startIngestForReadiness(ctx, t)
	defer owner.finish(ctx, t)
	js, bound, _ := boundIngestConsumer(ctx, t, c)

	require.NoError(t, js.DeleteConsumer(ctx, bound.stream, bound.name))

	health, err := awaitHealthAfterTick(ctx, t, c, func(h component.HealthStatus) bool {
		return healthNamesBacklog(h, bound.name)
	})
	require.NoError(t, err, "Health must read the lost consumer as unhealthy and name the backlog observation")
	require.Equal(t, "degraded", health.Status)
}

// consumeLoopClosed returns the Closed channel of the consume loop graph-ingest started for
// its one bound consumer. Call it after Start returned and before Stop: Start writes the
// binding and Stop clears it.
func consumeLoopClosed(t *testing.T, c *Component) <-chan struct{} {
	t.Helper()
	require.Len(t, c.consumers, 1, "the default config binds one stream consumer")
	return c.consumers[0].handle.Closed()
}

// awaitLoopEnded waits until the consume loop behind closed has ended. nats.go ends it
// on the terminal status a deletion sends, at a time it does not promise, so the test
// waits on the closure itself before it creates anything again: the scenario is a loop
// that has ended, not a race with one that is ending.
func awaitLoopEnded(ctx context.Context, t *testing.T, closed <-chan struct{}) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case <-closed:
	case <-waitCtx.Done():
		t.Fatalf("the consume loop did not end after the deletion: %v", waitCtx.Err())
	}
}

// healthNamesRestart reports whether a Health result is unhealthy, names consumer and says
// the process must be restarted: the lost input binding's reading.
func healthNamesRestart(h component.HealthStatus, consumer string) bool {
	return !h.Healthy && strings.Contains(h.LastError, consumer) && strings.Contains(h.LastError, "restart")
}

// TestIntegration_LoopEndedByStreamRecreatedStaysUnhealthy is graph-ingest-recovery,
// "Stream deleted and created again while the broker stays up" (#216, design D1 case d
// then e). Deleting the stream ends graph-ingest's consume loop, and nothing binds a new
// one, so Health stays unhealthy until the process restarts: once the stream is created
// again, and once a consumer is created again under graph-ingest's durable name.
func TestIntegration_LoopEndedByStreamRecreatedStaysUnhealthy(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	_, c, owner := startIngestForReadiness(ctx, t)
	defer owner.finish(ctx, t)
	js, bound, config := boundIngestConsumer(ctx, t, c)
	loopClosed := consumeLoopClosed(t, c)

	require.NoError(t, js.DeleteStream(ctx, bound.stream))
	awaitLoopEnded(ctx, t, loopClosed)
	_, err := natsfixture.CreateStream(ctx, js, jetstream.FileStorage, entityStream.name, entityStream.subjects...)
	require.NoError(t, err)

	_, err = awaitHealthAfterTick(ctx, t, c, func(h component.HealthStatus) bool {
		return !h.Healthy && strings.Contains(h.LastError, bound.name)
	})
	require.NoError(t, err, "Health must read the ended loop as unhealthy and name the consumer")
	status := c.computeBacklogStatus(ctx)
	require.Equal(t, graph.IndexStateDegraded, status.State, "the readiness envelope must be degraded: %+v", status)

	_, err = js.CreateConsumer(ctx, bound.stream, config)
	require.NoError(t, err)
	// Fresh ticks after the creation: the lookup now succeeds, and only the recorded lost
	// binding keeps Health unhealthy. Each read follows its tick, and every one must be
	// unhealthy: a wait that accepts the first match would pass a record a good lookup
	// clears, whenever the status loop's own tick records it again just before a read.
	// The envelope is the one each tick computes, so the lost binding alone must keep it
	// degraded and not ready.
	for range 3 {
		status := c.computeBacklogStatus(ctx)
		health := c.Health()
		require.True(t, healthNamesRestart(health, bound.name), "Health must stay unhealthy and say the process must be restarted once a consumer is created again: %+v", health)
		require.Equal(t, "degraded", health.Status)
		require.Equal(t, graph.IndexStateDegraded, status.State, "the readiness envelope must stay degraded once a consumer is created again: %+v", status)
		require.False(t, status.Ready, "the readiness envelope must stay not ready once a consumer is created again: %+v", status)
	}
}

// TestIntegration_HealthStaysStuckAfterConsumerRecreated is graph-ingest-recovery,
// "Consumer created again after it was deleted" (#216, design D1 case f). Deleting
// graph-ingest's durable consumer ends its consume loop; a consumer created again with the
// same stream, name and configuration is found by the next tick's lookup, but nothing
// consumes from it, so Health stays unhealthy.
func TestIntegration_HealthStaysStuckAfterConsumerRecreated(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	_, c, owner := startIngestForReadiness(ctx, t)
	defer owner.finish(ctx, t)
	js, bound, config := boundIngestConsumer(ctx, t, c)
	loopClosed := consumeLoopClosed(t, c)

	require.NoError(t, js.DeleteConsumer(ctx, bound.stream, bound.name))
	_, err := awaitHealthAfterTick(ctx, t, c, func(h component.HealthStatus) bool {
		return healthNamesBacklog(h, bound.name)
	})
	require.NoError(t, err, "Health must read the lost consumer as unhealthy")
	awaitLoopEnded(ctx, t, loopClosed)

	_, err = js.CreateConsumer(ctx, bound.stream, config)
	require.NoError(t, err)
	// Fresh ticks after the creation, whose lookups succeed. Each read follows its tick, and
	// every one must be unhealthy: a wait that accepts the first match would pass a record a
	// good lookup clears, whenever the status loop's own tick records it again just before a
	// read.
	// The envelope is the one each tick computes, so the lost binding alone must keep it
	// degraded and not ready.
	for range 3 {
		status := c.computeBacklogStatus(ctx)
		health := c.Health()
		require.True(t, !health.Healthy && strings.Contains(health.LastError, bound.name), "Health must stay unhealthy once a consumer is created again: %+v", health)
		require.Equal(t, graph.IndexStateDegraded, status.State, "the readiness envelope must stay degraded once a consumer is created again: %+v", status)
		require.False(t, status.Ready, "the readiness envelope must stay not ready once a consumer is created again: %+v", status)
	}
}

// TestIntegration_HealthClearsAfterConnectionInterruption is graph-ingest-recovery, "The
// observation succeeds again" (design, Tests T5). Only graph-ingest's connection is cut,
// through the relay: the broker, the file stream and the durable consumer stay, so the
// consume loop survives and a failed lookup is not a lost binding. A tick under the cut
// reads unhealthy; once the client is connected again a fresh tick reads healthy, and a
// message published after the restore is applied.
func TestIntegration_HealthClearsAfterConnectionInterruption(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()

	owner, client, relay := newRelayedComponent(t)
	c := owner.c
	registerMergeTestPayload(t, c)
	require.NoError(t, owner.Start(ctx))
	defer func() {
		stopCtx, cancelStop := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancelStop()
		if err := owner.Stop(stopCtx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}()
	defer restoreUntilConnected(t, relay, client) // runs before Stop: a drain on a reconnecting connection fails
	bound := c.boundConsumerSnapshot()
	require.Len(t, bound, 1, "the default config binds one stream consumer")
	loopClosed := consumeLoopClosed(t, c)

	cutUntilReconnecting(t, relay, client)
	// The cut tick runs under a context with no deadline. nats.go then bounds the lookup
	// with its own API timeout, so the lookup fails while this context is live and the
	// fault is stored; under awaitHealthAfterTick's bound the context would end first and
	// the tick would take the shutdown path, storing nothing.
	c.refreshReadinessStatus(t.Context())
	health := c.Health()
	require.True(t, healthNamesBacklog(health, bound[0].name), "a tick under the cut must read unhealthy and name the backlog observation: %+v", health)

	restoreUntilConnected(t, relay, client)
	health, err := awaitHealthAfterTick(ctx, t, c, func(h component.HealthStatus) bool { return h.Healthy })
	require.NoError(t, err, "Health must be healthy again once the observation succeeds: %+v", health)
	require.Empty(t, health.LastError)

	const entityID = "c360.platform.test.health.drone.001"
	publishEntity(ctx, t, client, entityID)
	applyCtx, cancelApply := context.WithTimeout(ctx, 30*time.Second)
	defer cancelApply()
	_, err = probe.Await(applyCtx, func(ctx context.Context) (bool, error) {
		_, _, readErr := c.readEntity(ctx, entityID)
		return readErr == nil, nil
	}, func(applied bool) bool { return applied })
	require.NoError(t, err, "a message published after the restore must be applied")
	// The applied message shows the loop delivers; the open channel shows it is the loop
	// Start began, not one ended and replaced, which is the premise the test rests on.
	require.False(t, isClosed(loopClosed), "the consume loop ended across the relay cut")
}

// TestIntegration_HealthIgnoresErrorCount is ADR-109 Decision 2 (design D3): a processing
// error is counted and reported as ErrorCount, and never lowers Healthy. An undecodable
// message is published; graph-ingest terminates it and counts it.
func TestIntegration_HealthIgnoresErrorCount(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	client, c, owner := startIngestForReadiness(ctx, t)
	defer owner.finish(ctx, t)

	_, err := client.PublishToStreamWithAck(ctx, "entity.c360.test.health.errors.drone.bad", []byte("not a message"))
	require.NoError(t, err)

	health, err := awaitHealthAfterTick(ctx, t, c, func(h component.HealthStatus) bool { return h.ErrorCount >= 1 })
	require.NoError(t, err, "the undecodable message must be counted")
	require.True(t, health.Healthy, "a counted processing error must not lower Healthy: %+v", health)
}
