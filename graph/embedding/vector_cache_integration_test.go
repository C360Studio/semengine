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

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/natsclient"
)

// cacheWaitBound bounds each wait on the server or on nats.go's goroutines in these tests.
const cacheWaitBound = time.Minute

// cacheBucket is the bucket these tests watch; the shared server is reset before each test.
const cacheBucket = "EMBEDDING_INDEX"

// unsubscribeFrame is the goroutine nats.go v1.54.0 starts to unsubscribe a subscription when
// the context it was made with ends (js.go:2063-2068), as a stack dump prints it: the module
// path's last element escapes its dot. Measured from this package's integration run
// (notes/3.3.md); the lost-watch test fails if it stops matching a live watch.
const unsubscribeFrame = "github.com/nats-io/nats%2ego.(*js).subscribe.func3("

// newCache returns a VectorCache over a fresh EMBEDDING_INDEX holding n generated records, the
// client it reached the shared server through, and a function that writes the n records again.
func newCache(t *testing.T, n int) (*VectorCache, *natsclient.Client, func()) {
	t.Helper()
	client := fixtureClient(t)
	bucket := createBucket(t, client, cacheBucket)
	rewrite := func() {
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
	rewrite()
	return NewVectorCache(NewStorage(client.NewKVStore(bucket), nil)), client, rewrite
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
	if _, err := probe.Await(wait, func(context.Context) (bool, error) {
		_, ok := cache.FindSimilar("", []float32{1, 0}, nil, 1)
		return ok, nil
	}, func(ok bool) bool { return ok }); err != nil {
		cancel()
		t.Fatalf("the cache never became authoritative: %v; Run: %v", err, <-done)
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

// TestVectorCacheRunLeavesNoWatchDelivery: after Run's context ends while nats.go's watch
// delivery is blocked on its full updates buffer, Run returns context.Canceled only after that
// delivery has ended, and no goroutine is left in a VectorCache method. A FindSimilar whose keep
// blocks until the cycle's context ends holds the cache's read lock, so Run's loop waits to
// apply the first of 600 rewritten records (above the 256-entry buffer, jetstream/kv.go
// WatchFiltered) and the delivery backs up behind it. Once the context ends, Run's loop picks at
// random between it and a ready update, so only some cycles leave the delivery blocked at Stop;
// ten cycles make missing the drain on code without it unlikely, as in graph/follower's
// TestFollowerRunReturnsAndLeavesNothing.
func TestVectorCacheRunLeavesNoWatchDelivery(t *testing.T) {
	cache, _, rewrite := newCache(t, 600)
	if err := cache.Run(nil); err == nil {
		t.Fatal("Run(nil) = nil; want a refusal")
	}
	for cycle := range 10 {
		ctx, cancel := context.WithCancel(t.Context())
		done, stopRun := startWarm(t, cache)
		entered := make(chan struct{}, 1)
		read := make(chan struct{})
		go func() {
			defer close(read)
			cache.FindSimilar("", []float32{1, 0}, func(string) bool {
				select {
				case entered <- struct{}{}:
				default:
				}
				<-ctx.Done()
				return true
			}, 1)
		}()
		select {
		case <-entered:
		case <-time.After(cacheWaitBound):
			t.Fatal("keep was not called")
		}
		rewrite()
		// The stack dump showing the native delivery blocked on its full buffer orders the
		// cancellation after the backlog, not elapsed time.
		if err := natsfixture.AwaitNativeDeliveryBlocked(t.Context(), cacheWaitBound); err != nil {
			t.Fatal(err)
		}
		cancel() // releases the read
		stopRun()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("cycle %d: Run returned %v; want context.Canceled", cycle, err)
		}
		<-read // joins the test's own FindSimilar, so only Run could be left in a VectorCache method
		if stacks := stacksIn("embedding.(*VectorCache)"); stacks != "" {
			t.Fatalf("cycle %d: a goroutine is still in a VectorCache method after Run returned:\n%s", cycle, stacks)
		}
		// Run drained the watch until nats.go closed it, which it does after its last delivery returns.
		if stacks := natsfixture.NativeDeliveryStacks(); stacks != "" {
			t.Fatalf("cycle %d: nats.go's watch delivery is still running after Run returned:\n%s", cycle, stacks)
		}
	}
}

// TestVectorCacheRunReportsLostWatch: when the bucket is deleted under a live Run, nats.go
// closes the watch's update channel; Run returns an error saying the watch was lost (not the
// context's), the cache stops answering, and the delivery has ended. nats.go's unsubscribe
// goroutine for the watch (js.go:2063-2068 at v1.54.0) waits on a child of the watch's context
// that nats.go also ends when the subscription is unsubscribed (js.go:1911-1913,
// nats.go:5553-5564); Run ends the watch's context and stops the watch, so it is released, not
// joined, and the test waits for its stack to go.
func TestVectorCacheRunReportsLostWatch(t *testing.T) {
	cache, client, _ := newCache(t, 3)
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
	select {
	case err = <-done:
	case <-time.After(cacheWaitBound):
		t.Fatal("Run did not return after its bucket was deleted")
	}
	if err == nil || errors.Is(err, context.Canceled) || !errors.Is(err, errWatchLost) {
		t.Fatalf("Run returned %v; want the lost-watch error", err)
	}
	if _, ok := cache.FindSimilar("", []float32{1, 0}, nil, 1); ok {
		t.Fatal("the cache answered after its watch was lost")
	}
	if stacks := natsfixture.NativeDeliveryStacks(); stacks != "" {
		t.Fatalf("nats.go's watch delivery is still running after Run returned:\n%s", stacks)
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
