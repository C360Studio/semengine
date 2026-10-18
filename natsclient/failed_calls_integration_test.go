//go:build integration

package natsclient

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// TestIntegration_FailedCallsRefuseNoOtherCall proves that a call's failure is its own: twenty
// publishes no stream captures each return the no-stream error, and a publish a stream captures
// then succeeds with the status still Connected (transport-client, "A failed call refuses no other
// call"). The fixture client's health monitor is off (WithHealthInterval(0)), so nothing but the
// calls themselves can move the status.
func TestIntegration_FailedCallsRefuseNoOtherCall(t *testing.T) {
	tc := newFixtureClient(t)
	tc.stream(t, "FAILEDCALLS", "failedcalls.captured")
	ctx := t.Context()

	for i := 1; i <= 20; i++ {
		err := tc.Client.PublishToStream(ctx, "failedcalls.uncaptured", []byte("x"))
		require.ErrorIs(t, err, jetstream.ErrNoStreamResponse, "publish %d to a subject no stream captures", i)
	}

	require.NoError(t, tc.Client.PublishToStream(ctx, "failedcalls.captured", []byte("x")),
		"publish to a captured subject after twenty failed publishes")
	require.Equal(t, StatusConnected, tc.Client.Status())
}
