package natsclient

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// optionStep is T9's write timeout and ping interval, the values P24 measured,
// so that no test waits on nats.go's one-minute write timeout or natsclient's
// 30 s ping.
const optionStep = 300 * time.Millisecond

// T9, design "Tests that replace the probes" and D4. WithWriteTimeout ends a
// publish the server is not reading. At nats.go v1.55.0 the write it ends
// partway closes the socket and nats.go reconnects, so the echo arrives after a
// reconnect (P24r, PR #492 comment 6545759092). The ping interval is set too, as
// P24 measured; at v1.54.0 it was what found the connection stale (P22, P24).
func TestWriteTimeoutAndPingIntervalOptions(t *testing.T) {
	t.Run("a held write fails, then the connection carries an echo", func(t *testing.T) {
		client, proxy := newHeldClient(t, WithWriteTimeout(optionStep), WithPingInterval(optionStep))
		echo := startEchoResponder(t, "nats://"+proxy.server)
		conn := client.GetConnection()
		published := holdPublish(t, client, proxy)

		// The test's own wait, well under nats.go's one-minute default.
		err := within(t, published, lifecycleBound, "the held publish")
		require.ErrorIs(t, err, os.ErrDeadlineExceeded)

		proxy.resume()
		wait, cancel := context.WithTimeout(t.Context(), lifecycleBound)
		defer cancel()
		reply, err := probe.Await(wait, func(ctx context.Context) (string, error) {
			attempt, cancelAttempt := context.WithTimeout(ctx, optionStep)
			defer cancelAttempt()
			msg, err := conn.RequestWithContext(attempt, echo, []byte("t9"))
			if err != nil {
				return "", err
			}
			return string(msg.Data), nil
		}, func(reply string) bool { return reply == "t9" })
		require.NoError(t, err, "the connection carried no echo after the proxy read again: %q", reply)
		require.Positive(t, conn.Stats().Reconnects, "the echo succeeded without a reconnect")
	})

	t.Run("each option refuses a value that is not positive", func(t *testing.T) {
		for _, d := range []time.Duration{0, -time.Second} {
			for _, tc := range []struct {
				option ClientOption
				says   string
			}{
				{WithWriteTimeout(d), "write timeout"},
				{WithPingInterval(d), "ping interval"},
			} {
				client, err := NewClient("nats://unused", tc.option)
				require.Nil(t, client)
				require.True(t, errs.IsInvalid(err), "%s %v: not refused as invalid: %v", tc.says, d, err)
				require.ErrorContains(t, err, tc.says)
			}
		}
	})
}

// startEchoResponder connects to url and answers each request on the subject
// it returns with the request's own data.
func startEchoResponder(t *testing.T, url string) string {
	t.Helper()
	const subject = "natsclient.t9.echo"
	responder, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(responder.Close)
	_, err = responder.Subscribe(subject, func(m *nats.Msg) { _ = m.Respond(m.Data) })
	require.NoError(t, err)
	require.NoError(t, responder.Flush())
	return subject
}
