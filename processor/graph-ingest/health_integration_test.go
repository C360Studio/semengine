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

// TestIntegration_HealthClearsWhenObservationSucceeds is ADR-109's "healthy again when it
// clears", worded on the observation (design D3). graph-ingest's durable consumer is deleted:
// unhealthy. A consumer with the same stream, name and configuration is then created through
// the JetStream API: healthy again. Consumption is NOT restored by this (graph-ingest's
// consume loop is not rebound, #216); the test proves only that Health follows the latest
// tick's observation.
func TestIntegration_HealthClearsWhenObservationSucceeds_ConsumptionNotRestored(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	_, c, owner := startIngestForReadiness(ctx, t)
	defer owner.finish(ctx, t)
	js, bound, config := boundIngestConsumer(ctx, t, c)

	require.NoError(t, js.DeleteConsumer(ctx, bound.stream, bound.name))
	_, err := awaitHealthAfterTick(ctx, t, c, func(h component.HealthStatus) bool {
		return healthNamesBacklog(h, bound.name)
	})
	require.NoError(t, err, "Health must read the lost consumer as unhealthy")

	_, err = js.CreateConsumer(ctx, bound.stream, config)
	require.NoError(t, err)
	health, err := awaitHealthAfterTick(ctx, t, c, func(h component.HealthStatus) bool { return h.Healthy })
	require.NoError(t, err, "Health must be healthy again once the observation succeeds")
	require.Empty(t, health.LastError)
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
