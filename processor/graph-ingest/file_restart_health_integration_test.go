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
)

// TestIntegration_FileStreamRestartStaysHealthy is graph-ingest-recovery, "File stream
// across a broker restart" (design D1 case c). The file stream and graph-ingest's durable
// consumer survive the restart and the consume loop resumes, so a tick after the restart
// reads healthy and a message published after it is applied.
func TestIntegration_FileStreamRestartStaysHealthy(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	// Own server: restarts the server mid-test.
	f := natsfixture.New(t)
	require.NoError(t, f.Start(ctx))
	_, err := natsfixture.CreateStream(ctx, f.JetStream(), jetstream.FileStorage, entityStream.name, entityStream.subjects...)
	require.NoError(t, err)
	// A client that reconnects keeps graph-ingest running across Restart, as a production
	// client does. Restart's doc says a caller stops what it built on the URL first; this
	// test departs from that on purpose, and restartKeepingURL fails it loudly if Docker
	// remaps the port.
	client := natsfixture.Open(t, f.URL(), openReconnectingClient)
	owner := startIngestOn(ctx, t, client)
	defer owner.finish(ctx, t)

	restartKeepingURL(ctx, t, f)
	awaitStreamVisible(ctx, t, client)

	health, err := awaitHealthAfterTick(ctx, t, owner.component, func(h component.HealthStatus) bool { return h.Healthy })
	require.NoError(t, err, "Health must read healthy after the restart: %+v", health)

	const entityID = "c360.test.health.restart.drone.002"
	publishTriples(ctx, t, client, entityID, message.Triple{
		Subject: entityID, Predicate: "test.content.body", Object: "after-restart", Timestamp: time.Now(), Confidence: 1.0,
	})
	// When the resumed loop pulls again is nats.go's pacing, not promised (ADR-112), so
	// the bound is generous.
	applyCtx, cancelApply := context.WithTimeout(ctx, 30*time.Second)
	defer cancelApply()
	_, err = probe.Await(applyCtx, func(ctx context.Context) (bool, error) {
		_, _, readErr := owner.component.readEntity(ctx, entityID)
		return readErr == nil, nil
	}, func(applied bool) bool { return applied })
	require.NoError(t, err, "a message published after the restart must be applied")
}
