//go:build integration

package clustering

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// shared is this package's NATS server. It starts at the first reset; TestMain removes it
// after the tests.
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

// clientTimeout is the request timeout of the fixture client, the value the pin's
// NewTestClient family used.
const clientTimeout = 15 * time.Second

// fixtureClient resets the package's shared server and returns a Client connected to it,
// closed on t.Cleanup before the reset's connection check runs.
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

// communityBucket creates COMMUNITY_INDEX from the framework catalog, as a product does,
// and returns the raw bucket and a storage over it.
func communityBucket(t *testing.T) (jetstream.KeyValue, *NATSCommunityStorage) {
	t.Helper()
	client := fixtureClient(t)
	kv, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketCommunityIndex)
	if err != nil {
		t.Fatalf("ensure %s: %v", graph.BucketCommunityIndex, err)
	}
	storage, err := NewNATSCommunityStorage(client.NewKVStore(kv))
	if err != nil {
		t.Fatalf("NewNATSCommunityStorage: %v", err)
	}
	return kv, storage
}
