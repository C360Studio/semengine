package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	serrs "github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

type lifecycleAtomicWatcher struct {
	updates chan jetstream.KeyValueEntry
	stopped atomic.Bool
}

func newLifecycleAtomicWatcher() *lifecycleAtomicWatcher {
	return &lifecycleAtomicWatcher{updates: make(chan jetstream.KeyValueEntry, 16)}
}

func (w *lifecycleAtomicWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w *lifecycleAtomicWatcher) Stop() error {
	w.stopped.Store(true)
	return nil
}

// errStopWatch is what a test's callback returns to end a watch once it has seen what it needs.
var errStopWatch = errors.New("test: stop the watch")

// refuseCall is a watch callback for a watch that must deliver nothing.
func refuseCall(t *testing.T, name string) func(Participant) error {
	return func(p Participant) error {
		t.Errorf("%s delivered %#v", name, p)
		return errStopWatch
	}
}

// requirePoisonEnded checks that a watch ended on the poisoned entry, with the state contract
// error that names it.
func requirePoisonEnded(t *testing.T, err error, name string) {
	t.Helper()
	var contractErr *graph.StateContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("%s returned %v, want the poisoned entry's state contract error", name, err)
	}
}

func TestLifecycleMatchingWatchPoisonDoesNotBlockUnrelatedExactRead(t *testing.T) {
	t.Parallel()
	mgr, _, bucket := newTestManager(t)

	validID := "acme.ops.gcs.lifecycle.mission.valid-b"
	bucket.put(validID, validLifecycleState(validID))

	watcher := newLifecycleAtomicWatcher()
	bucket.watchFactory = func(string) (jetstream.KeyWatcher, error) { return watcher, nil }

	poisonID := "acme.ops.gcs.lifecycle.mission.poison-a"
	watcher.updates <- poisonLifecycleWatchEntry(poisonID, 2)
	err := mgr.Watch(context.Background(), "fixture", refuseCall(t, "matching poison subscription"))
	requirePoisonEnded(t, err, "matching poison subscription")

	participant, err := mgr.Get(context.Background(), "fixture", validID)
	if err != nil {
		t.Fatalf("unrelated exact read after matching watch poison: %v", err)
	}
	if participant.EntityID() != validID {
		t.Fatalf("unrelated exact read entity = %q, want %q", participant.EntityID(), validID)
	}
}

func TestLifecyclePoisonClosesOnlyMatchingSubscription(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		mgr, _, bucket := newTestManager(t)
		other := lifecycle{}.fixtureWorkflow()
		other.Name = "other"
		other.EntityIDPattern = "*.*.other.gcs.mission.*"
		if err := mgr.Register(other); err != nil {
			t.Fatalf("Register other: %v", err)
		}

		fixtureWatcher := newLifecycleAtomicWatcher()
		otherWatcher := newLifecycleAtomicWatcher()
		bucket.watchFactory = func(pattern string) (jetstream.KeyWatcher, error) {
			switch pattern {
			case "*.*.gcs.lifecycle.mission.*":
				return fixtureWatcher, nil
			case "*.*.other.gcs.mission.*":
				return otherWatcher, nil
			default:
				t.Errorf("unexpected watch pattern %q", pattern)
				return nil, errors.New("unexpected watch pattern")
			}
		}

		// The other subscription is open and waiting before the poison arrives.
		var otherSeen []string
		otherDone := make(chan error, 1)
		go func() {
			otherDone <- mgr.Watch(context.Background(), "other", func(p Participant) error {
				otherSeen = append(otherSeen, p.EntityID())
				return errStopWatch
			})
		}()
		synctest.Wait()

		poisonID := "acme.ops.gcs.lifecycle.mission.poison-a"
		fixtureWatcher.updates <- poisonLifecycleWatchEntry(poisonID, 2)
		err := mgr.Watch(context.Background(), "fixture", refuseCall(t, "fixture poison subscription"))
		requirePoisonEnded(t, err, "fixture poison subscription")

		validID := "acme.ops.other.gcs.mission.valid-b"
		otherWatcher.updates <- validLifecycleWatchEntry(t, validID, 3)
		if err := <-otherDone; !errors.Is(err, errStopWatch) {
			t.Fatalf("unrelated subscription returned %v after matching poison, want the callback's stop", err)
		}
		if len(otherSeen) != 1 || otherSeen[0] != validID {
			t.Fatalf("other subscription delivered %v, want [%s]", otherSeen, validID)
		}
	})
}

func TestLifecycleWatchEventsPoisonEmitsNoEvent(t *testing.T) {
	t.Parallel()
	mgr, _, bucket := newTestManager(t)
	watcher := newLifecycleAtomicWatcher()
	bucket.watchFactory = func(string) (jetstream.KeyWatcher, error) { return watcher, nil }
	poisonID := "acme.ops.gcs.lifecycle.mission.poison-a"
	watcher.updates <- poisonLifecycleWatchEntry(poisonID, 7)
	err := mgr.WatchEvents(context.Background(), "fixture", func(event Event) error {
		t.Errorf("poison emitted event %#v", event)
		return errStopWatch
	})
	requirePoisonEnded(t, err, "WatchEvents")
}

func TestLifecycleWatchPoisonWarningNamesSubscriptionAndAuthorityEntryOnce(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	bucket := newFakeBucket()
	mgr := newManagerForTest(logger, &fakeEmitter{bucket: bucket}, bucket)
	if err := mgr.Register(lifecycle{}.fixtureWorkflow()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	watcher := newLifecycleAtomicWatcher()
	bucket.watchFactory = func(string) (jetstream.KeyWatcher, error) { return watcher, nil }

	poisonID := "acme.ops.gcs.lifecycle.mission.poison-a"
	watcher.updates <- poisonLifecycleWatchEntry(poisonID, 41)
	watcher.updates <- poisonLifecycleWatchEntry(poisonID, 42)
	err := mgr.Watch(context.Background(), "fixture", refuseCall(t, "poison subscription"))
	requirePoisonEnded(t, err, "poison subscription")

	const message = "lifecycle workflow watch encountered poisoned graph state; closing subscription"
	logged := logs.String()
	if got := strings.Count(logged, message); got != 1 {
		t.Fatalf("poison warning count = %d, want 1; logs=%s", got, logged)
	}
	for _, fragment := range []string{
		`"level":"WARN"`,
		`"workflow":"fixture"`,
		`"entity":"` + poisonID + `"`,
		`"revision":41`,
		`"code":"` + graph.ErrorCodeGraphStateResetRequired + `"`,
		`"reason":"` + string(graph.GraphStateReasonNoncanonicalPredicate) + `"`,
	} {
		if !strings.Contains(logged, fragment) {
			t.Fatalf("poison warning missing %s; logs=%s", fragment, logged)
		}
	}
}

func TestLifecycleWatchTransportCloseIsLocalAndLaterSubscriptionWorks(t *testing.T) {
	t.Parallel()
	mgr, _, bucket := newTestManager(t)
	first := newLifecycleAtomicWatcher()
	second := newLifecycleAtomicWatcher()
	var calls atomic.Int64
	bucket.watchFactory = func(string) (jetstream.KeyWatcher, error) {
		if calls.Add(1) == 1 {
			return first, nil
		}
		return second, nil
	}

	close(first.updates)
	err := mgr.Watch(context.Background(), "fixture", refuseCall(t, "transport-closed subscription"))
	if err == nil || !serrs.IsTransient(err) {
		t.Fatalf("transport-closed subscription returned %T %v, want transient", err, err)
	}

	validID := "acme.ops.gcs.lifecycle.mission.valid-b"
	second.updates <- validLifecycleWatchEntry(t, validID, 2)
	var seen []string
	err = mgr.Watch(context.Background(), "fixture", func(participant Participant) error {
		seen = append(seen, participant.EntityID())
		return errStopWatch
	})
	if !errors.Is(err, errStopWatch) || len(seen) != 1 || seen[0] != validID {
		t.Fatalf("later subscription returned %v having delivered %v, want the callback's stop after [%s]",
			err, seen, validID)
	}
}

func TestLifecycleWatchCancellationIsQuiet(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	bucket := newFakeBucket()
	mgr := newManagerForTest(logger, &fakeEmitter{bucket: bucket}, bucket)
	if err := mgr.Register(lifecycle{}.fixtureWorkflow()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	watcher := newLifecycleAtomicWatcher()
	bucket.watchFactory = func(string) (jetstream.KeyWatcher, error) { return watcher, nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watcher.updates <- validLifecycleWatchEntry(t, "acme.ops.gcs.lifecycle.mission.valid-b", 1)
	err := mgr.Watch(ctx, "fixture", func(Participant) error {
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled subscription returned %v, want context.Canceled", err)
	}
	if strings.Contains(logs.String(), `"level":"WARN"`) {
		t.Fatalf("cancellation emitted warning: %s", logs.String())
	}
}

func TestLifecycleWatcherStartFailuresAreTransientIndexNotReady(t *testing.T) {
	t.Parallel()
	mgr, _, bucket := newTestManager(t)
	bucket.watchFactory = func(string) (jetstream.KeyWatcher, error) {
		return nil, context.DeadlineExceeded
	}
	err := mgr.Watch(context.Background(), "fixture", refuseCall(t, "unstarted subscription"))
	if err == nil || !serrs.IsTransient(err) {
		t.Fatalf("Watch error = %T %v, want transient", err, err)
	}
}

func validLifecycleState(entityID string) *graph.EntityState {
	return &graph.EntityState{
		ID: entityID,
		Triples: []message.Triple{{
			Subject: entityID, Predicate: "mission.lifecycle.phase", Object: "planning",
		}},
	}
}

func validLifecycleWatchEntry(t *testing.T, entityID string, revision uint64) jetstream.KeyValueEntry {
	t.Helper()
	data, err := graph.MarshalEntityState(validLifecycleState(entityID))
	if err != nil {
		t.Fatalf("MarshalEntityState: %v", err)
	}
	return &fakeKVEntry{key: entityID, value: data, revision: revision, created: time.Now()}
}

func poisonLifecycleWatchEntry(entityID string, revision uint64) jetstream.KeyValueEntry {
	return &fakeKVEntry{
		key: entityID, value: poisonedLifecycleState(entityID), revision: revision, created: time.Now(),
	}
}
