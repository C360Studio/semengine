package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/c360studio/semengine/graph"
	serrs "github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// The watch shape (design D5; background-work, "Nothing left behind, nil refused"): Watch and
// WatchEvents call the caller's function on the caller's goroutine and return once the watch has
// ended, so nothing they started is left running. Each test runs in a synctest bubble with the
// watch's context still live when the bubble's function returns: a goroutine left behind is
// then blocked for good, and the bubble fails as deadlocked instead of returning.

// newWatchedManager returns a manager whose ENTITY_STATES watch is w, and the number of watches
// it has opened.
func newWatchedManager(t *testing.T) (*Manager, *lifecycleAtomicWatcher, *atomic.Int64) {
	t.Helper()
	mgr, _, bucket := newTestManager(t)
	watcher := newLifecycleAtomicWatcher()
	var opened atomic.Int64
	bucket.watchFactory = func(string) (jetstream.KeyWatcher, error) {
		opened.Add(1)
		return watcher, nil
	}
	return mgr, watcher, &opened
}

// deletedEntry is the entry a KV watch delivers when its key is deleted.
type deletedEntry struct{ fakeKVEntry }

func (*deletedEntry) Operation() jetstream.KeyValueOp { return jetstream.KeyValueDelete }

func TestWatchReturnsTheCallbackErrorAndLeavesNothingRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mgr, watcher, _ := newWatchedManager(t)
		const id = "acme.ops.gcs.lifecycle.mission.watched"
		watcher.updates <- validLifecycleWatchEntry(t, id, 1)
		errSeen := errors.New("seen")
		var seen []string
		err := mgr.Watch(context.Background(), "fixture", func(p Participant) error {
			seen = append(seen, p.EntityID())
			return errSeen
		})
		if !errors.Is(err, errSeen) {
			t.Fatalf("Watch returned %v, want the callback's error", err)
		}
		if len(seen) != 1 || seen[0] != id {
			t.Fatalf("Watch delivered %v, want [%s]", seen, id)
		}
		if !watcher.stopped.Load() {
			t.Fatal("Watch returned with its subscription still open")
		}
	})
}

func TestWatchEventsReturnsTheCallbackErrorAndLeavesNothingRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mgr, watcher, _ := newWatchedManager(t)
		const id = "acme.ops.gcs.lifecycle.mission.watched"
		watcher.updates <- validLifecycleWatchEntry(t, id, 1)
		watcher.updates <- &deletedEntry{fakeKVEntry{key: id, revision: 2}}
		errSeen := errors.New("seen")
		var seen []Event
		err := mgr.WatchEvents(context.Background(), "fixture", func(e Event) error {
			seen = append(seen, e)
			if e.Op == Deleted {
				return errSeen
			}
			return nil
		})
		if !errors.Is(err, errSeen) {
			t.Fatalf("WatchEvents returned %v, want the callback's error", err)
		}
		if len(seen) != 2 || seen[0].Op != Upserted || seen[0].EntityID != id || seen[0].Participant == nil ||
			seen[1].Op != Deleted || seen[1].EntityID != id || seen[1].Participant != nil {
			t.Fatalf("WatchEvents delivered %+v, want an upsert then a delete of %s", seen, id)
		}
		if !watcher.stopped.Load() {
			t.Fatal("WatchEvents returned with its subscription still open")
		}
	})
}

func TestWatchReturnsWhenItsContextEnds(t *testing.T) {
	for _, watch := range []struct {
		name string
		run  func(*Manager, context.Context, func()) error
	}{
		{"Watch", func(mgr *Manager, ctx context.Context, cancel func()) error {
			return mgr.Watch(ctx, "fixture", func(Participant) error { cancel(); return nil })
		}},
		{"WatchEvents", func(mgr *Manager, ctx context.Context, cancel func()) error {
			return mgr.WatchEvents(ctx, "fixture", func(Event) error { cancel(); return nil })
		}},
	} {
		t.Run(watch.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mgr, watcher, _ := newWatchedManager(t)
				watcher.updates <- validLifecycleWatchEntry(t, "acme.ops.gcs.lifecycle.mission.watched", 1)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if err := watch.run(mgr, ctx, cancel); !errors.Is(err, context.Canceled) {
					t.Fatalf("%s returned %v, want context.Canceled", watch.name, err)
				}
				if !watcher.stopped.Load() {
					t.Fatalf("%s returned with its subscription still open", watch.name)
				}
			})
		})
	}
}

// TestWatchReturnsProjectionFailure: a value that decodes but cannot be projected into the
// workflow's type (its phase is an object, not a string) ends Watch and WatchEvents with an error
// naming the entity and its revision, and nothing after it is delivered. Skipping the entry would
// leave the caller's view silently behind the graph.
func TestWatchReturnsProjectionFailure(t *testing.T) {
	for _, watch := range []struct {
		name string
		run  func(*Manager, func() error) error
	}{
		{"Watch", func(mgr *Manager, call func() error) error {
			return mgr.Watch(context.Background(), "fixture", func(Participant) error { return call() })
		}},
		{"WatchEvents", func(mgr *Manager, call func() error) error {
			return mgr.WatchEvents(context.Background(), "fixture", func(Event) error { return call() })
		}},
	} {
		t.Run(watch.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mgr, watcher, _ := newWatchedManager(t)
				const id = "acme.ops.gcs.lifecycle.mission.unprojectable"
				state := validLifecycleState(id)
				state.Triples[0].Object = map[string]any{"unexpected": true}
				data, err := graph.MarshalEntityState(state)
				if err != nil {
					t.Fatalf("MarshalEntityState: %v", err)
				}
				watcher.updates <- &fakeKVEntry{key: id, value: data, revision: 7}
				watcher.updates <- validLifecycleWatchEntry(t, "acme.ops.gcs.lifecycle.mission.later", 8)
				calls := 0
				err = watch.run(mgr, func() error { calls++; return errStopWatch })
				if err == nil || errors.Is(err, errStopWatch) {
					t.Fatalf("%s returned %v, want the unprojectable entry's error", watch.name, err)
				}
				if calls != 0 {
					t.Errorf("%s delivered %d entries after the unprojectable one", watch.name, calls)
				}
				if msg := err.Error(); !strings.Contains(msg, `"`+id+`"`) || !strings.Contains(msg, "revision 7") {
					t.Errorf("%s error %q does not name entity %s at revision 7", watch.name, msg, id)
				}
				if !watcher.stopped.Load() {
					t.Fatalf("%s returned with its subscription still open", watch.name)
				}
			})
		})
	}
}

func TestWatchRefusesANilContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mgr, _, opened := newWatchedManager(t)
		var nilCtx context.Context // the nil context is the input under test
		if err := mgr.Watch(nilCtx, "fixture", func(Participant) error { return nil }); err == nil {
			t.Error("Watch accepted a nil context")
		}
		if err := mgr.WatchEvents(nilCtx, "fixture", func(Event) error { return nil }); err == nil {
			t.Error("WatchEvents accepted a nil context")
		}
		if n := opened.Load(); n != 0 {
			t.Errorf("%d watches opened on a nil context, want none", n)
		}
	})
}

// TestWatchRefusesAStoredValueGraphIngestRefuses: Watch and WatchEvents apply graph-ingest's
// stored-value rule (graph-entity-writes, "One rule decodes a stored entity"). A value whose
// statement has no Source, and a value under key K holding another entity, end the watch with the
// refusal for K, and the callback is not called for them; a valid entry queued after each would be
// delivered (and stop the watch) if the refused one were skipped.
func TestWatchRefusesAStoredValueGraphIngestRefuses(t *testing.T) {
	const key = "acme.ops.gcs.lifecycle.mission.refused"
	unsourced := validLifecycleState(key)
	unsourced.Triples[0].Source = ""
	misfiled := validLifecycleState("acme.ops.gcs.lifecycle.mission.other")
	values := []struct {
		name   string
		state  *graph.EntityState
		reason graph.StateResetReason
	}{
		{"statement without a source", unsourced, graph.GraphStateReasonMissingStatementMetadata},
		{"value stored under another key", misfiled, graph.GraphStateReasonNoncanonicalEntityID},
	}
	watches := []struct {
		name string
		run  func(*Manager, func(string) error) error
	}{
		{"Watch", func(mgr *Manager, call func(string) error) error {
			return mgr.Watch(context.Background(), "fixture", func(p Participant) error { return call(p.EntityID()) })
		}},
		{"WatchEvents", func(mgr *Manager, call func(string) error) error {
			return mgr.WatchEvents(context.Background(), "fixture", func(e Event) error { return call(e.EntityID) })
		}},
	}
	for _, value := range values {
		for _, watch := range watches {
			t.Run(value.name+"/"+watch.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					mgr, watcher, _ := newWatchedManager(t)
					// json.Marshal, not graph.MarshalEntityState: the encoder does not check
					// statement metadata or the key, and the value is one another writer could store.
					data, err := json.Marshal(value.state)
					if err != nil {
						t.Fatalf("encode the stored value: %v", err)
					}
					watcher.updates <- &fakeKVEntry{key: key, value: data, revision: 5}
					watcher.updates <- validLifecycleWatchEntry(t, "acme.ops.gcs.lifecycle.mission.later", 6)
					var delivered []string
					err = watch.run(mgr, func(id string) error {
						delivered = append(delivered, id)
						return errStopWatch
					})
					if len(delivered) != 0 {
						t.Errorf("%s delivered %v; want nothing after the refused value", watch.name, delivered)
					}
					var classified *serrs.ClassifiedError
					if !errors.As(err, &classified) || classified.Code != graph.ErrorCodeGraphStateResetRequired {
						t.Fatalf("%s returned %v, want code %s", watch.name, err, graph.ErrorCodeGraphStateResetRequired)
					}
					var contractErr *graph.StateContractError
					if !errors.As(err, &contractErr) || contractErr.Reason != value.reason || contractErr.EntityID != key {
						t.Fatalf("%s returned %v, want reason %s for entity %s", watch.name, err, value.reason, key)
					}
					if !watcher.stopped.Load() {
						t.Fatalf("%s returned with its subscription still open", watch.name)
					}
				})
			})
		}
	}
}
