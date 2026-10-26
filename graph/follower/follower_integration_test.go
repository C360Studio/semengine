//go:build integration

package follower

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// The follower's tests run against the package's shared NATS server on the real clock. Design D11
// placed the two heartbeat tests in a synctest bubble; a bubble's clock does not advance while a
// goroutine waits on a network read, and nats.go's read loop always does, so they run here instead
// (notes/3.2.md).

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

// waitBound is the real-clock failure bound of every wait on the server. The longest wait is the
// lost feed after a delete: the watch closes within two of nats.go's activity checks, 20 s
// (measured 18.00 s, PR #414 comment 6426443564), so the bound leaves room under load.
const waitBound = 60 * time.Second

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

// entityStates creates ENTITY_STATES as its owner, graph-ingest, would, and returns the write
// handle the test uses in graph-ingest's place.
func entityStates(t *testing.T, client *natsclient.Client) jetstream.KeyValue {
	t.Helper()
	kv, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEntityStates)
	if err != nil {
		t.Fatalf("ensure %s: %v", graph.BucketEntityStates, err)
	}
	return kv
}

// entityID is the n-th test entity's literal six-part ID.
func entityID(n int) string { return fmt.Sprintf("acme.ops.test.system.widget.%03d", n) }

// storedEntity is a value graph.UnmarshalEntityState accepts for id: one statement with its source
// and timestamp.
func storedEntity(id string) []byte {
	return []byte(`{"id":"` + id + `","triples":[{"subject":"` + id +
		`","predicate":"test.state.value","object":1,"source":"test","timestamp":"2026-10-26T12:00:00Z"}]}`)
}

func put(t *testing.T, kv jetstream.KeyValue, key string, value []byte) uint64 {
	t.Helper()
	rev, err := kv.Put(t.Context(), key, value)
	if err != nil {
		t.Fatalf("put %s: %v", key, err)
	}
	return rev
}

func open(t *testing.T, client *natsclient.Client, deliver func(context.Context, Entry), failed func() int64) *Follower {
	t.Helper()
	f, err := Open(t.Context(), Config{
		Client:          client,
		Key:             readiness.KeyGraphIndex,
		MetricsRegistry: metric.NewMetricsRegistry(),
		StartupAttempts: 3,
		StartupInterval: 50 * time.Millisecond,
		Deliver:         deliver,
		Failed:          failed,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return f
}

func noFailures() int64 { return 0 }

// completing is a Deliver that completes each entry at once, as a plug-in whose apply is synchronous.
func completing(f **Follower) func(context.Context, Entry) {
	return func(_ context.Context, e Entry) { (*f).Complete(e.Key, e.Revision) }
}

// start runs f.Run on a goroutine. The returned stop cancels Run's context and returns Run's
// result; a cleanup does the same if the test did not.
func start(t *testing.T, f *Follower) (stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- f.Run(ctx) }()
	var result error
	stopped := false
	stop = func() error {
		if stopped {
			return result
		}
		stopped = true
		cancel()
		select {
		case result = <-done:
		case <-time.After(waitBound):
			result = errors.New("Run did not return after its context ended")
		}
		return result
	}
	t.Cleanup(func() { _ = stop() })
	return stop
}

// statusWatch watches the follower's GRAPH_STATUS key from the outside, as a reader of the key does.
func statusWatch(t *testing.T, client *natsclient.Client) jetstream.KeyWatcher {
	t.Helper()
	reader, err := kvcatalog.OpenCatalogReader(t.Context(), client, graph.BucketGraphStatus)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketGraphStatus, err)
	}
	w, err := reader.Watch(t.Context(), readiness.KeyGraphIndex)
	if err != nil {
		t.Fatalf("watch %s/%s: %v", graph.BucketGraphStatus, readiness.KeyGraphIndex, err)
	}
	t.Cleanup(func() { _ = w.Stop() })
	return w
}

type envelope struct {
	status    graph.IndexStatusResponse
	committed time.Time // the server's commit time of the write
}

// nextEnvelope returns the next envelope written to the key, waiting at most bound.
func nextEnvelope(t *testing.T, w jetstream.KeyWatcher, bound time.Duration) envelope {
	t.Helper()
	timer := time.NewTimer(bound)
	defer timer.Stop()
	for {
		select {
		case entry, ok := <-w.Updates():
			if !ok {
				t.Fatalf("status watch closed")
			}
			if entry == nil {
				continue
			}
			var s graph.IndexStatusResponse
			if err := json.Unmarshal(entry.Value(), &s); err != nil {
				t.Fatalf("decode envelope %q: %v", entry.Value(), err)
			}
			return envelope{status: s, committed: entry.Created()}
		case <-timer.C:
			t.Fatalf("no envelope within %s", bound)
		}
	}
}

// awaitEnvelope reads envelopes until one satisfies done, within waitBound.
func awaitEnvelope(t *testing.T, w jetstream.KeyWatcher, done func(graph.IndexStatusResponse) bool) graph.IndexStatusResponse {
	t.Helper()
	deadline := time.Now().Add(waitBound)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			t.Fatalf("no matching envelope within %s", waitBound)
		}
		if e := nextEnvelope(t, w, left); done(e.status) {
			return e.status
		}
	}
}

func requireCode(t *testing.T, err error, class errs.ErrorClass, code string) {
	t.Helper()
	var ce *errs.ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatalf("error %v is not classified; want class %v code %s", err, class, code)
	}
	if ce.Class != class || ce.Code != code {
		t.Fatalf("error %v has class %v code %q; want class %v code %q", err, ce.Class, ce.Code, class, code)
	}
}

func stuckNames(f *Follower, text string) bool { return names(f.Stuck(), text) }

func names(stuck []string, text string) bool {
	return slices.ContainsFunc(stuck, func(s string) bool { return strings.Contains(s, text) })
}

const feedLost = "ENTITY_STATES feed lost"

// TestFollowerOpenRefusesAbsentBucket: with no ENTITY_STATES, Open gives up after its bounded wait
// with a transient error naming the bucket.
func TestFollowerOpenRefusesAbsentBucket(t *testing.T) {
	client := fixtureClient(t)
	f, err := Open(t.Context(), Config{
		Client:          client,
		Key:             readiness.KeyGraphIndex,
		StartupAttempts: 2,
		StartupInterval: 10 * time.Millisecond,
		Deliver:         func(context.Context, Entry) {},
		Failed:          noFailures,
	})
	if err == nil || f != nil {
		t.Fatalf("Open = %v, %v; want a refusal", f, err)
	}
	if !errs.IsTransient(err) || !strings.Contains(err.Error(), graph.BucketEntityStates) {
		t.Fatalf("Open error %v: want transient, naming %s", err, graph.BucketEntityStates)
	}
}

// TestFollowerRunReturnsAndLeavesNothing: Run refuses a nil context; after its context ends it
// returns nil, no goroutine is left in a Follower method, and the watch's consumer is gone from
// ENTITY_STATES's stream.
func TestFollowerRunReturnsAndLeavesNothing(t *testing.T) {
	client := fixtureClient(t)
	kv := entityStates(t, client)
	put(t, kv, entityID(1), storedEntity(entityID(1)))
	var f *Follower
	f = open(t, client, completing(&f), noFailures)

	if err := f.Run(nil); err == nil {
		t.Fatal("Run(nil) = nil; want a refusal")
	}

	status := statusWatch(t, client)
	stop := start(t, f)
	// The first envelope orders the check after the heartbeat goroutine started and published.
	nextEnvelope(t, status, waitBound)
	if err := stop(); err != nil {
		t.Fatalf("Run returned %v; want nil", err)
	}
	// Run has returned, so every goroutine it started has been joined: none is in a Follower method.
	buf := make([]byte, 1<<20)
	if dump := string(buf[:runtime.Stack(buf, true)]); strings.Contains(dump, "follower.(*Follower)") {
		t.Fatalf("a goroutine is still in a Follower method after Run returned:\n%s", dump)
	}
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.Stream(t.Context(), "KV_"+graph.BucketEntityStates)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), waitBound)
	defer cancel()
	if _, err := probe.Await(ctx, func(ctx context.Context) (int, error) {
		info, err := stream.Info(ctx)
		if err != nil {
			return -1, err
		}
		return info.State.Consumers, nil
	}, func(n int) bool { return n == 0 }); err != nil {
		t.Fatalf("the watch's consumer is still on the stream: %v", err)
	}
}

// TestFollowerPublishesOnEveryHeartbeat: the envelope is written once at once and then every
// DefaultHeartbeat, while Deliver is parked on the first entry for the whole run. The heartbeat
// does not wait for delivery.
func TestFollowerPublishesOnEveryHeartbeat(t *testing.T) {
	client := fixtureClient(t)
	kv := entityStates(t, client)
	put(t, kv, entityID(1), storedEntity(entityID(1)))
	parked := make(chan struct{})
	f := open(t, client, func(ctx context.Context, _ Entry) {
		close(parked)
		<-ctx.Done() // a plug-in whose lane is full: it returns only when Run's context ends
	}, noFailures)
	status := statusWatch(t, client)
	started := time.Now()
	start(t, f)

	first := nextEnvelope(t, status, waitBound)
	select {
	case <-parked:
	case <-time.After(waitBound):
		t.Fatal("Deliver was never called")
	}
	// The first write comes before the first tick: within one heartbeat of Run starting.
	if elapsed := first.committed.Sub(started); elapsed >= readiness.DefaultHeartbeat {
		t.Fatalf("first envelope committed %s after Run started; want before one heartbeat", elapsed)
	}
	// Two more writes while Deliver stays parked, each within two heartbeats of the last: no tick
	// was lost behind the parked Deliver. Commit times are the server's. There is no lower bound: a
	// tick whose write is slow commits late, and the next on-time tick then lands less than one
	// heartbeat after it, so a lower bound would fail on a loaded runner.
	prev := first
	for range 2 {
		next := nextEnvelope(t, status, 3*readiness.DefaultHeartbeat)
		if gap := next.committed.Sub(prev.committed); gap > 2*readiness.DefaultHeartbeat {
			t.Fatalf("envelopes %s apart; want at most %s", gap, 2*readiness.DefaultHeartbeat)
		}
		prev = next
	}
}

// TestFollowerSnapshotEndSetsBootstrapTarget: at the snapshot's end the bootstrap target is the
// highest revision delivered; the latch sets when the floor reaches it although later writes keep
// the live target above, and never before.
func TestFollowerSnapshotEndSetsBootstrapTarget(t *testing.T) {
	client := fixtureClient(t)
	kv := entityStates(t, client)
	var target uint64
	for n := 1; n <= 3; n++ {
		target = put(t, kv, entityID(n), storedEntity(entityID(n)))
	}
	delivered := make(chan Entry, 16)
	f := open(t, client, func(_ context.Context, e Entry) { delivered <- e }, noFailures)
	start(t, f)

	receive := func() Entry {
		t.Helper()
		select {
		case e := <-delivered:
			return e
		case <-time.After(waitBound):
			t.Fatal("no delivery")
			return Entry{}
		}
	}
	var snapshot []Entry
	for range 3 {
		snapshot = append(snapshot, receive())
	}
	// Two writes after the snapshot: the watch sent its end marker right after the third entry, so
	// these arrive after it and raise only the live target.
	put(t, kv, entityID(4), storedEntity(entityID(4)))
	put(t, kv, entityID(5), storedEntity(entityID(5)))
	receive()
	receive()

	for _, e := range snapshot {
		if e.Revision == target {
			continue
		}
		f.Complete(e.Key, e.Revision)
	}
	// Floor below the target: the initial build is not complete.
	if got := f.Indexed(); got >= target {
		t.Fatalf("Indexed = %d before the last snapshot entry completed; want below %d", got, target)
	}
	requireCode(t, f.QueryReady(t.Context()), errs.ErrorTransient, graph.ErrorCodeIndexNotReady)

	f.Complete(entityID(3), target)
	if got := f.Indexed(); got != target {
		t.Fatalf("Indexed = %d; want the snapshot's last revision %d", got, target)
	}
	if err := f.QueryReady(t.Context()); err != nil {
		t.Fatalf("QueryReady after the floor reached the snapshot target %d, live writes pending: %v", target, err)
	}
}

// TestFollowerLatchesResetOnUnreadableSnapshotValue: a stored value that does not decode latches
// reset with its reason; the entry is still delivered; QueryReady is fatal; a later Reset changes
// nothing.
func TestFollowerLatchesResetOnUnreadableSnapshotValue(t *testing.T) {
	client := fixtureClient(t)
	kv := entityStates(t, client)
	put(t, kv, entityID(1), []byte("{not json"))
	delivered := make(chan Entry, 4)
	f := open(t, client, func(_ context.Context, e Entry) { delivered <- e }, noFailures)
	start(t, f)

	// The delivery orders the reads below after the latch: the follower validates before it delivers.
	select {
	case e := <-delivered:
		if e.Key != entityID(1) || e.Deleted || e.Bytes != len("{not json") {
			t.Fatalf("delivered %+v; want the unreadable entry", e)
		}
	case <-time.After(waitBound):
		t.Fatal("the unreadable entry was not delivered")
	}
	reason := string(graph.GraphStateReasonUnreadableEntity)
	err := f.QueryReady(t.Context())
	requireCode(t, err, errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired)
	if !graph.IsStateContractError(err) {
		t.Errorf("QueryReady error %v does not carry the state contract", err)
	}
	if !stuckNames(f, reason) {
		t.Fatalf("Stuck = %q; want it to name %s", f.Stuck(), reason)
	}

	f.Reset(graph.GraphStateReasonNoncanonicalPredicate)
	if !stuckNames(f, reason) || stuckNames(f, string(graph.GraphStateReasonNoncanonicalPredicate)) {
		t.Fatalf("Stuck = %q after a second Reset; want only the first reason %s", f.Stuck(), reason)
	}
}

// TestFollowerDeclaresLostFeed: the two real triggers an operator causes. Deleting ENTITY_STATES
// closes the watch; deleting and creating it again leaves the watch open on a stream it no longer
// reads (PR #414 comment 6426443564). Each is declared: Stuck names it, the envelope reads
// degraded, QueryReady answers index_not_ready, and the heartbeat goes on.
func TestFollowerDeclaresLostFeed(t *testing.T) {
	t.Run("deleted", func(t *testing.T) { lostFeedCase(t, false) })
	t.Run("deleted and recreated", func(t *testing.T) { lostFeedCase(t, true) })
}

func lostFeedCase(t *testing.T, recreate bool) {
	client := fixtureClient(t)
	kv := entityStates(t, client)
	put(t, kv, entityID(1), storedEntity(entityID(1)))
	var f *Follower
	f = open(t, client, completing(&f), noFailures)
	status := statusWatch(t, client)
	start(t, f)
	awaitEnvelope(t, status, func(s graph.IndexStatusResponse) bool { return s.Ready && s.BootstrapComplete })
	if err := f.QueryReady(t.Context()); err != nil {
		t.Fatalf("QueryReady before the delete: %v", err)
	}

	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if err := js.DeleteKeyValue(t.Context(), graph.BucketEntityStates); err != nil {
		t.Fatalf("delete %s: %v", graph.BucketEntityStates, err)
	}
	if recreate {
		entityStates(t, client)
	}

	ctx, cancel := context.WithTimeout(t.Context(), waitBound)
	defer cancel()
	if _, err := probe.Await(ctx, func(context.Context) ([]string, error) { return f.Stuck(), nil },
		func(s []string) bool { return names(s, feedLost) }); err != nil {
		t.Fatalf("Stuck never named the lost feed: %v", err)
	}
	// The envelope written after the mark reads degraded; the heartbeat keeps writing.
	awaitEnvelope(t, status, func(s graph.IndexStatusResponse) bool {
		return s.State == graph.IndexStateDegraded && !s.Ready
	})
	requireCode(t, f.QueryReady(t.Context()), errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
}
