//go:build integration

package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/natsclient"
)

// shared is this package's NATS server. It starts at the first reset; TestMain removes it after the tests.
var shared natsfixture.Shared

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	code := m.Run()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := shared.Stop(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

// clientTimeout is the request timeout of the fixture client, the value the pin's NewTestClient
// family used.
const clientTimeout = 15 * time.Second

// fixtureClient replaces the pin's natsclient.NewTestClient(t, natsclient.WithJetStream()): it resets the
// package's shared broker (JetStream is always on) and returns a Client connected to it, built as
// the pin built it (no reconnects, no health monitor). natsfixture.Open bounds the connect and
// closes the Client on t.Cleanup before the reset's connection check runs (nats-fixture,
// "Connected value for a package's tests").
func fixtureClient(t *testing.T) *natsclient.Client {
	t.Helper()
	return natsfixture.Open(t, shared.Reset(t), openClient)
}

func openClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(clientTimeout),
		natsclient.WithMaxReconnects(0),
		natsclient.WithHealthInterval(0),
	)
	if err != nil {
		return nil, nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	if err := client.WaitForConnection(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	return client, client.Close, nil
}
