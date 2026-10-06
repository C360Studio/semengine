//go:build integration

package natsclient

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/nats-io/nats.go"
)

// This file is the integration lane's half of the helpers that replace the pin's test_client.go
// (admission ledger row natsclient/test_client.go): the broker is internal/harness/natsfixture,
// which always runs JetStream, so the pin's WithJetStream, WithKV, WithMinimalFeatures and
// WithFileStorage have nothing left to choose. Each fixture registers its own Stop on t.Cleanup
// under a fresh bounded context, which is what the pin's deferred Terminate calls did.

// fixtureClientTimeout is the Client's dial timeout (WithTimeout sets nothing else) and the bound on
// its first connection. It is a failure bound, never reached against a broker whose Start has
// returned (design D8 R1b); 15 s is the largest the pin asked for (WithTestTimeout at
// kv_key_contract_integration_test.go:47).
const fixtureClientTimeout = 15 * time.Second

// startFixture starts one broker for t; natsfixture.New registers its Stop on t.Cleanup.
func startFixture(t *testing.T, opts ...natsfixture.Option) *natsfixture.Fixture {
	t.Helper()
	f := natsfixture.New(t, opts...)
	if err := f.Start(t.Context()); err != nil {
		t.Fatalf("natsfixture Start: %v", err)
	}
	return f
}

// fixtureClient is a started broker and a connected Client on it, the pin's TestClient without the
// container handle.
type fixtureClient struct {
	Fixture *natsfixture.Fixture
	Client  *Client
	URL     string
}

// newFixtureClient replaces the pin's NewTestClient (test_client.go:854): the Client is built as
// the pin built it (:777-781: no reconnects, no health monitor) and connected under
// fixtureClientTimeout. Its Close is registered on t.Cleanup after the fixture's Stop, so it runs
// first.
func newFixtureClient(t *testing.T, opts ...natsfixture.Option) *fixtureClient {
	t.Helper()
	f := startFixture(t, opts...)
	client, err := NewClient(f.URL(),
		WithTimeout(fixtureClientTimeout),
		WithMaxReconnects(0),
		WithHealthInterval(0),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
		defer cancel()
		if err := client.Close(ctx); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), fixtureClientTimeout)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := client.WaitForConnection(ctx); err != nil {
		t.Fatalf("WaitForConnection: %v", err)
	}
	return &fixtureClient{Fixture: f, Client: client, URL: f.URL()}
}

// stream creates a stream the fixture owns, with the bounds the pin's testStreamConfig declared
// (test_client.go:919-930: MaxAge, MaxBytes, DiscardOld; file storage). It replaces the pin's
// WithStreams(TestStreamConfig{...}).
func (tc *fixtureClient) stream(t *testing.T, name string, subjects ...string) {
	t.Helper()
	if _, err := tc.Fixture.CreateStream(t.Context(), name, subjects...); err != nil {
		t.Fatalf("CreateStream %s: %v", name, err)
	}
}

// dial returns a connection of the test's own on the fixture's URL, replacing the pin's
// GetNativeConnection where a test needs a raw connection; it is closed on t.Cleanup.
func dial(t *testing.T, url string) *nats.Conn {
	t.Helper()
	nc, err := nats.Connect(url, nats.Timeout(fixtureClientTimeout), nats.MaxReconnects(0))
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	t.Cleanup(nc.Close)
	return nc
}

// failureBound bounds a wait on a channel, callback or observed state that a correct run reaches
// first (design D8 R1b). The lane runs under -race; a bound below 10 s is not a failure bound, so the
// pin's 1-5 s bounds on such waits take this one.
const failureBound = 10 * time.Second

// restartBound bounds natsfixture.Restart, which stops and starts a container; stopBound bounds a
// test's own Fixture.Stop, the one natsfixture.New's cleanup also gives it. Both are failure bounds.
const (
	restartBound = 2 * time.Minute
	stopBound    = 60 * time.Second
)

// closeClient closes c under closeBudget, a bounded root (harness-boundaries, "Bounded cleanup
// roots"): the pin's deferred Close calls ran under the test's own context or under
// context.Background(), neither of which ends. A Close error fails the test.
func closeClient(t *testing.T, c *Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), closeBudget)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// flushClient returns once the broker has answered a PING on c's connection. The broker handles a
// connection's protocol in order, so a subscription c made before the flush is registered when it
// returns: the event the pin's 50 ms sleeps after SubscribeForRequests stood in for (design D8 R1b).
func flushClient(t *testing.T, c *Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), failureBound)
	defer cancel()
	if err := c.GetConnection().FlushWithContext(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
}
