//go:build integration

package natsclient

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
)

// siblingTestName is the parent's client name; the server lists every sibling under it.
const siblingTestName = "natsclient-sibling-test"

// connectNamedParent connects a Client named siblingTestName to url, which carries the shared
// server's password, and registers its Close, under a bounded context, on t's cleanup.
func connectNamedParent(t *testing.T, url string) *Client {
	t.Helper()
	parent, err := NewClient(url,
		WithName(siblingTestName),
		WithTimeout(fixtureClientTimeout),
		WithMaxReconnects(0),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
		defer cancel()
		_ = parent.Close(ctx)
	})
	require.NoError(t, parent.Connect(t.Context()))
	return parent
}

// newConnectedSibling makes and connects a sibling of parent and registers its Close, under a
// bounded context, on t's cleanup.
func newConnectedSibling(t *testing.T, parent *Client) *Client {
	t.Helper()
	sibling, err := parent.NewSibling()
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
		defer cancel()
		_ = sibling.Close(ctx)
	})
	require.Equal(t, StatusDisconnected, sibling.Status(), "a sibling starts unconnected")
	require.Nil(t, sibling.GetConnection(), "a sibling starts unconnected")
	require.NoError(t, sibling.Connect(t.Context()))
	return sibling
}

// connectionNames asks the server, over nc, for the account's connection listing and returns
// each connection's name by its client ID.
func connectionNames(t *testing.T, nc *nats.Conn) map[uint64]string {
	t.Helper()
	msg, err := nc.RequestWithContext(t.Context(), "$SYS.REQ.ACCOUNT.PING.CONNZ", nil)
	require.NoError(t, err)
	var reply struct {
		Data struct {
			Conns []struct {
				CID  uint64 `json:"cid"`
				Name string `json:"name"`
			} `json:"connections"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(msg.Data, &reply))
	names := make(map[uint64]string, len(reply.Data.Conns))
	for _, c := range reply.Data.Conns {
		names[c.CID] = c.Name
	}
	return names
}

// A sibling of a Client on the password-protected shared server connects with the parent's
// settings, and the server lists both connections under the parent's name.
func TestSiblingCarriesTheClientSettings(t *testing.T) {
	parent := connectNamedParent(t, shared.Reset(t))
	sibling := newConnectedSibling(t, parent)

	parentID, err := parent.GetConnection().GetClientID()
	require.NoError(t, err)
	siblingID, err := sibling.GetConnection().GetClientID()
	require.NoError(t, err)
	require.NotEqual(t, parentID, siblingID, "the sibling has a connection of its own")

	// Connect returned after the server answered the sibling's handshake, so the listing has it.
	names := connectionNames(t, parent.GetConnection())
	require.Equal(t, siblingTestName, names[parentID], "the server lists the parent under its name")
	require.Equal(t, siblingTestName, names[siblingID], "the server lists the sibling under the parent's name")
}

// The parent's Close returns while the sibling stays connected and keeps working until its own
// Close, after which no Client goroutine is left beyond those running before the sibling was made.
func TestParentCloseLeavesSiblingOpen(t *testing.T) {
	parent := connectNamedParent(t, shared.Reset(t))
	baseline := len(clientWorkStacks(clientWorkFrames...))
	sibling := newConnectedSibling(t, parent)

	require.NoError(t, parent.Close(t.Context()))

	// The parent's Close has returned nil, so its cleanup has finished.
	require.Equal(t, StatusConnected, sibling.Status(), "the parent's Close leaves the sibling connected")
	nc := sibling.GetConnection()
	require.NotNil(t, nc)
	_, err := nc.RTT()
	require.NoError(t, err, "the sibling still reaches the server")
	siblingID, err := nc.GetClientID()
	require.NoError(t, err)
	require.Equal(t, siblingTestName, connectionNames(t, nc)[siblingID], "the server still lists the sibling")

	require.NoError(t, sibling.Close(t.Context()))
	require.Equal(t, StatusDisconnected, sibling.Status())
	// Both Closes have returned nil, so each joined its work; a goroutine that signalled its
	// joiner can still be in the dump until it exits, which AwaitNoStacks waits out.
	if err := natsfixture.AwaitNoStacks(t.Context(), lifecycleBound,
		noMoreClientWork(baseline, clientWorkFrames...)); err != nil {
		t.Fatalf("a Client goroutine is left after both Closes: %v", err)
	}
}
