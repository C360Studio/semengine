//go:build integration

package embedding

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/storage"
)

// workerFrames are the stack frames of the Worker's own goroutines: its methods and its keyed
// pool's lanes. None may be left once Run has returned.
var workerFrames = []string{"embedding.(*Worker)", "dispatch.(*KeyedPool["}

// newWorkerStorage returns a Storage over fresh EMBEDDING_INDEX and EMBEDDING_DEDUP buckets on
// the shared server, and the client it reached the server through.
func newWorkerStorage(t *testing.T) (*Storage, *natsclient.Client) {
	t.Helper()
	client := fixtureClient(t)
	index := createBucket(t, client, cacheBucket)
	dedup := createBucket(t, client, "EMBEDDING_DEDUP")
	return NewStorage(client.NewKVStore(index), client.NewKVStore(dedup)), client
}

// workerStacks returns the stacks of any goroutine in a Worker method or a pool lane.
func workerStacks() string {
	var found []string
	for _, frame := range workerFrames {
		if stacks := stacksIn(frame); stacks != "" {
			found = append(found, stacks)
		}
	}
	return strings.Join(found, "\n\n")
}

// receive waits for one value on ch for at most cacheWaitBound.
func receive[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(cacheWaitBound):
		t.Fatalf("timed out waiting for %s", what)
	}
	panic("unreachable")
}

// TestWorkerRunLeavesNoWatchDelivery: after Run's context ends while nats.go's watch delivery
// is blocked on its full updates buffer, Run returns context.Canceled only after that delivery
// has ended, and no goroutine is left in a Worker method or a pool lane. One lane, whose
// terminal callback blocks until the cycle's context ends, holds the pool; the lane's queue
// fills, Run's read waits on it, and the 600-record snapshot (above the 256-entry buffer,
// jetstream/kv.go WatchFiltered, plus the 256-entry lane queue) backs the delivery up behind
// it. As in TestVectorCacheRunLeavesNoWatchDelivery, ten cycles make missing the drain on code
// without it unlikely.
func TestWorkerRunLeavesNoWatchDelivery(t *testing.T) {
	s, _ := newWorkerStorage(t)
	for i := 1; i <= 600; i++ {
		id := fmt.Sprintf("acme.ops.robotics.gcs.drone.w%d", i)
		if err := pendingText(t.Context(), s, id, "", fmt.Sprintf("text %d", i), 1); err != nil {
			t.Fatal(err)
		}
	}
	w := NewWorker(s, fixedEmbedder(), discardLogger()).WithWorkers(1).
		WithOnTerminal(func(ctx context.Context, _ string, _ uint64, _ TerminalOutcome, _ string) { <-ctx.Done() })
	if err := w.Run(nil); err == nil {
		t.Fatal("Run(nil) = nil; want a refusal")
	}
	for cycle := range 10 {
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- w.Run(ctx) }()
		// The stack dump showing the native delivery blocked on its full buffer orders the
		// cancellation after the backlog, not elapsed time.
		if err := natsfixture.AwaitNativeDeliveryBlocked(t.Context(), cacheWaitBound); err != nil {
			cancel()
			t.Fatal(err)
		}
		cancel()
		if err := receive(t, done, "Run to return"); !errors.Is(err, context.Canceled) {
			t.Fatalf("cycle %d: Run returned %v; want context.Canceled", cycle, err)
		}
		if err := natsfixture.AwaitNoStacks(t.Context(), cacheWaitBound, workerStacks); err != nil {
			t.Fatalf("cycle %d: a goroutine is still in the Worker or its pool after Run returned: %v", cycle, err)
		}
		// Run drained the watch until nats.go closed it, which it does after its last delivery returns.
		if err := natsfixture.AwaitNoStacks(t.Context(), cacheWaitBound, natsfixture.NativeDeliveryStacks); err != nil {
			t.Fatalf("cycle %d: nats.go's watch delivery is still running after Run returned: %v", cycle, err)
		}
	}
}

// TestWorkerRunReportsLostWatch: when the bucket is deleted under a live Run, nats.go closes
// the watch's update channel; Run returns an error saying the watch was lost (not the
// context's), no goroutine is left in the Worker or its pool, and the delivery has ended.
// nats.go's unsubscribe goroutine for the watch is released, not joined (notes/3.3.md), so the
// test waits for its stack to go.
func TestWorkerRunReportsLostWatch(t *testing.T) {
	s, client := newWorkerStorage(t)
	processed := make(chan string, 1)
	w := NewWorker(s, fixedEmbedder(), discardLogger()).
		WithOnTerminal(func(_ context.Context, id string, _ uint64, _ TerminalOutcome, _ string) { processed <- id })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	const id = "acme.ops.robotics.gcs.drone.lost"
	if err := pendingText(t.Context(), s, id, "", "before the loss", 1); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, processed, "the record to be processed"); got != id {
		t.Fatalf("processed %s; want %s", got, id)
	}
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
	err = receive(t, done, "Run to return after its bucket was deleted")
	if err == nil || errors.Is(err, context.Canceled) || !errors.Is(err, errWatchLost) {
		t.Fatalf("Run returned %v; want the lost-watch error", err)
	}
	if err := natsfixture.AwaitNoStacks(t.Context(), cacheWaitBound, workerStacks); err != nil {
		t.Fatalf("a goroutine is still in the Worker or its pool after Run returned: %v", err)
	}
	if err := natsfixture.AwaitNoStacks(t.Context(), cacheWaitBound, natsfixture.NativeDeliveryStacks); err != nil {
		t.Fatalf("nats.go's watch delivery is still running after Run returned: %v", err)
	}
	wait, cancelWait := context.WithTimeout(t.Context(), cacheWaitBound)
	defer cancelWait()
	if _, err := probe.Await(wait, func(context.Context) (string, error) {
		return stacksIn(unsubscribeFrame), nil
	}, func(stacks string) bool { return stacks == "" }); err != nil {
		t.Fatalf("nats.go's unsubscribe goroutine is still waiting on the watch's context: %v\n%s",
			err, stacksIn(unsubscribeFrame))
	}
}

// gateStore is a StreamableStore whose Open holds until the test releases it, then serves an
// empty body: an offloaded entity whose body resolved to no text.
type gateStore struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func newGateStore() *gateStore {
	return &gateStore{entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *gateStore) Open(ctx context.Context, _ string) (io.ReadCloser, error) {
	g.once.Do(func() { close(g.entered) })
	select {
	case <-g.release:
		return io.NopCloser(strings.NewReader("")), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (g *gateStore) Put(context.Context, string, []byte) error      { return nil }
func (g *gateStore) Get(context.Context, string) ([]byte, error)    { return nil, nil }
func (g *gateStore) List(context.Context, string) ([]string, error) { return nil, nil }
func (g *gateStore) Delete(context.Context, string) error           { return nil }

// atRevision1 is what N's terminal callback saw: its outcome and the stored record, read
// inside the callback, where the entity's lane holds M's entries back.
type atRevision1 struct {
	outcome TerminalOutcome
	record  *Record
	err     error
}

// runNoText runs a Worker whose revision-1 record N is offloaded with an empty body; the body
// fetch is held while writeM writes revision 2 (M) and the test sees it in the bucket. It
// returns what N's terminal callback saw.
func runNoText(t *testing.T, writeM func(context.Context, *Storage) error, wantM func(*Record) bool) atRevision1 {
	t.Helper()
	const id = "acme.ops.robotics.gcs.drone.notext"
	ctx := t.Context()
	s, _ := newWorkerStorage(t)
	gate := newGateStore()
	seen := make(chan atRevision1, 1)
	w := NewWorker(s, fixedEmbedder(), discardLogger()).WithWorkers(2).
		WithStoreResolver(fakeResolver{stores: map[string]storage.StreamableStore{"objectstore": gate}}).
		WithOnTerminal(func(ctx context.Context, entityID string, rev uint64, outcome TerminalOutcome, _ string) {
			if rev == 1 {
				rec, err := s.GetEmbedding(ctx, entityID)
				seen <- atRevision1{outcome, rec, err}
			}
		})
	stop := runWorker(t, w)

	if err := pendingRef(ctx, s, id, "", "", &StorageRef{StorageInstance: "objectstore", Key: "n"}, 1); err != nil {
		t.Fatal(err)
	}
	receive(t, gate.entered, "N's body fetch")
	if err := writeM(ctx, s); err != nil {
		t.Fatal(err)
	}
	if rec, err := s.GetEmbedding(ctx, id); err != nil || !wantM(rec) {
		t.Fatalf("record before N is released = %+v, %v; want M's", rec, err)
	}
	close(gate.release)
	got := receive(t, seen, "N's terminal callback")
	stopWorker(t, stop)
	return got
}

// TestNoTextDeleteKeepsNewerPending (design D3, changed; Codex's finding 1 on PR #458): N's
// empty body fetch is held until hop 1 has written pending M. After release N removes nothing
// and reports OutcomeDeleted; the record is still pending at M. The pin deleted the key
// unconditionally.
func TestNoTextDeleteKeepsNewerPending(t *testing.T) {
	const id = "acme.ops.robotics.gcs.drone.notext"
	pendingAtM := func(r *Record) bool { return r != nil && r.Status == StatusPending && r.SourceRevision == 2 }
	got := runNoText(t, func(ctx context.Context, s *Storage) error {
		return pendingText(ctx, s, id, "", "text M", 2)
	}, pendingAtM)
	if got.outcome != OutcomeDeleted {
		t.Fatalf("N's outcome = %v; want OutcomeDeleted", got.outcome)
	}
	if got.err != nil || !pendingAtM(got.record) {
		t.Fatalf("record after N = %+v, %v; want still pending at 2", got.record, got.err)
	}
}

// TestNoTextDeleteKeepsNewerGenerated (design D3, changed): as above, but M's generated record
// is written directly through Storage, as another process's Worker would. After release N
// removes nothing and reports OutcomeDeleted; the record is still generated at M with M's
// vector.
func TestNoTextDeleteKeepsNewerGenerated(t *testing.T) {
	const id = "acme.ops.robotics.gcs.drone.notext"
	vectorM := []float32{0, 1, 0}
	generatedAtM := func(r *Record) bool {
		return r != nil && r.Status == StatusGenerated && r.SourceRevision == 2 && slices.Equal(r.Vector, vectorM)
	}
	got := runNoText(t, func(ctx context.Context, s *Storage) error {
		if err := pendingText(ctx, s, id, "", "text M", 2); err != nil {
			return err
		}
		return s.SaveGenerated(ctx, id, vectorM, "other-worker", 3, "hash-m", 2)
	}, generatedAtM)
	if got.outcome != OutcomeDeleted {
		t.Fatalf("N's outcome = %v; want OutcomeDeleted", got.outcome)
	}
	if got.err != nil || !generatedAtM(got.record) {
		t.Fatalf("record after N = %+v, %v; want still generated at 2 with M's vector", got.record, got.err)
	}
}
