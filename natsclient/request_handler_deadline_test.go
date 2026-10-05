package natsclient

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// TestRequestHandlerDeadlineIsTheConfiguredTimeout proves, at the seam that derives
// it, that a SubscribeForRequests handler's context ends exactly the configured
// handler timeout after the callback starts. The callback runs inside a synctest
// bubble, whose clock does not move while the callback runs, so the deadline is
// compared for equality rather than against a tolerance a slow host could exceed.
// The configured value differs from DefaultRequestHandlerTimeout, so a callback that
// ignored the configuration would fail the comparison.
func TestRequestHandlerDeadlineIsTheConfiguredTimeout(t *testing.T) {
	const configured = 7 * time.Second
	require.NotEqual(t, DefaultRequestHandlerTimeout, configured)
	t.Setenv(requestHandlerTimeoutEnv, configured.String())
	client, err := NewClient("nats://unused")
	require.NoError(t, err)

	synctest.Test(t, func(t *testing.T) {
		var (
			deadline    time.Time
			hasDeadline bool
		)
		callback := client.requestCallback(t.Context(), nil, "test.deadline",
			func(hctx context.Context, _ []byte) ([]byte, error) {
				deadline, hasDeadline = hctx.Deadline()
				return nil, nil
			})

		start := time.Now()
		// No reply subject: the callback runs the handler and sends nothing.
		callback(&nats.Msg{Subject: "test.deadline"})

		require.True(t, hasDeadline, "the handler context carries a deadline")
		require.True(t, deadline.Equal(start.Add(configured)),
			"handler deadline %v, want start %v plus the configured %v", deadline, start, configured)
	})
}
