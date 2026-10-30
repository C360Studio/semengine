//go:build integration

// Integration tests for pkg/graphview on a real NATS KV bucket and a real
// WatchAll (ordered consumer, end-of-replay nil marker, tombstone delivery),
// on the package's shared server through the reset (design D6, #195). The
// coherence burden lives in the unit suite on the fake source; these tests
// prove the composed path, the join of nats.go's watch delivery and the loss
// of a watch whose bucket is deleted.
package graphview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

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

// waitBound bounds each wait on the server or on Run in these tests. The longest is the lost watch
// after a delete: nats.go closed the update channel about 20 s after the delete at v1.54.0 (probe
// G6, PR #476 comment 6522730160; not promised), so the bound leaves room under load.
const waitBound = 60 * time.Second

// viewBucket is the bucket these tests watch; the shared server is reset before each test.
const viewBucket = "GRAPHVIEW_IT"

// fixtureClient resets the package's shared server and returns a Client connected to it, closed
// on t.Cleanup before the reset's connection check runs.
func fixtureClient(t *testing.T) *natsclient.Client {
	t.Helper()
	return natsfixture.Open(t, shared.Reset(t), openClient)
}

func openClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(15*time.Second),
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

func createBucket(t *testing.T, client *natsclient.Client) jetstream.KeyValue {
	t.Helper()
	kv, err := client.CreateKeyValueBucket(t.Context(), jetstream.KeyValueConfig{Bucket: viewBucket})
	require.NoError(t, err)
	return kv
}

func putIT(t *testing.T, kv jetstream.KeyValue, key, value string) {
	t.Helper()
	_, err := kv.Put(t.Context(), key, []byte(value))
	require.NoError(t, err)
}

// errRunHung says Run had not returned within waitBound.
var errRunHung = errors.New("run did not return within the wait bound")

// startRun runs view.Run on its own goroutine under a context whose cancel it returns, with a
// function that waits, up to waitBound, for Run to return and returns its error (every call
// returns the same result). A cleanup cancels the context and waits, so no test leaves a Run
// behind when it fails early.
func startRun[T any](t *testing.T, view *View[T]) (wait func() error, stop context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- view.Run(ctx) }()
	wait = sync.OnceValue(func() error {
		select {
		case err := <-done:
			return err
		case <-time.After(waitBound):
			return errRunHung
		}
	})
	t.Cleanup(func() {
		cancel()
		if errors.Is(wait(), errRunHung) {
			t.Error("Run did not return after its context ended")
		}
	})
	return wait, cancel
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

type itRecord struct {
	Name string `json:"name"`
}

func decodeITRecord(key string, value []byte, _ EntryMeta) (*itRecord, bool, error) {
	var rec itRecord
	if err := json.Unmarshal(value, &rec); err != nil {
		return nil, false, fmt.Errorf("decode record %s: %w", key, err)
	}
	if rec.Name == "" {
		return nil, false, nil // not-ready record maps to absent
	}
	return &rec, true, nil
}

// TestIntegration_GraphViewOverRealBucket is the pin's real-bucket test on Run: bootstrap from
// stored entries, caught-up on the real end-of-replay marker, snapshot and subscribe, live
// create, update and delete converging with no revision falling, point reads coherent with the
// stream, and a fresh attach snapshotting the converged state. Ending Run's context then returns
// context.Canceled.
func TestIntegration_GraphViewOverRealBucket(t *testing.T) {
	client := fixtureClient(t)
	kv := createBucket(t, client)

	// Pre-view state: the bootstrap replay must deliver it.
	putIT(t, kv, "alpha", `{"name":"a1"}`)
	putIT(t, kv, "beta", `{"name":"b1"}`)

	view, err := New[*itRecord](kv, decodeITRecord, WithTickInterval(50*time.Millisecond))
	require.NoError(t, err)
	wait, stop := startRun(t, view)
	ctx, cancel := context.WithTimeout(t.Context(), waitBound)
	defer cancel()

	// Readiness is caught-up, not started: the real end-of-replay marker.
	require.NoError(t, view.WaitCaughtUp(ctx))

	snap, sub, err := view.SnapshotAndSubscribe(ctx)
	require.NoError(t, err)
	require.Len(t, snap.Entries, 2)
	require.Equal(t, "a1", snap.Entries["alpha"].Value.Name)

	values := make(map[string]string, len(snap.Entries))
	revs := make(map[string]uint64, len(snap.Entries))
	for k, e := range snap.Entries {
		values[k] = e.Value.Name
		revs[k] = e.Revision
	}

	// Live phase: create, update, delete, all through the real bucket.
	putIT(t, kv, "gamma", `{"name":"g1"}`)
	putIT(t, kv, "alpha", `{"name":"a2"}`)
	require.NoError(t, kv.Delete(ctx, "beta"))

	// Converge by reading batches; the bound is the context, not a sleep.
	want := map[string]string{"alpha": "a2", "gamma": "g1"}
	for !mapsEqualIT(values, want) {
		select {
		case batch, ok := <-sub.Deltas():
			require.True(t, ok, "subscription closed early: %v", sub.Err())
			for _, d := range batch {
				require.Greater(t, d.Revision, revs[d.Key], "stale delivery over the real wire")
				revs[d.Key] = d.Revision
				switch d.Op {
				case DeltaUpsert:
					values[d.Key] = d.Value.Name
				case DeltaDelete, DeltaPoison:
					delete(values, d.Key)
				}
			}
		case <-ctx.Done():
			t.Fatalf("did not converge to %v; have %v", want, values)
		}
	}

	// Point reads are coherent with the stream the subscriber saw.
	val, rev, err := view.Get("alpha")
	require.NoError(t, err)
	require.Equal(t, "a2", val.Name)
	require.GreaterOrEqual(t, rev, revs["alpha"])
	_, _, err = view.Get("beta")
	require.ErrorIs(t, err, ErrKeyNotFound)

	// A fresh attach snapshots the converged state (the snapshot+delta seam over the real wire).
	snap2, sub2, err := view.SnapshotAndSubscribe(ctx)
	require.NoError(t, err)
	got := make(map[string]string, len(snap2.Entries))
	for k, e := range snap2.Entries {
		got[k] = e.Value.Name
	}
	require.Equal(t, want, got)
	sub2.Unsubscribe()
	sub.Unsubscribe()

	stop()
	require.ErrorIs(t, wait(), context.Canceled)
}

func mapsEqualIT(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// TestRunLeavesNoWatchDelivery (spec "Stopped with a backlogged watch"): after Run's context ends
// while nats.go's watch delivery is blocked on its full updates buffer, Run returns
// context.Canceled only after that delivery has ended, and no goroutine is left in a View or
// Subscription method. An OnApply hook that blocks until the cycle's context ends holds Run on the
// first of 600 stored entries, above the 256-entry buffer (nats.go v1.54.0, jetstream/kv.go
// WatchFiltered), so the delivery backs up behind it. Once the context ends, Run's loop picks at
// random between it and a ready update, and a read unblocks the delivery, so only some cycles
// leave the delivery blocked at the stop; ten cycles on fresh views make missing the drain on code
// without it unlikely, as in graph/follower's TestFollowerRunReturnsAndLeavesNothing.
func TestRunLeavesNoWatchDelivery(t *testing.T) {
	client := fixtureClient(t)
	kv := createBucket(t, client)
	for n := 1; n <= 600; n++ {
		putIT(t, kv, fmt.Sprintf("k%03d", n), fmt.Sprintf(`{"name":"n%d"}`, n))
	}
	for cycle := range 10 {
		ctx, cancel := context.WithCancel(t.Context())
		entered := make(chan struct{}, 1)
		view, err := New[*itRecord](kv, decodeITRecord, WithHooks(Hooks{
			OnApply: func(string, uint64) {
				select {
				case entered <- struct{}{}:
				default:
				}
				<-ctx.Done()
			},
		}))
		require.NoError(t, err)
		done := make(chan error, 1)
		go func() { done <- view.Run(ctx) }()
		select {
		case <-entered:
		case <-time.After(waitBound):
			cancel()
			t.Fatal("OnApply was not called")
		}
		// The stack dump showing the native delivery blocked on its full buffer orders the
		// cancellation after the backlog, not elapsed time.
		if err := natsfixture.AwaitNativeDeliveryBlocked(t.Context(), waitBound); err != nil {
			cancel()
			t.Fatal(err)
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cycle %d: Run returned %v; want context.Canceled", cycle, err)
			}
		case <-time.After(waitBound):
			t.Fatalf("cycle %d: Run did not return after its context ended", cycle)
		}
		// Run has returned, so every goroutine it started has been joined.
		for _, frame := range []string{"graphview.(*View[", "graphview.(*Subscription["} {
			if stacks := stacksIn(frame); stacks != "" {
				t.Fatalf("cycle %d: a goroutine is still in %s after Run returned:\n%s", cycle, frame, stacks)
			}
		}
		// Run drained the watch until nats.go closed it, which it does after its last delivery returns.
		if stacks := natsfixture.NativeDeliveryStacks(); stacks != "" {
			t.Fatalf("cycle %d: nats.go's watch delivery is still running after Run returned:\n%s", cycle, stacks)
		}
	}
}

// TestRunReportsLostWatch (spec "The bucket is deleted"): when the bucket is deleted under a live
// Run, nats.go closes the watch's update channel (about 20 s after the delete at v1.54.0, probe
// G6; not promised); Run returns on its own, its context still live, with an error matching
// ErrWatcherLost, not the context's. The attached subscription closes with ErrWatcherLost, reads
// refuse, and nats.go's watch delivery has ended.
func TestRunReportsLostWatch(t *testing.T) {
	client := fixtureClient(t)
	kv := createBucket(t, client)
	putIT(t, kv, "alpha", `{"name":"a1"}`)

	view, err := New[*itRecord](kv, decodeITRecord)
	require.NoError(t, err)
	wait, _ := startRun(t, view)
	ctx, cancel := context.WithTimeout(t.Context(), waitBound)
	defer cancel()
	require.NoError(t, view.WaitCaughtUp(ctx))
	sub, err := view.Subscribe(t.Context())
	require.NoError(t, err)

	js, err := client.JetStream()
	require.NoError(t, err)
	require.NoError(t, js.DeleteKeyValue(t.Context(), viewBucket))

	// Run returns on its own, its context still live, once the update channel closes.
	err = wait()
	if err == nil || errors.Is(err, context.Canceled) || !errors.Is(err, ErrWatcherLost) {
		t.Fatalf("Run returned %v; want an error matching ErrWatcherLost", err)
	}
	// Run returned after the subscription's Deltas closed (failClosed waits on it), so this drain ends.
	for open := true; open; {
		_, open = <-sub.Deltas()
	}
	require.ErrorIs(t, sub.Err(), ErrWatcherLost)
	_, _, err = view.Get("alpha")
	require.ErrorIs(t, err, ErrWatcherLost)
	require.ErrorIs(t, err, ErrNotReady)
	if stacks := natsfixture.NativeDeliveryStacks(); stacks != "" {
		t.Fatalf("nats.go's watch delivery is still running after Run returned:\n%s", stacks)
	}
}
