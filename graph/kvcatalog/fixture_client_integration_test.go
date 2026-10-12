//go:build integration

package kvcatalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/natsclient"
)

// clientTimeout is the request timeout of the fixture client, the value the pin's NewTestClient
// family used.
const clientTimeout = 15 * time.Second

// fixtureClient replaces the pin's natsclient.NewTestClient(t, natsclient.WithKV()): one broker
// for t (JetStream is always on) and a Client connected to it, built as the pin built it (no
// reconnects, no health monitor). natsfixture.Open bounds the connect and closes the Client on
// t.Cleanup before the fixture stops (nats-fixture, "Connected value for a package's tests").
func fixtureClient(t *testing.T) *natsclient.Client {
	t.Helper()
	f := natsfixture.New(t)
	if err := f.Start(t.Context()); err != nil {
		t.Fatalf("natsfixture Start: %v", err)
	}
	return natsfixture.Open(t, f, openClient)
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
