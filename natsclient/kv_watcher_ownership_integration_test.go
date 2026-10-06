//go:build integration

package natsclient

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	setupCtx, cancelSetup := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancelSetup()
	bucket, err := testClient.Client.CreateKeyValueBucket(setupCtx, jetstream.KeyValueConfig{
		Bucket: "KV_NATIVE_DELIVERY_CLOSURE", Storage: jetstream.FileStorage,
	})
	require.NoError(t, err)
	for i := range 5000 {
		_, putErr := bucket.Put(setupCtx, fmt.Sprintf("diag.%04d", i), []byte("v"))
		require.NoError(t, putErr)
	}
	t.Logf("native fixture keys=5000 storage=file image=%s server=%s sdk=%s",
		os.Getenv("SEMENGINE_NATS_IMAGE"), dial(t, testClient.URL).ConnectedServerVersion(), pinnedNATSGoVersion)

	gate := &nativeWatchReturnGate{KeyValue: bucket, constructed: make(chan nativeWatchConstruction, 1), release: make(chan struct{})}
	callDone := make(chan struct{})
	defer func() {
		gate.open()
		select {
		case <-callDone:
		case <-time.After(25 * time.Second):
			t.Error("public listing task did not join during test cleanup")
		}
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
	select {
	case native = <-gate.constructed:
	case <-time.After(10 * time.Second):
		t.Fatal("native WatchFiltered construction did not complete")
	}
	require.NoError(t, native.err)
	require.NotNil(t, native.watcher)
	updates := native.watcher.Updates()
	require.Greater(t, cap(updates), 0, "native watcher must have a finite Updates buffer")
	// The buffer must fill while the framework child is live, so the wait runs under the child's
	// own context: its deadline (the KV timeout) is the failure bound. The pin polled on a 10 ms
	// ticker under a 3 s timer of its own.
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
	select {
	case <-native.ctx.Done():
	case <-time.After(failureBound):
		t.Error("framework child did not reach its existing deadline")
	}
	assert.ErrorIs(t, native.ctx.Err(), context.DeadlineExceeded)
	gate.open()

	var got struct {
		keys []string
		err  error
	}
	select {
	case got = <-result:
	case <-time.After(20 * time.Second):
		t.Fatal("public filtered listing did not return after native constructor release")
	}
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
