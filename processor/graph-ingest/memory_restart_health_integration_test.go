//go:build integration

package graphingest

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
)

// restartKeepingURL restarts f and requires its URL unchanged, since graph-ingest's client
// reconnects only to the URL it was given.
func restartKeepingURL(ctx context.Context, t *testing.T, f *natsfixture.Fixture) {
	t.Helper()
	before := f.URL()
	require.NoError(t, f.Restart(ctx))
	require.Equal(t, before, f.URL(), "the broker's URL changed across Restart, so graph-ingest's client cannot reconnect to it")
}

// awaitStreamVisible waits until graph-ingest's own client can look up the ENTITY stream:
// the client has reconnected, so a later tick's lookup reaches the broker.
func awaitStreamVisible(ctx context.Context, t *testing.T, client *natsclient.Client) {
	t.Helper()
	js, err := client.JetStream()
	require.NoError(t, err)
	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, err = probe.Await(waitCtx, func(ctx context.Context) (bool, error) {
		_, lookupErr := js.Stream(ctx, entityStream.name)
		return lookupErr == nil, nil
	}, func(found bool) bool { return found })
	require.NoError(t, err, "graph-ingest's client must reach the ENTITY stream after the restart")
}

// TestIntegration_MemoryStreamLostAtBrokerRestartStaysUnhealthy is graph-ingest-recovery,
// "Memory transport lost at broker restart" (design D1 case a). The memory stream and
// graph-ingest's consumer on it do not survive the restart, and the consume loop does not
// end, so the tick's lookup is what reads the loss: once the producer creates the stream
// again, Health stays unhealthy and names the backlog observation, before and after a
// message is published to it.
func TestIntegration_MemoryStreamLostAtBrokerRestartStaysUnhealthy(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	// Own server: restarts the server mid-test.
	f := natsfixture.New(t)
	require.NoError(t, f.Start(ctx))
	_, err := natsfixture.CreateStream(ctx, f.JetStream(), jetstream.MemoryStorage, entityStream.name, entityStream.subjects...)
	require.NoError(t, err)
	// A client that reconnects keeps graph-ingest running across Restart, as a production
	// client does. Restart's doc says a caller stops what it built on the URL first; this
	// test departs from that on purpose, and restartKeepingURL fails it loudly if Docker
	// remaps the port.
	client := natsfixture.Open(t, f.URL(), openReconnectingClient)
	owner := startIngestOn(ctx, t, client)
	defer owner.finish(ctx, t)

	restartKeepingURL(ctx, t, f)
	_, err = natsfixture.CreateStream(ctx, f.JetStream(), jetstream.MemoryStorage, entityStream.name, entityStream.subjects...)
	require.NoError(t, err)
	awaitStreamVisible(ctx, t, client)

	health, err := awaitHealthAfterTick(ctx, t, owner.component, func(h component.HealthStatus) bool {
		return healthNamesBacklog(h, restartConsumer)
	})
	require.NoError(t, err, "Health must read the lost consumer as unhealthy and name the backlog observation: %+v", health)

	const entityID = "c360.test.health.restart.drone.001"
	publishTriples(ctx, t, client, entityID, message.Triple{
		Subject: entityID, Predicate: "test.content.body", Object: "after-restart", Timestamp: time.Now(), Confidence: 1.0,
	})
	health, err = awaitHealthAfterTick(ctx, t, owner.component, func(h component.HealthStatus) bool {
		return healthNamesBacklog(h, restartConsumer)
	})
	require.NoError(t, err, "Health must stay unhealthy after a message is published to the recreated stream: %+v", health)
}
