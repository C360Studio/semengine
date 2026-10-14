//go:build integration

package natsclient

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// This file is the integration lane's half of the helpers that replace the pin's test_client.go
// (admission ledger row natsclient/test_client.go): the broker is internal/harness/natsfixture,
// which always runs JetStream, so the pin's WithJetStream, WithKV, WithMinimalFeatures and
// WithFileStorage have nothing left to choose. Shared tests reset the package-wide broker via
// shared.Reset(t); tests that restart, stop, or reconfigure the broker start their own via
// natsfixture.New.

var shared natsfixture.Shared

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := shared.Stop(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "natsfixture Stop: %v\n", err)
		}
	}()
	return m.Run()
}

// fixtureClientTimeout is the Client's dial timeout (WithTimeout sets nothing else) and the bound on
// its first connection. It is a failure bound, never reached against a broker whose Start has
// returned (design D8 R1b); 15 s is the largest the pin asked for (WithTestTimeout at
// kv_key_contract_integration_test.go:47).
const fixtureClientTimeout = 15 * time.Second

type sharedFixture struct {
	url string
}

func (f sharedFixture) URL() string {
	return f.url
}

// startFixture resets the package's shared broker for t and returns its URL handle.
// Tests that restart, stop, or reconfigure the server call natsfixture.New directly.
func startFixture(t *testing.T) sharedFixture {
	t.Helper()
	return sharedFixture{url: shared.Reset(t)}
}

// fixtureClient is a connected Client on a started broker, the pin's TestClient without the
// container handle.
type fixtureClient struct {
	Client *Client
	URL    string
}

// newFixtureClient replaces the pin's NewTestClient (test_client.go:854): the shared broker is
// reset via shared.Reset(t), and the Client is built as the pin built it (:777-781: no reconnects,
// no health monitor) and connected under fixtureClientTimeout. Its Close is registered on t.Cleanup
// after shared.Reset's post-test check, so it runs first.
func newFixtureClient(t *testing.T) *fixtureClient {
	t.Helper()
	return newFixtureClientAt(t, shared.Reset(t))
}

// newFixtureClientAt connects a test Client to url and registers Close on t.Cleanup.
func newFixtureClientAt(t *testing.T, url string) *fixtureClient {
	t.Helper()
	client, err := NewClient(url,
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
	return &fixtureClient{Client: client, URL: url}
}

// stream creates a stream on the broker, with the bounds the pin's testStreamConfig declared
// (test_client.go:919-930: MaxAge, MaxBytes, DiscardOld; file storage). It replaces the pin's
// WithStreams(TestStreamConfig{...}).
func (tc *fixtureClient) stream(t *testing.T, name string, subjects ...string) {
	t.Helper()
	js, err := tc.Client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	if _, err := natsfixture.CreateStream(t.Context(), js, jetstream.FileStorage, name, subjects...); err != nil {
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
