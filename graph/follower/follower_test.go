package follower

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/internal/revlag"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// TestOpenRefusesMissingInputs: Open refuses each input the entity-state-follower spec names
// ("Open waits for ENTITY_STATES within a bound") before it makes any NATS call, so a zero client
// value is enough to reach every check after the client's.
func TestOpenRefusesMissingInputs(t *testing.T) {
	valid := func() Config {
		return Config{
			Client:  &natsclient.Client{},
			Key:     "graph-index",
			Deliver: func(context.Context, Entry) {},
			Failed:  func() (int64, map[string]uint64) { return 0, nil },
		}
	}
	cases := []struct {
		name  string
		ctx   context.Context
		edit  func(*Config)
		names string
	}{
		{"nil context", nil, func(*Config) {}, "nil context"},
		{"nil client", t.Context(), func(c *Config) { c.Client = nil }, "nil client"},
		{"empty key", t.Context(), func(c *Config) { c.Key = "" }, "empty status key"},
		{"nil Deliver", t.Context(), func(c *Config) { c.Deliver = nil }, "nil Deliver"},
		{"nil Failed", t.Context(), func(c *Config) { c.Failed = nil }, "nil Failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid()
			tc.edit(&cfg)
			f, err := Open(tc.ctx, cfg)
			if err == nil || f != nil {
				t.Fatalf("Open = %v, %v; want a refusal", f, err)
			}
			if !errs.IsInvalid(err) {
				t.Errorf("Open error %v is not class invalid", err)
			}
			if !strings.Contains(err.Error(), tc.names) {
				t.Errorf("Open error %q does not name %q", err, tc.names)
			}
		})
	}
}

// openWatcher is a jetstream.KeyWatcher whose update channel never closes: the state nats.go
// v1.55.0 leaves a watch in when its context ends between Subscribe and the install of the
// handler that closes the channel (#454).
type openWatcher struct{ updates chan jetstream.KeyValueEntry }

func (w openWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w openWatcher) Stop() error                             { return nil }

// TestStopWatchGivesUpOnADeliveryThatNeverCloses: after Run's context ends, stopWatch waits for
// the watch's delivery to close for at most the named budget, 5 s by the entity-state-follower
// spec ("The watch's delivery never closes"), then reports that it did not close. The bubble's
// clock advances only when every goroutine in it is blocked, so time.Since reads exactly how long
// stopWatch waited; with no bound the bubble deadlocks and the test fails.
func TestStopWatchGivesUpOnADeliveryThatNeverCloses(t *testing.T) {
	if watchDrainBudget != 5*time.Second {
		t.Fatalf("watchDrainBudget = %v; the spec names 5s", watchDrainBudget)
	}
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		start := time.Now()
		err := stopWatch(ctx, openWatcher{updates: make(chan jetstream.KeyValueEntry)})
		if !errors.Is(err, errWatchDeliveryOpen) {
			t.Fatalf("stopWatch = %v; want errWatchDeliveryOpen", err)
		}
		if elapsed := time.Since(start); elapsed != watchDrainBudget {
			t.Errorf("stopWatch returned after %v; want %v", elapsed, watchDrainBudget)
		}
	})
}

// putEntry is a delivered live write: only what observe reads is implemented. A zero value reads
// as "{}".
type putEntry struct {
	jetstream.KeyValueEntry
	key      string
	revision uint64
	value    []byte
}

func (e putEntry) Key() string                     { return e.key }
func (e putEntry) Revision() uint64                { return e.revision }
func (e putEntry) Created() time.Time              { return time.Unix(int64(e.revision), 0) }
func (e putEntry) Operation() jetstream.KeyValueOp { return jetstream.KeyValuePut }
func (e putEntry) Value() []byte {
	if e.value == nil {
		return []byte("{}")
	}
	return e.value
}

// closingWatcher delivers what its channel holds and closes the channel on Stop, as nats.go's
// watcher does once it is stopped.
type closingWatcher struct {
	updates chan jetstream.KeyValueEntry
	once    sync.Once
}

func (w *closingWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w *closingWatcher) Stop() error {
	w.once.Do(func() { close(w.updates) })
	return nil
}

// watchOnlyReader is a kvcatalog.CatalogReader with only WatchAll implemented: follow calls
// nothing else.
type watchOnlyReader struct {
	kvcatalog.CatalogReader
	watcher jetstream.KeyWatcher
}

func (r watchOnlyReader) WatchAll(context.Context, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return r.watcher, nil
}

// TestFollowerDeclaresLostFeedOnOutOfOrderDelivery: a watch that delivers revision 5 and then
// revision 3 has broken the ascending order the floor rests on, so the follower declares the feed
// lost (entity-state-follower spec, "The follower checks delivery order"), and still delivers the
// entry with revision 3's value (design D2). The watch delivers the snapshot marker first, so the
// two writes are live and no stored entity is validated.
func TestFollowerDeclaresLostFeedOnOutOfOrderDelivery(t *testing.T) {
	updates := make(chan jetstream.KeyValueEntry, 3)
	updates <- nil
	updates <- putEntry{key: "k5", revision: 5, value: []byte("value at 5")}
	updates <- putEntry{key: "k3", revision: 3, value: []byte("value at 3")}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var delivered []uint64
	values := map[uint64]string{}
	f := &Follower{
		key:    "graph-index",
		logger: slog.New(slog.DiscardHandler),
		deliver: func(_ context.Context, e Entry) {
			delivered = append(delivered, e.Revision)
			values[e.Revision] = string(e.Value) // copied: Value is valid only during Deliver
			if e.Revision == 3 {
				cancel() // both writes are delivered: follow stops the watch and returns
			}
		},
		failed:    func() (int64, map[string]uint64) { return 0, nil },
		reader:    watchOnlyReader{watcher: &closingWatcher{updates: updates}},
		watermark: revlag.New(),
	}

	// follow runs on this goroutine, so its return orders the reads below after both deliveries.
	if err := f.follow(ctx); err != nil {
		t.Fatalf("follow = %v", err)
	}
	if !slices.Equal(delivered, []uint64{5, 3}) {
		t.Errorf("delivered revisions %v; want [5 3]", delivered)
	}
	if want := map[uint64]string{5: "value at 5", 3: "value at 3"}; !maps.Equal(values, want) {
		t.Errorf("delivered values %v; want %v", values, want)
	}
	// Fatal: without a lost feed QueryReady would go on to read the stream status, which the fake
	// reader does not implement.
	if !slices.ContainsFunc(f.Stuck(), func(s string) bool { return strings.Contains(s, "feed lost") }) {
		t.Fatalf("Stuck = %q; want the lost feed named", f.Stuck())
	}
	var ce *errs.ClassifiedError
	err := f.QueryReady(t.Context())
	if !errors.As(err, &ce) || ce.Class != errs.ErrorTransient || ce.Code != graph.ErrorCodeIndexNotReady ||
		!strings.Contains(err.Error(), "feed is lost") {
		t.Errorf("QueryReady = %v; want transient %s for the lost feed", err, graph.ErrorCodeIndexNotReady)
	}
}

// progressFollower is a Follower with only what envelope and Complete read: the watermark, the
// failed count and the progress record. The tests below observe revisions straight into the
// watermark, as observe does for a live write.
func progressFollower(key string) *Follower {
	return &Follower{key: key, logger: slog.New(slog.DiscardHandler), failed: func() (int64, map[string]uint64) { return 0, nil },
		watermark: revlag.New()}
}

// observeAll records revisions 1..n, key k<rev> committed at the bubble's now.
func observeAll(t *testing.T, f *Follower, n uint64) {
	t.Helper()
	for rev := uint64(1); rev <= n; rev++ {
		if err := f.watermark.Observe(rev, fmt.Sprintf("k%d", rev), time.Now()); err != nil {
			t.Fatalf("Observe(%d): %v", rev, err)
		}
	}
}

// heldLowRevision is the history both held-revision tests run: revisions 1..6 are delivered, and
// revision 1 is never completed while revisions 2..6 complete one every 10 s, so the floor stays
// at 0 for 50 s. The envelope is read after each completion and once more 10 s after the last.
// Inside a synctest bubble a wait on time.After advances the fake clock exactly, so each reading is taken
// at a known age (docs/testing.md: the bubble's clock is the ordering event).
func heldLowRevision(t *testing.T, key string) {
	t.Helper()
	synctest.Test(t, func(t *testing.T) {
		f := progressFollower(key)
		observeAll(t, f, 6)
		const target = 6
		read(f, target) // the first reading starts the progress clock
		for rev := uint64(2); rev <= target; rev++ {
			<-time.After(10 * time.Second)
			f.Complete(fmt.Sprintf("k%d", rev), rev)
			status := read(f, target)
			if status.Ready || status.State != graph.IndexStateBuilding || status.IndexedRevision != 0 {
				t.Fatalf("after revision %d completed at %v: envelope %+v; want not ready, building, indexed 0",
					rev, time.Duration(rev-1)*10*time.Second, status)
			}
			// staleness_ms is the age of revision 1, the oldest one not applied: it rises.
			if want := (rev - 1) * 10_000; status.StalenessMs != want {
				t.Errorf("after revision %d: staleness_ms %d; want %d", rev, status.StalenessMs, want)
			}
		}
		// 10 s after the last completion: still inside the 30 s window.
		<-time.After(10 * time.Second)
		if status := read(f, target); status.State != graph.IndexStateBuilding {
			t.Errorf("10 s after the last completion: state %s; want building", status.State)
		}
	})
}

// TestHeldLowRevisionWithProgress: one revision not completed for more than 30 s while later
// revisions keep completing reads not ready and not degraded (entity-state-follower spec, "A held
// low revision while later revisions complete"; owner ruling (a1), PR #550 comment 6721867279).
func TestHeldLowRevisionWithProgress(t *testing.T) {
	heldLowRevision(t, "graph-embedding")
}

// TestGraphIndexHeldRevisionNotDegraded: graph-index reads the same rule, since the rule is the
// follower's and graph-index has no option of its own (design D1: "graph-index changes the same
// way"). The history is the one above under graph-index's status key.
func TestGraphIndexHeldRevisionNotDegraded(t *testing.T) {
	heldLowRevision(t, readiness.KeyGraphIndex)
}

// TestNoCompletionsReadsDegraded: while the floor is behind and no Complete call arrives for more
// than 30 s, the envelope reads degraded (spec, "Nothing completes"); 30 s exactly does not. A
// completion afterwards, even one that does not raise the floor, ends it; caught up is never
// degraded, whatever the age of the last completion, and the 30 s count from the moment the
// follower falls behind, not from the last completion before it.
func TestNoCompletionsReadsDegraded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := progressFollower("graph-embedding")
		observeAll(t, f, 3)
		const target = 3
		read(f, target)
		<-time.After(degradedStuckAfter)
		if status := read(f, target); status.State != graph.IndexStateBuilding {
			t.Fatalf("at exactly %v with no completion: state %s; want building", degradedStuckAfter, status.State)
		}
		<-time.After(time.Millisecond)
		if status := read(f, target); status.State != graph.IndexStateDegraded || status.Ready {
			t.Fatalf("past %v with no completion: envelope %+v; want degraded, not ready", degradedStuckAfter, status)
		}
		f.Complete("k3", 3) // revision 1 still holds the floor at 0
		if status := read(f, target); status.State != graph.IndexStateBuilding ||
			status.IndexedRevision != 0 {
			t.Fatalf("after a completion that leaves the floor: envelope %+v; want building at indexed 0", status)
		}
		f.Complete("k1", 1)
		f.Complete("k2", 2)
		<-time.After(time.Hour)
		if status := read(f, target); !status.Ready || status.State != graph.IndexStateReady {
			t.Fatalf("caught up an hour after the last completion: envelope %+v; want ready", status)
		}
		// That reading saw the last completions; another hour passes caught up, read by the
		// heartbeat, with none. A write then arrives: the follower is behind, but has not been
		// behind for 30 s, so the reading is building, not degraded.
		<-time.After(time.Hour)
		if status := read(f, target); !status.Ready {
			t.Fatalf("caught up two hours after the last completion: envelope %+v; want ready", status)
		}
		if err := f.watermark.Observe(4, "k4", time.Now()); err != nil {
			t.Fatalf("Observe(4): %v", err)
		}
		if status := read(f, 4); status.State != graph.IndexStateBuilding {
			t.Errorf("first reading behind after two hours caught up: state %s; want building", status.State)
		}
	})
}

// TestFirstWriteAfterIdleEmptyGraph: an authoritatively empty graph, read by the heartbeat for
// more than 30 s, counts as caught up, so the first write after it reads building, not degraded
// (spec, "Falling behind after a long time caught up"; Codex review C1 on PR #550).
func TestFirstWriteAfterIdleEmptyGraph(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := progressFollower("graph-embedding")
		f.snapshotDone.Store(true)
		if status := read(f, 0); !status.Ready {
			t.Fatalf("empty graph after the snapshot: envelope %+v; want ready", status)
		}
		for range 40 {
			<-time.After(time.Second)
			if status := read(f, 0); !status.Ready {
				t.Fatalf("idle empty graph: envelope %+v; want ready", status)
			}
		}
		if err := f.watermark.Observe(1, "k1", time.Now()); err != nil {
			t.Fatalf("Observe(1): %v", err)
		}
		if status := read(f, 1); status.State != graph.IndexStateBuilding {
			t.Fatalf("first write after an idle empty graph: state %s; want building", status.State)
		}
	})
}

// read is the envelope compute builds once it has read target from the stream.
func read(f *Follower, target uint64) graph.IndexStatusResponse {
	indexed, indexedAt := f.watermark.IndexedAt()
	return f.envelope(indexed, indexedAt, target)
}

// TestEnvelopeCarriesFailedReasons: the envelope carries the plug-in's per-reason counts from the
// same Failed call as failed_count (entity-state-follower spec, "Failures with reasons"; design
// D3). The plug-in's failures change on every call while revisions complete on other goroutines,
// so an envelope built from two calls would pair a count with reasons from another moment; each
// envelope is read once Failed has returned to it, on this goroutine. Run under -race: the
// envelope's map is marshalled here while the plug-in keeps changing its own state.
func TestEnvelopeCarriesFailedReasons(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	f := progressFollower("graph-embedding")
	f.failed = func() (int64, map[string]uint64) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		n := uint64(calls)
		// A fresh map per call, as the plug-in contract requires: count 3n, of which 2n timeouts.
		return int64(3 * n), map[string]uint64{"timeout": 2 * n, "delete_failed": n}
	}
	const target = 100
	observeAll(t, f, target)

	var completing sync.WaitGroup
	for g := range 4 {
		completing.Go(func() {
			for rev := uint64(g + 1); rev <= target; rev += 4 {
				f.Complete(fmt.Sprintf("k%d", rev), rev)
			}
		})
	}
	for range 50 {
		mu.Lock()
		before := calls
		mu.Unlock()
		status := read(f, target)
		mu.Lock()
		after := calls
		mu.Unlock()
		if after != before+1 {
			t.Fatalf("one envelope called Failed %d times; want once", after-before)
		}
		n := uint64(after)
		want := map[string]uint64{"timeout": 2 * n, "delete_failed": n}
		if status.FailedCount != 3*n || !maps.Equal(status.FailedReasons, want) {
			t.Fatalf("envelope failed_count %d, failed_reasons %v; want %d and %v from one call",
				status.FailedCount, status.FailedReasons, 3*n, want)
		}
		if status.Ready || status.State != graph.IndexStateDegraded {
			t.Fatalf("envelope with failures: ready %v, state %s; want not ready, degraded", status.Ready, status.State)
		}
		data, err := json.Marshal(status)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"failed_reasons":{"delete_failed":`) {
			t.Fatalf("published envelope %s carries no failed_reasons", data)
		}
	}
	completing.Wait()
}

// TestGraphIndexEnvelopeUnchanged: a plug-in that returns nil reasons, as graph-index does, gets
// the envelope it had before reasons existed: failed_count only (spec, "A plug-in with no
// reasons"). Reasons beside a zero count are not published, and neither key is written for no
// failures, whether the plug-in returns nil or an empty map. first_failure_at is never written.
func TestGraphIndexEnvelopeUnchanged(t *testing.T) {
	cases := []struct {
		name    string
		count   int64
		reasons map[string]uint64
		want    string // the failure keys of the marshalled envelope
	}{
		{"graph-index's failures", 2, nil, `"failed_count":2`},
		{"failures with an empty map", 2, map[string]uint64{}, `"failed_count":2`},
		{"no failures, nil reasons", 0, nil, ``},
		{"no failures, empty map", 0, map[string]uint64{}, ``},
		{"no failures, stray reasons", 0, map[string]uint64{"timeout": 1}, ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := progressFollower(readiness.KeyGraphIndex)
			f.failed = func() (int64, map[string]uint64) { return tc.count, tc.reasons }
			data, err := json.Marshal(read(f, 0))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, key := range []string{"failed_count", "failed_reasons", "first_failure_at"} {
				if i := strings.Index(string(data), `"`+key+`":`); i >= 0 {
					end := strings.IndexAny(string(data[i:]), ",}")
					got = append(got, string(data[i:i+end]))
				}
			}
			if strings.Join(got, ",") != tc.want {
				t.Errorf("envelope %s: failure keys %q; want %q", data, strings.Join(got, ","), tc.want)
			}
		})
	}
}

// requireNotReady requires err to be the transient index_not_ready refusal.
func requireNotReady(t *testing.T, what string, err error) {
	t.Helper()
	var ce *errs.ClassifiedError
	if !errors.As(err, &ce) || ce.Class != errs.ErrorTransient || ce.Code != graph.ErrorCodeIndexNotReady {
		t.Errorf("%s: QueryReady = %v; want transient %s", what, err, graph.ErrorCodeIndexNotReady)
	}
}

// builtFollower is a Follower whose first build is complete, so QueryReady never reads the
// stream, with the plug-in's Failed and Withhold as given.
func builtFollower(key string, failed func() (int64, map[string]uint64), withhold func() int64) *Follower {
	f := progressFollower(key)
	f.failed, f.withhold = failed, withhold
	f.bootstrapped.Store(true)
	return f
}

// TestWithholdNarrowsQueryReady: after the first build a plug-in's Withhold, not Failed, decides
// whether failures withhold queries (entity-state-follower spec, "A failure the plug-in does not
// withhold on"), while the envelope and Stuck keep reading Failed. One failure Withhold does not
// count lets QueryReady answer; counted, it refuses; uncounted again, the refusal clears. A reset
// and a lost feed refuse whatever Withhold says.
func TestWithholdNarrowsQueryReady(t *testing.T) {
	var withheld atomic.Int64
	failed := func() (int64, map[string]uint64) { return 1, map[string]uint64{"content_error": 1} }
	f := builtFollower("graph-embedding", failed, withheld.Load)

	if err := f.QueryReady(t.Context()); err != nil {
		t.Errorf("one failure Withhold does not count: QueryReady = %v; want nil", err)
	}
	if status := read(f, 0); status.FailedCount != 1 || status.State != graph.IndexStateDegraded || status.Ready {
		t.Errorf("envelope with one failure Withhold does not count = %+v; want failed_count 1, degraded", status)
	}
	if !slices.ContainsFunc(f.Stuck(), func(s string) bool { return strings.Contains(s, "1 unresolved failures: content_error 1") }) {
		t.Errorf("Stuck = %q; want the unresolved failure named with its reason", f.Stuck())
	}

	withheld.Store(1)
	requireNotReady(t, "Withhold above 0", f.QueryReady(t.Context()))
	withheld.Store(0)
	if err := f.QueryReady(t.Context()); err != nil {
		t.Errorf("Withhold back at 0: QueryReady = %v; want nil", err)
	}

	f.feedLost.Store(true)
	requireNotReady(t, "a lost feed with Withhold at 0", f.QueryReady(t.Context()))
	f.Reset(graph.StateResetReason("test"))
	var ce *errs.ClassifiedError
	if err := f.QueryReady(t.Context()); !errors.As(err, &ce) || ce.Code != graph.ErrorCodeGraphStateResetRequired {
		t.Errorf("a reset with Withhold at 0: QueryReady = %v; want %s", err, graph.ErrorCodeGraphStateResetRequired)
	}
}

// TestGraphIndexWithholdUnchanged: with no Withhold, as graph-index configures its follower,
// QueryReady withholds on Failed's count as before (entity-state-follower spec, "No Withhold").
func TestGraphIndexWithholdUnchanged(t *testing.T) {
	var count atomic.Int64
	f := builtFollower(readiness.KeyGraphIndex, func() (int64, map[string]uint64) { return count.Load(), nil }, nil)
	if err := f.QueryReady(t.Context()); err != nil {
		t.Fatalf("no failures: QueryReady = %v; want nil", err)
	}
	count.Store(1)
	requireNotReady(t, "one failure, no Withhold", f.QueryReady(t.Context()))
	count.Store(0)
	if err := f.QueryReady(t.Context()); err != nil {
		t.Errorf("failure resolved: QueryReady = %v; want nil", err)
	}
}

// TestStuckNamesReasons: Stuck names Failed's count as unresolved failures, without assuming they
// are index writes, and each reason with its count when Failed returns reasons (entity-state-follower
// spec, "Stuck names unresolved failures without assuming their kind"). Reasons are named in sorted
// order, so the sentence is the same on every call.
func TestStuckNamesReasons(t *testing.T) {
	cases := []struct {
		name    string
		count   int64
		reasons map[string]uint64
		want    []string
	}{
		{"one reason", 2, map[string]uint64{"refused_entity": 2}, []string{"2 unresolved failures: refused_entity 2"}},
		{"two reasons", 3, map[string]uint64{"publication_failed": 1, "refused_entity": 2},
			[]string{"3 unresolved failures: publication_failed 1, refused_entity 2"}},
		{"no reasons", 1, nil, []string{"1 unresolved failures"}},
		{"none", 0, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := builtFollower("graph-clustering", func() (int64, map[string]uint64) { return tc.count, tc.reasons }, nil)
			if got := f.Stuck(); !slices.Equal(got, tc.want) {
				t.Errorf("Stuck = %q; want %q", got, tc.want)
			}
		})
	}
}
