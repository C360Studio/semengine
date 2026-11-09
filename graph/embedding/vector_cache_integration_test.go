//go:build integration

package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/graphview"
)

// cacheWaitBound bounds each wait on the server or on nats.go's goroutines in these tests.
const cacheWaitBound = time.Minute

// cacheBucket is the bucket these tests watch; the shared server is reset before each test.
const cacheBucket = "EMBEDDING_INDEX"

// unsubscribeFrame is the goroutine nats.go v1.55.0 starts to unsubscribe a subscription when
// the context it was made with ends (js.go:2072-2077), as a stack dump prints it: the module
// path's last element escapes its dot. Measured from this package's integration run
// (notes/3.3.md); the lost-watch test fails if it stops matching a live watch.
const unsubscribeFrame = "github.com/nats-io/nats%2ego.(*js).subscribe.func3("

// putRecords writes n generated records to bucket.
func putRecords(t *testing.T, bucket jetstream.KeyValue, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("acme.ops.robotics.gcs.drone.c%d", i)
		data, err := json.Marshal(&Record{EntityID: id, Vector: []float32{1, float32(i)}, Status: StatusGenerated})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := bucket.Put(t.Context(), id, data); err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
	}
}

// newCache returns a VectorCache over a fresh EMBEDDING_INDEX holding n generated records, and
// the client it reached the shared server through.
func newCache(t *testing.T, n int) (*VectorCache, *natsclient.Client) {
	t.Helper()
	client := fixtureClient(t)
	bucket := createBucket(t, client, cacheBucket)
	putRecords(t, bucket, n)
	return NewVectorCache(NewStorage(client.NewKVStore(bucket), nil)), client
}

// startWarm starts cache.Run on its own goroutine under a context it returns the cancel of, and
// returns Run's result channel once the cache answers.
func startWarm(t *testing.T, cache *VectorCache) (<-chan error, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- cache.Run(ctx) }()
	wait, cancelWait := context.WithTimeout(t.Context(), cacheWaitBound)
	defer cancelWait()
	if _, err := probe.Await(wait, func(context.Context) (error, error) {
		_, _, err := cache.FindSimilar("", []float32{1, 0}, nil, 1)
		return err, nil
	}, func(err error) bool { return err == nil }); err != nil {
		cancel()
		t.Fatalf("the cache never answered: %v; Run: %v", err, <-done)
	}
	return done, cancel
}

// stacksIn returns the goroutine stacks that contain frame.
func stacksIn(frame string) string {
	buf := make([]byte, 1<<20)
	var found []string
	for _, stack := range strings.Split(string(buf[:runtime.Stack(buf, true)]), "\n\n") {
		if strings.Contains(stack, frame) {
			found = append(found, stack)
		}
	}
	return strings.Join(found, "\n\n")
}

// backloggedBucket is a bucket whose Watch opens the real watch, waits until nats.go's delivery
// is blocked on the full updates buffer, tells the test so on blocked, and hands the watcher over
// only once the watch's context has ended. The cache therefore reads the watch only after the
// test's cancel, as a cache whose context ends while the watch is backlogged does (the shape of
// natsclient/storagereport's backloggedWatchStore).
type backloggedBucket struct {
	jetstream.KeyValue
	blocked chan error
}

func (b *backloggedBucket) Watch(ctx context.Context, keys string, opts ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	watcher, err := b.KeyValue.Watch(ctx, keys, opts...)
	if err != nil {
		b.blocked <- err
		return nil, err
	}
	b.blocked <- natsfixture.AwaitNativeDeliveryBlocked(ctx, cacheWaitBound)
	<-ctx.Done()
	return watcher, nil
}

// TestVectorCacheRunLeavesNoWatchDelivery: after Run's context ends while nats.go's watch
// delivery is blocked on its full updates buffer, Run returns context.Canceled only after that
// delivery has ended, and no goroutine is left in a VectorCache method. The bucket holds 600
// records, above the 256-entry buffer (jetstream/kv.go WatchFiltered), and backloggedBucket
// keeps Run from reading the watch until the cycle's context has ended. Once it has, Run's loop
// picks at random between it and a ready update, so only some cycles leave the delivery blocked
// at the stop; ten cycles on one cache, each a new Run, make missing the drain on code without
// it unlikely, as in graph/follower's TestFollowerRunReturnsAndLeavesNothing.
func TestVectorCacheRunLeavesNoWatchDelivery(t *testing.T) {
	client := fixtureClient(t)
	index := createBucket(t, client, cacheBucket)
	putRecords(t, index, 600)
	bucket := &backloggedBucket{KeyValue: index, blocked: make(chan error, 1)}
	cache := NewVectorCache(NewStorage(client.NewKVStore(bucket), nil))
	if err := cache.Run(nil); err == nil {
		t.Fatal("Run(nil) = nil; want a refusal")
	}
	for cycle := range 10 {
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- cache.Run(ctx) }()
		if err := <-bucket.blocked; err != nil {
			cancel()
			t.Fatalf("cycle %d: %v", cycle, err)
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("cycle %d: Run returned %v; want context.Canceled", cycle, err)
		}
		if err := natsfixture.AwaitNoStacks(t.Context(), cacheWaitBound, func() string {
			return stacksIn("embedding.(*VectorCache)")
		}); err != nil {
			t.Fatalf("cycle %d: a goroutine is still in a VectorCache method after Run returned: %v", cycle, err)
		}
		// Run drained the watch until nats.go closed it, which it does after its last delivery returns.
		if err := natsfixture.AwaitNoStacks(t.Context(), cacheWaitBound, natsfixture.NativeDeliveryStacks); err != nil {
			t.Fatalf("cycle %d: nats.go's watch delivery is still running after Run returned: %v", cycle, err)
		}
	}
}

// TestVectorCacheRunReportsLostWatch: when the bucket is deleted under a live Run, nats.go
// closes the watch's update channel; Run returns an error matching graphview.ErrWatcherLost (not
// the context's), FindSimilar returns an error matching graphview.ErrNotReady, and the delivery
// has ended. nats.go's unsubscribe
// goroutine for the watch (js.go:2072-2077 at v1.55.0) waits on a child of the watch's context
// that nats.go also ends when the subscription is unsubscribed (js.go:1920-1922,
// nats.go:5639-5650); Run ends the watch's context and stops the watch, so it is released, not
// joined, and the test waits for its stack to go.
func TestVectorCacheRunReportsLostWatch(t *testing.T) {
	cache, client := newCache(t, 3)
	done, stop := startWarm(t, cache)
	defer stop()
	if stacks := stacksIn(unsubscribeFrame); stacks == "" {
		t.Fatalf("no goroutine is in %s while Run watches; the pattern no longer matches nats.go", unsubscribeFrame)
	}
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if err := js.DeleteKeyValue(t.Context(), cacheBucket); err != nil {
		t.Fatalf("delete %s: %v", cacheBucket, err)
	}
	// Run returns on its own, its context still live, once the update channel closes.
	err = <-done
	if errors.Is(err, context.Canceled) || !errors.Is(err, graphview.ErrWatcherLost) {
		t.Fatalf("Run returned %v; want an error matching ErrWatcherLost", err)
	}
	if _, _, err := cache.FindSimilar("", []float32{1, 0}, nil, 1); !errors.Is(err, graphview.ErrNotReady) {
		t.Fatalf("FindSimilar after the watch was lost = %v; want an error matching ErrNotReady", err)
	}
	if err := natsfixture.AwaitNoStacks(t.Context(), cacheWaitBound, natsfixture.NativeDeliveryStacks); err != nil {
		t.Fatalf("nats.go's watch delivery is still running after Run returned: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), cacheWaitBound)
	defer cancel()
	if _, err := probe.Await(ctx, func(context.Context) (string, error) {
		return stacksIn(unsubscribeFrame), nil
	}, func(stacks string) bool { return stacks == "" }); err != nil {
		t.Fatalf("nats.go's unsubscribe goroutine is still waiting on the watch's context: %v\n%s",
			err, stacksIn(unsubscribeFrame))
	}
}
