//go:build integration

package natsclient

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nativeWatchConstruction struct {
	watcher jetstream.KeyWatcher
	ctx     context.Context
	err     error
}

// nativeWatchReturnGate holds only the constructor return. It never relays,
// reads or stops native Updates; production remains the sole delivery owner.
type nativeWatchReturnGate struct {
	jetstream.KeyValue
	constructed chan nativeWatchConstruction
	release     chan struct{}
	once        sync.Once
}

func (g *nativeWatchReturnGate) WatchFiltered(ctx context.Context, filters []string, opts ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	watcher, err := g.KeyValue.WatchFiltered(ctx, filters, opts...)
	g.constructed <- nativeWatchConstruction{watcher: watcher, ctx: ctx, err: err}
	if err != nil {
		return nil, err
	}
	<-g.release
	return watcher, nil
}

func (g *nativeWatchReturnGate) open() { g.once.Do(func() { close(g.release) }) }

// spec: nats-kv-keys / Filtered KVStore listings finalize native watcher delivery
func TestIntegration_KVStoreFilteredNativeDeliveryClosure(t *testing.T) {
	testClient := newFixtureClient(t)
	bucket, err := testClient.Client.CreateKeyValueBucket(t.Context(), jetstream.KeyValueConfig{
		Bucket: "KV_NATIVE_DELIVERY_CLOSURE", Storage: jetstream.FileStorage,
	})
	require.NoError(t, err)
	for i := range 5000 {
		_, putErr := bucket.Put(t.Context(), fmt.Sprintf("diag.%04d", i), []byte("v"))
		require.NoError(t, putErr)
	}
	t.Logf("native fixture keys=5000 storage=file server=%s sdk=%s",
		dial(t, testClient.URL).ConnectedServerVersion(), pinnedNATSGoVersion)

	gate := &nativeWatchReturnGate{KeyValue: bucket, constructed: make(chan nativeWatchConstruction, 1), release: make(chan struct{})}
	callDone := make(chan struct{})
	defer func() {
		gate.open()
		<-callDone
	}()
	result := make(chan struct {
		keys []string
		err  error
	}, 1)
	go func() {
		defer close(callDone)
		keys, listErr := testClient.Client.NewKVStore(gate).KeysByFilter(t.Context(), "diag.>")
		result <- struct {
			keys []string
			err  error
		}{keys, listErr}
	}()

	var native nativeWatchConstruction
	native = <-gate.constructed
	require.NoError(t, native.err)
	require.NotNil(t, native.watcher)
	updates := native.watcher.Updates()
	require.Greater(t, cap(updates), 0, "native watcher must have a finite Updates buffer")
	// The buffer must fill while the framework child is live, so the wait runs under the child's
	// own context: its deadline (the KV timeout) is the failure bound, and the test sets no
	// deadline of its own.
	filled, err := probe.Await(native.ctx, func(context.Context) (int, error) { return len(updates), nil },
		func(n int) bool { return n == cap(updates) })
	switch {
	case err != nil:
		t.Errorf("native Updates buffer never reached capacity before child expiry: %v", err)
	case native.ctx.Err() != nil:
		t.Error("native Updates reached capacity only after child expiry")
	default:
		t.Logf("native Updates reached capacity while child live: len=%d cap=%d", filled, cap(updates))
	}
	<-native.ctx.Done()
	assert.ErrorIs(t, native.ctx.Err(), context.DeadlineExceeded)
	gate.open()

	var got struct {
		keys []string
		err  error
	}
	got = <-result
	assert.Nil(t, got.keys)
	assert.True(t, errors.Is(got.err, context.DeadlineExceeded), "public error=%v", got.err)
	// This immediate nonblocking probe is the ownership assertion. A residual
	// entry or still-open channel fails; it does not rescue native delivery.
	select {
	case _, ok := <-updates:
		if ok {
			t.Error("native Updates retained an entry after public return")
		}
	default:
		t.Error("native Updates remained open after public return")
	}
}

// spec: transport-client / The key listings complete the same way
func TestIntegration_FilteredKeyListingsNoMatch(t *testing.T) {
	testClient := newFixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	bucket, err := testClient.Client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: "KV_FILTER_NO_MATCH"})
	require.NoError(t, err)

	// An empty bucket first, then one whose only key the filter does not match:
	// every listing gives nil and no error on the pinned server and SDK. Keys
	// lists the whole bucket, so only the empty bucket is a no-match for it.
	assertNoMatch := func(state string, empty bool) {
		t.Helper()
		for _, listing := range filteredListings {
			if listing.pattern == ">" && !empty {
				continue
			}
			keys, listErr := listing.list(ctx, bucket, listing.pattern)
			require.NoError(t, listErr, "%s on %s", listing.name, state)
			assert.Nil(t, keys, "%s on %s", listing.name, state)
		}
	}
	assertNoMatch("an empty bucket", true)
	_, err = bucket.Put(ctx, "other.one", []byte("v"))
	require.NoError(t, err)
	assertNoMatch("a bucket with no matching key", false)
}
