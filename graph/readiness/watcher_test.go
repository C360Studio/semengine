package readiness

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/nats-io/nats.go/jetstream"
)

// Every Watcher test runs in a synctest.Test bubble, with every fake channel made
// inside it. The bubble's clock is the one Read and apply call through time.Now, and it
// moves only when every goroutine in the bubble is blocked, so a test advances it with
// <-time.After(d) and the age it observes is exact. A delivery is a send on the fake
// watch's unbuffered channel followed by synctest.Wait(): the send returns once Run's
// goroutine has taken the entry, and Wait returns once that goroutine is blocked again,
// which is after the entry has been folded into held state. No real NATS runs in a
// bubble.

// fakeEntry is a KV entry. Only the accessors the watcher uses carry real values.
//
// created is the server-side commit stamp. The zero value means "unset", which the
// watcher treats as no commit-time information and therefore never stale-on-arrival —
// so tests that are not about that rule can leave it alone.
type fakeEntry struct {
	key     string
	value   []byte
	rev     uint64
	op      jetstream.KeyValueOp
	created time.Time
}

func (e fakeEntry) Bucket() string                  { return BucketGraphStatus }
func (e fakeEntry) Key() string                     { return e.key }
func (e fakeEntry) Value() []byte                   { return e.value }
func (e fakeEntry) Revision() uint64                { return e.rev }
func (e fakeEntry) Created() time.Time              { return e.created }
func (e fakeEntry) Delta() uint64                   { return 0 }
func (e fakeEntry) Operation() jetstream.KeyValueOp { return e.op }

// fakeWatcher is a jetstream.KeyWatcher over a test-driven channel.
type fakeWatcher struct {
	updates chan jetstream.KeyValueEntry
	stopped atomic.Bool
}

func (w *fakeWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w *fakeWatcher) Stop() error {
	w.stopped.Store(true)
	return nil
}

// fakeBucket embeds jetstream.KeyValue so it satisfies the interface without
// implementing 20 unused methods. ONLY Watch is ever called by the watcher — any
// other call would nil-panic, which is the intended loud failure if that changes.
type fakeBucket struct {
	jetstream.KeyValue
	watch func(ctx context.Context) (jetstream.KeyWatcher, error)
}

func (b *fakeBucket) Watch(ctx context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return b.watch(ctx)
}

// fakeSource hands out buckets and counts each open attempt.
type fakeSource struct {
	open   func(ctx context.Context) (jetstream.KeyValue, error)
	opened atomic.Int32
}

func (s *fakeSource) GetKeyValueBucket(ctx context.Context, _ string) (jetstream.KeyValue, error) {
	s.opened.Add(1)
	return s.open(ctx)
}

// watchSource is a source whose every open binds a watch on updates.
func watchSource(updates chan jetstream.KeyValueEntry) *fakeSource {
	return &fakeSource{open: func(context.Context) (jetstream.KeyValue, error) {
		return &fakeBucket{watch: func(context.Context) (jetstream.KeyWatcher, error) {
			return &fakeWatcher{updates: updates}, nil
		}}, nil
	}}
}

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// newTestWatcher builds a watcher the way a consumer does, with a discarding logger.
func newTestWatcher(src BucketSource, key string, heartbeat time.Duration) *Watcher {
	return NewWatcher(src, key, WithHeartbeat(heartbeat), WithLogger(discardLogger()))
}

func statusJSON(t *testing.T, s graph.IndexStatusResponse) []byte {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	return b
}

// harness wires a running watcher over a single fake bucket whose watch channel the
// test drives directly. It must be built inside a synctest bubble.
type harness struct {
	t       *testing.T
	watcher *Watcher
	updates chan jetstream.KeyValueEntry
	source  *fakeSource
}

func newHarness(t *testing.T, heartbeat time.Duration) *harness {
	t.Helper()
	h := &harness{t: t, updates: make(chan jetstream.KeyValueEntry)} // unbuffered: send == received
	h.source = watchSource(h.updates)
	h.watcher = newTestWatcher(h.source, KeyGraphIndex, heartbeat)
	runWatcher(t, h.watcher)
	return h
}

// runWatcher runs w on a goroutine the test owns, as a component would, and in cleanup
// ends Run's context and joins it. In a bubble, t.Context is cancelled before cleanup
// runs, so Run has already been told to stop when the join starts.
func runWatcher(t *testing.T, w *Watcher) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Errorf("Run = %v, want context.Canceled", err)
		}
	})
}

// deliver sends an entry on the watch and waits until Run's goroutine is blocked again,
// by which point it has applied the entry.
func (h *harness) deliver(entry jetstream.KeyValueEntry) {
	h.t.Helper()
	h.updates <- entry
	synctest.Wait()
}

func readyEntry(t *testing.T) fakeEntry {
	t.Helper()
	return fakeEntry{
		key: KeyGraphIndex,
		value: statusJSON(t, graph.IndexStatusResponse{
			Ready: true, State: graph.IndexStateReady,
			IndexedRevision: 500, TargetRevision: 500,
		}),
		rev: 1,
		op:  jetstream.KeyValuePut,
	}
}

// TestWatcher_CurrentValueIsFreshOnBind covers the spec scenario "a restarted
// consumer is fresh within one delivery": the watch is bound without UpdatesOnly, so
// the key's current value arrives immediately and the consumer is fresh without
// waiting a heartbeat.
func TestWatcher_CurrentValueIsFreshOnBind(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, DefaultHeartbeat)
		synctest.Wait()

		// Before any delivery the answer is UNKNOWN, never a fabricated envelope.
		if r := h.watcher.Read(); r.Known || r.Fresh || r.Status.Ready {
			t.Fatalf("pre-delivery read = %+v, want unknown with a zero envelope", r)
		}

		h.deliver(readyEntry(t))

		r := h.watcher.Read()
		if !r.Known || !r.Fresh {
			t.Fatalf("read after bind delivery = %+v, want known and fresh", r)
		}
		if !r.Status.Ready || r.Status.TargetRevision != 500 {
			t.Errorf("held envelope = %+v, want the delivered ready envelope", r.Status)
		}
		if r.Err != nil {
			t.Errorf("Err = %v, want nil after a good update", r.Err)
		}
	})
}

// TestWatcher_FreshnessExpiresAtThreeHeartbeats pins the D2 window. The bubble's clock
// moves only by the test's own wait, so the age is exact rather than a timing race.
func TestWatcher_FreshnessExpiresAtThreeHeartbeats(t *testing.T) {
	const heartbeat = 5 * time.Second
	tests := []struct {
		name      string
		advance   time.Duration
		wantFresh bool
	}{
		{"immediately after arrival", 0, true},
		{"one heartbeat later", heartbeat, true},
		{"just inside three heartbeats", 3*heartbeat - time.Millisecond, true},
		{"exactly three heartbeats", 3 * heartbeat, true},
		{"one millisecond past three heartbeats", 3*heartbeat + time.Millisecond, false},
		{"long dead feed", time.Hour, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHarness(t, heartbeat)
				h.deliver(readyEntry(t))
				<-time.After(tt.advance)

				r := h.watcher.Read()
				if r.Fresh != tt.wantFresh {
					t.Errorf("Fresh = %v after %v, want %v", r.Fresh, tt.advance, tt.wantFresh)
				}
				// Knownness survives expiry: "the feed went quiet holding Ready" is a
				// different operator problem from "no producer was ever seen", and the
				// defer log distinguishes them. Either way the gate fails closed.
				if !r.Known {
					t.Error("Known = false; an expired status is still last-known")
				}
				if r.Age != tt.advance {
					t.Errorf("Age = %v, want %v", r.Age, tt.advance)
				}
			})
		})
	}
}

// TestWatcher_AlreadyStaleOnArrivalIsUnknown pins the ONE commit-time comparison in
// the package, and it is the sharpest fail-closed rule here: a watch delivers the
// key's CURRENT value the instant it binds, so a consumer starting against a DEAD
// producer receives that producer's final envelope — possibly Ready=true, written an
// hour ago. Pure arrival-time freshness would stamp it brand new and serve it as live
// for a whole window, a fail-open the request/reply never had (it got no responders).
//
// The control row is the point: a value committed just inside the window is accepted,
// so the rule cannot be satisfied by rejecting everything. The window is written out
// as three heartbeats rather than read from the package, so the oracle is the
// requirement, not the code.
func TestWatcher_AlreadyStaleOnArrivalIsUnknown(t *testing.T) {
	const heartbeat = 5 * time.Second
	const window = 3 * heartbeat

	tests := []struct {
		name      string
		committed time.Duration // age at arrival
		wantFresh bool
	}{
		{"committed just now", 0, true},
		{"committed one heartbeat ago", heartbeat, true},
		{"committed just inside the window", window - time.Millisecond, true},
		{"committed one millisecond past the window", window + time.Millisecond, false},
		{"dead producer's hour-old final envelope", time.Hour, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHarness(t, heartbeat)
				entry := readyEntry(t)
				// The bubble's clock does not move during the delivery, so the age at
				// arrival is exactly tt.committed.
				entry.created = time.Now().Add(-tt.committed)
				h.deliver(entry)

				r := h.watcher.Read()
				if r.Fresh != tt.wantFresh {
					t.Errorf("Fresh = %v for a value committed %v before arrival, want %v",
						r.Fresh, tt.committed, tt.wantFresh)
				}
				if !r.Known {
					t.Error("Known = false; the envelope did arrive and is last-known")
				}
				if tt.wantFresh {
					return
				}
				// A stale-on-arrival rejection is otherwise invisible — knownness is true
				// and Age is zero — so it MUST carry its own attribution, or the defer
				// log reads "status_known=true status_age=0s reason=status_unknown" with
				// nothing explaining it.
				if r.Err == nil {
					t.Error("Err = nil; a stale-on-arrival rejection must be attributable")
				}
			})
		})
	}
}

// TestWatcher_MissingBucketIsUnknown covers the standalone deployment (a consumer
// running without its producer) and a producer that has not created the bucket yet:
// unknown, fail closed, with the bind error carried for the defer log — never a
// fabricated envelope, and Run keeps running and retries once its rebind wait ends.
func TestWatcher_MissingBucketIsUnknown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bucketErr := errors.New("nats: bucket not found")
		source := &fakeSource{open: func(context.Context) (jetstream.KeyValue, error) { return nil, bucketErr }}
		w := newTestWatcher(source, KeyGraphIndex, DefaultHeartbeat)
		runWatcher(t, w)

		// Run is now blocked in its rebind wait, after the failed open returned.
		synctest.Wait()
		if got := source.opened.Load(); got != 1 {
			t.Fatalf("bucket opened %d times before the rebind wait, want 1", got)
		}

		r := w.Read()
		if r.Known || r.Fresh {
			t.Fatalf("read with an absent bucket = %+v, want unknown", r)
		}
		if !reflect.DeepEqual(r.Status, graph.IndexStatusResponse{}) {
			t.Errorf("held envelope = %+v, want the zero value (nothing fabricated)", r.Status)
		}
		if !errors.Is(r.Err, bucketErr) {
			t.Errorf("Err = %v, want it to wrap %v for the structured defer log", r.Err, bucketErr)
		}

		// One second later (the rebind wait) Run tries again.
		<-time.After(time.Second)
		synctest.Wait()
		if got := source.opened.Load(); got != 2 {
			t.Errorf("bucket opened %d times one second after the failure, want 2 (a retry)", got)
		}
	})
}

// TestWatcher_MissingKeyIsUnknown covers a live bucket with no envelope yet: the
// watch delivers only the end-of-initial-values marker, which is not a value.
func TestWatcher_MissingKeyIsUnknown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, DefaultHeartbeat)

		h.deliver(nil)
		if r := h.watcher.Read(); r.Known || r.Fresh {
			t.Fatalf("read with no published key = %+v, want unknown", r)
		}

		// And the key appearing later is picked up on the same watch.
		h.deliver(readyEntry(t))
		if r := h.watcher.Read(); !r.Known || !r.Fresh {
			t.Fatalf("read after the key appears = %+v, want known and fresh", r)
		}
		if got := h.source.opened.Load(); got != 1 {
			t.Errorf("bucket opened %d times, want 1 (the same watch)", got)
		}
	})
}

// TestWatcher_TombstoneClearsKnownness: a deleted status key is not evidence of a
// live producer, so it must revoke knownness rather than leave the last good
// envelope standing as fresh.
func TestWatcher_TombstoneClearsKnownness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, DefaultHeartbeat)
		h.deliver(readyEntry(t))

		h.deliver(fakeEntry{key: KeyGraphIndex, rev: 2, op: jetstream.KeyValueDelete})

		r := h.watcher.Read()
		if r.Known || r.Fresh {
			t.Fatalf("read after tombstone = %+v, want unknown", r)
		}
		if r.Err == nil {
			t.Error("Err = nil after a tombstone; the defer log needs the cause")
		}
	})
}

// TestWatcher_UndecodableValueDoesNotRefreshFreshness: garbage on the key must not
// stamp an arrival time, or a broken producer would read as perfectly fresh.
func TestWatcher_UndecodableValueDoesNotRefreshFreshness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const heartbeat = 5 * time.Second
		h := newHarness(t, heartbeat)
		h.deliver(readyEntry(t))

		// Age the good value out of the window, then deliver garbage.
		<-time.After(4 * heartbeat)
		h.deliver(fakeEntry{key: KeyGraphIndex, value: []byte("{not json"), rev: 2, op: jetstream.KeyValuePut})

		r := h.watcher.Read()
		if r.Fresh {
			t.Fatalf("read after an undecodable update = %+v, want stale (arrival not refreshed)", r)
		}
		if r.Age != 4*heartbeat {
			t.Errorf("Age = %v, want %v (the good value's arrival, not the garbage's)", r.Age, 4*heartbeat)
		}
		if r.Err == nil {
			t.Error("Err = nil after a decode failure")
		}
		if !r.Status.Ready {
			t.Error("the last GOOD envelope must survive for diagnostics")
		}
	})
}

// TestWatcher_UndecodableValueRevokesFreshness: an undecodable value is the key's
// CURRENT status, and nothing can say what it reports. The last good envelope must stop
// reading as fresh the moment the garbage lands, not three heartbeats later, or the gate
// keeps admitting reads on a status the producer has already replaced. That envelope is
// kept for diagnostics only, and the next good update restores freshness.
//
// The test above ages the good value out of its window before the garbage arrives, so
// it cannot see this: here the garbage lands while the good value is still fresh.
func TestWatcher_UndecodableValueRevokesFreshness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, DefaultHeartbeat)
		healthy := graph.IndexStatusResponse{
			Ready: true, State: graph.IndexStateReady, BootstrapComplete: true,
			IndexedRevision: 500, TargetRevision: 500,
		}
		good := fakeEntry{key: KeyGraphIndex, value: statusJSON(t, healthy), rev: 1, op: jetstream.KeyValuePut}
		gate := func(r Reading) (bool, DeferReason) {
			return EvaluateReadinessGate(StatusReading{Status: r.Status, Fresh: r.Fresh})
		}

		h.deliver(good)
		// Control: the healthy envelope admits reads, so the refusal below is the garbage's.
		if proceed, reason := gate(h.watcher.Read()); !proceed {
			t.Fatalf("gate on the healthy envelope = (false, %q), want proceed", reason)
		}

		h.deliver(fakeEntry{key: KeyGraphIndex, value: []byte("{not json"), rev: 2, op: jetstream.KeyValuePut})

		r := h.watcher.Read()
		if proceed, reason := gate(r); proceed || reason != DeferStatusUnknown {
			t.Fatalf("gate after an undecodable update = (%v, %q), want (false, %q); Known=%v Fresh=%v Err=%v",
				proceed, reason, DeferStatusUnknown, r.Known, r.Fresh, r.Err)
		}
		if !r.Known {
			t.Error("Known = false; an envelope did arrive, so the defer log must not read as no producer seen")
		}
		// Diagnostics: Err carries the decode failure, and Status and Raw are still the last
		// good envelope, so the defer log can say what the producer last reported.
		var syntaxErr *json.SyntaxError
		if !errors.As(r.Err, &syntaxErr) {
			t.Errorf("Err = %v, want the JSON decode failure", r.Err)
		}
		if !reflect.DeepEqual(r.Status, healthy) || string(r.Raw) != string(good.value) {
			t.Errorf("held envelope = %+v (raw %q), want the last good one %+v (raw %q)",
				r.Status, r.Raw, healthy, good.value)
		}

		// Recovery: the next good update is the current status again.
		good.rev = 3
		h.deliver(good)
		r = h.watcher.Read()
		if proceed, reason := gate(r); !proceed {
			t.Fatalf("gate after a good update following garbage = (false, %q), want proceed; Known=%v Fresh=%v Err=%v",
				reason, r.Known, r.Fresh, r.Err)
		}
		if r.Err != nil {
			t.Errorf("Err = %v after a good update, want nil", r.Err)
		}
	})
}

// TestWatcher_RebindsAfterWatchLoss proves the feed recovers on its own: when the
// watch closes (connection loss, server restart), the watcher re-opens and the next
// delivery makes the consumer fresh again — the recovery path that keeps a transient
// blip from becoming a permanent status_unknown.
func TestWatcher_RebindsAfterWatchLoss(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, DefaultHeartbeat)
		h.deliver(readyEntry(t))

		// Close the current watch channel and hand out a fresh one on the next open. The
		// open is swapped before the close, which Run's receive of it orders.
		second := make(chan jetstream.KeyValueEntry)
		h.source.open = watchSource(second).open
		close(h.updates)

		// Run is now in its rebind wait. The send blocks the test, which lets the bubble's
		// clock run out that wait, so the send returns only once Run has re-bound.
		second <- fakeEntry{
			key: KeyGraphIndex,
			value: statusJSON(t, graph.IndexStatusResponse{
				State: graph.IndexStateBuilding, TargetRevision: 600, Lag: 100, StalenessMs: 1200,
			}),
			rev: 2,
			op:  jetstream.KeyValuePut,
		}
		synctest.Wait()

		r := h.watcher.Read()
		if !r.Known || !r.Fresh {
			t.Fatalf("read after rebind = %+v, want known and fresh", r)
		}
		if r.Status.StalenessMs != 1200 {
			t.Errorf("StalenessMs = %d, want 1200 (the post-rebind envelope)", r.Status.StalenessMs)
		}
		if got := h.source.opened.Load(); got != 2 {
			t.Errorf("bucket opened %d times, want 2 (one rebind)", got)
		}
	})
}

// TestWaitForFirst: WaitForFirst returns nil once an envelope has been decoded and held
// (fresh or not: Read judges that), and the context's error when none arrives. A
// tombstone or an undecodable value is not an envelope, so neither releases it.
func TestWaitForFirst(t *testing.T) {
	// startWait runs WaitForFirst on its own goroutine and returns its result channel.
	startWait := func(ctx context.Context, w *Watcher) <-chan error {
		done := make(chan error, 1)
		go func() { done <- w.WaitForFirst(ctx) }()
		return done
	}
	// returned reports whether the waiter has returned, after every goroutine in the
	// bubble has settled.
	returned := func(done <-chan error) (error, bool) {
		synctest.Wait()
		select {
		case err := <-done:
			return err, true
		default:
			return nil, false
		}
	}

	t.Run("released by a decoded envelope", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := newHarness(t, DefaultHeartbeat)
			done := startWait(t.Context(), h.watcher)
			if err, ok := returned(done); ok {
				t.Fatalf("WaitForFirst = %v before any delivery, want it still waiting", err)
			}

			h.deliver(readyEntry(t))
			err, ok := returned(done)
			if !ok {
				t.Fatal("WaitForFirst still waiting after an envelope was decoded")
			}
			if err != nil {
				t.Errorf("WaitForFirst = %v, want nil", err)
			}
			// A later call answers at once: the release is latched.
			if err, ok := returned(startWait(t.Context(), h.watcher)); !ok || err != nil {
				t.Errorf("second WaitForFirst = (%v, returned %v), want (nil, true)", err, ok)
			}
		})
	})

	t.Run("released by a stale-on-arrival envelope", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := newHarness(t, DefaultHeartbeat)
			done := startWait(t.Context(), h.watcher)

			entry := readyEntry(t)
			entry.created = time.Now().Add(-time.Hour)
			h.deliver(entry)

			err, ok := returned(done)
			if !ok {
				t.Fatal("WaitForFirst still waiting after a stale-on-arrival envelope was decoded")
			}
			if err != nil {
				t.Errorf("WaitForFirst = %v, want nil", err)
			}
			if r := h.watcher.Read(); r.Fresh {
				t.Errorf("read after a stale-on-arrival envelope = %+v, want not fresh", r)
			}
		})
	})

	t.Run("context error when nothing arrives", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := newHarness(t, DefaultHeartbeat)
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()

			if err := h.watcher.WaitForFirst(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("WaitForFirst = %v, want context.DeadlineExceeded", err)
			}
		})
	})

	t.Run("not released by a tombstone or an undecodable value", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := newHarness(t, DefaultHeartbeat)
			done := startWait(t.Context(), h.watcher)

			h.deliver(fakeEntry{key: KeyGraphIndex, rev: 1, op: jetstream.KeyValueDelete})
			if err, ok := returned(done); ok {
				t.Fatalf("WaitForFirst = %v after a tombstone, want it still waiting", err)
			}
			h.deliver(fakeEntry{key: KeyGraphIndex, value: []byte("{not json"), rev: 2, op: jetstream.KeyValuePut})
			if err, ok := returned(done); ok {
				t.Fatalf("WaitForFirst = %v after an undecodable value, want it still waiting", err)
			}

			// Control: the same waiter is released by a decoded envelope, so the waits
			// above are the tombstone's and the garbage's, not a waiter that never returns.
			h.deliver(readyEntry(t))
			if err, ok := returned(done); !ok || err != nil {
				t.Errorf("WaitForFirst = (%v, returned %v) after a decoded envelope, want (nil, true)", err, ok)
			}
		})
	})
}

// TestWatcherRunReturnsContextErrorAndLeavesNothingRunning: Run returns ctx.Err()
// once its context ends, whether it is consuming a watch or waiting to rebind after a
// failed open, and leaves no goroutine and no open KV watch behind (design D5;
// background-work, "Run returns when its context ends" and "Stopped inside a bubble").
// synctest.Test fails if a goroutine started in the bubble is still blocked when the
// function returns, so its return is the proof that nothing is left running.
func TestWatcherRunReturnsContextErrorAndLeavesNothingRunning(t *testing.T) {
	t.Run("cancelled while watching", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			kvWatch := &fakeWatcher{updates: make(chan jetstream.KeyValueEntry)}
			src := &fakeSource{open: func(context.Context) (jetstream.KeyValue, error) {
				return &fakeBucket{watch: func(context.Context) (jetstream.KeyWatcher, error) {
					return kvWatch, nil
				}}, nil
			}}
			w := newTestWatcher(src, KeyGraphIndex, DefaultHeartbeat)

			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- w.Run(ctx) }()

			kvWatch.updates <- readyEntry(t)
			synctest.Wait()
			if r := w.Read(); !r.Known || !r.Fresh {
				t.Fatalf("read while running = %+v, want known and fresh", r)
			}

			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("Run = %v, want context.Canceled", err)
			}
			if !kvWatch.stopped.Load() {
				t.Error("Run returned with the KV watch still open")
			}
		})
	})

	t.Run("deadline while waiting to rebind", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			src := &fakeSource{open: func(context.Context) (jetstream.KeyValue, error) {
				return nil, errors.New("nats: bucket not found")
			}}
			w := newTestWatcher(src, KeyGraphIndex, DefaultHeartbeat)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			if err := w.Run(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("Run = %v, want context.DeadlineExceeded", err)
			}
			if got := src.opened.Load(); got < 2 {
				t.Errorf("bucket opened %d times before the deadline, want a rebind after the failed open", got)
			}
		})
	})
}

// TestWatcher_RunValidatesWiring: misconfiguration fails loudly at Run, before any
// I/O, unlike an absent bucket (a legitimate deployment shape that must stay a runtime
// unknown). A nil context is refused at the call (background-work, "Nil context at
// the entry").
func TestWatcher_RunValidatesWiring(t *testing.T) {
	var nilCtx context.Context

	tests := []struct {
		name   string
		nilCtx bool
		src    func(*fakeSource) BucketSource
		key    string
	}{
		{name: "nil context", nilCtx: true, src: func(s *fakeSource) BucketSource { return s }, key: KeyGraphIndex},
		{name: "nil source", src: func(*fakeSource) BucketSource { return nil }, key: KeyGraphIndex},
		{name: "empty key", src: func(s *fakeSource) BucketSource { return s }, key: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				src := &fakeSource{open: func(context.Context) (jetstream.KeyValue, error) {
					return nil, errors.New("unused")
				}}
				w := newTestWatcher(tt.src(src), tt.key, DefaultHeartbeat)
				ctx := t.Context()
				if tt.nilCtx {
					ctx = nilCtx
				}
				if err := w.Run(ctx); err == nil {
					t.Fatal("Run = nil, want an error")
				}
				if got := src.opened.Load(); got != 0 {
					t.Fatalf("Run opened the bucket %d times before refusing, want 0", got)
				}
			})
		})
	}
}

// TestFreshnessWindow pins the ENVELOPE-liveness window, which is the concept that
// survived the view-age deletion and is easy to confuse with it. It answers "can this
// consumer still vouch for the status reading it holds", not "how far behind is the
// index" — the former gates (StatusReading.Fresh, fail closed), the latter never does.
//
// The satisfiability-floor rules that used to live beside it (MinBoundedStaleness,
// ValidateStalenessBound) are gone with the tolerance they constrained: with no
// view-age bound to declare, there is no bound that can be unsatisfiable.
func TestFreshnessWindow(t *testing.T) {
	if got, want := FreshnessWindow(2*time.Second), 6*time.Second; got != want {
		t.Errorf("FreshnessWindow(2s) = %s, want %s (%dx heartbeat)", got, want, FreshnessMultiplier)
	}

	// A consumer that never declared a heartbeat gets the real default window rather
	// than an accidental zero, which would mark every reading stale on arrival.
	for _, heartbeat := range []time.Duration{0, -time.Second} {
		if got, want := FreshnessWindow(heartbeat), FreshnessMultiplier*DefaultHeartbeat; got != want {
			t.Errorf("FreshnessWindow(%s) = %s, want the default window %s", heartbeat, got, want)
		}
	}
}
