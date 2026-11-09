package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/graphview"
)

// feedKV is a memKV whose Watch returns a watcher on the test's own unbuffered channel: the
// test sends each entry, the end-of-snapshot nil included, and a send returns only once Run has
// received it. Stop closes the channel, so a test sends nothing after it ends Run's context;
// lose closes it while Run's context is live, as nats.go does when the bucket is deleted.
type feedKV struct {
	*memKV
	watcher *memWatcher
	watches atomic.Int32
}

func newFeedKV() *feedKV {
	return &feedKV{memKV: newMemKV(), watcher: &memWatcher{updates: make(chan jetstream.KeyValueEntry)}}
}

func (f *feedKV) Watch(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	f.watches.Add(1)
	return f.watcher, nil
}

// send hands one entry to Run and returns once Run has received it.
func (f *feedKV) send(entry jetstream.KeyValueEntry) { f.watcher.updates <- entry }

// lose closes the watch's update channel under a live Run.
func (f *feedKV) lose() { _ = f.watcher.Stop() }

// runCache starts a VectorCache over index on its own goroutine and returns it with a stop
// function that ends Run's context and returns Run's error, and Run's result channel.
func runCache(t *testing.T, index jetstream.KeyValue) (*VectorCache, func() error, <-chan error) {
	t.Helper()
	cache := NewVectorCache(newTestStorage(t, index, newMemKV()))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- cache.Run(ctx) }()
	return cache, func() error { cancel(); return <-done }, done
}

// generatedEntry is a put of a generated record for id with vector.
func generatedEntry(t *testing.T, id string, vector ...float32) jetstream.KeyValueEntry {
	t.Helper()
	data, err := json.Marshal(&Record{EntityID: id, Vector: vector, Status: StatusGenerated})
	if err != nil {
		t.Fatal(err)
	}
	return &memKVEntry{key: id, value: data, op: jetstream.KeyValuePut}
}

// undecodableEntry is a put under id of a value that is not a record.
func undecodableEntry(id string) jetstream.KeyValueEntry {
	return &memKVEntry{key: id, value: []byte("{not json"), op: jetstream.KeyValuePut}
}

// ids lists the entity IDs of a result, in order.
func ids(scored []ScoredEntity) []string {
	out := make([]string, len(scored))
	for i, s := range scored {
		out[i] = s.EntityID
	}
	return out
}

// TestVectorCacheRunRefusesNilContext: Run(nil) returns an invalid-input error and opens no
// watch.
func TestVectorCacheRunRefusesNilContext(t *testing.T) {
	kv := newFeedKV()
	cache := NewVectorCache(newTestStorage(t, kv, newMemKV()))
	err := cache.Run(nil)
	if !errs.IsInvalid(err) {
		t.Fatalf("Run(nil) = %v; want an invalid-input error", err)
	}
	if n := kv.watches.Load(); n != 0 {
		t.Fatalf("Run(nil) opened %d watches; want 0", n)
	}
}

// TestVectorCacheSecondRunRefused: a second Run while one runs returns an error without
// opening a watch, and the first keeps running and answering until its context ends.
func TestVectorCacheSecondRunRefused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kv := newFeedKV()
		cache, stop, done := runCache(t, kv)
		kv.send(generatedEntry(t, "o.p.s.d.t.a", 1, 0))
		kv.send(nil)
		synctest.Wait() // the first Run has applied the snapshot and waits for the next entry

		second, cancelSecond := context.WithCancel(t.Context())
		defer cancelSecond()
		if err := cache.Run(second); err == nil {
			t.Fatal("a second Run returned nil; want a refusal")
		}
		if n := kv.watches.Load(); n != 1 {
			t.Fatalf("watches opened = %d; want 1 (the second Run opened one)", n)
		}
		synctest.Wait() // anything the second call started has run
		select {
		case err := <-done:
			t.Fatalf("the first Run returned %v after the second was refused", err)
		default:
		}
		if got, _, err := cache.FindSimilar("", []float32{1, 0}, nil, 10); err != nil || len(got) != 1 {
			t.Fatalf("FindSimilar = %v, %v; want the first Run's one vector", got, err)
		}
		if err := stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("the first Run returned %v; want context.Canceled", err)
		}
	})
}

// TestVectorCacheRunLeavesNothingBehind: Run started, queried and cancelled inside a bubble
// returns context.Canceled, and the bubble ends with no goroutine left (synctest.Test fails
// otherwise). After Run returns the cache does not answer.
func TestVectorCacheRunLeavesNothingBehind(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kv := newFeedKV()
		cache, stop, _ := runCache(t, kv)
		kv.send(generatedEntry(t, "o.p.s.d.t.a", 1, 0))
		kv.send(nil)
		synctest.Wait() // Run has applied the snapshot
		if got, _, err := cache.FindSimilar("", []float32{1, 0}, nil, 10); err != nil ||
			!slices.Equal(ids(got), []string{"o.p.s.d.t.a"}) {
			t.Fatalf("FindSimilar = %v, %v; want [o.p.s.d.t.a], nil", got, err)
		}
		if err := stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v; want context.Canceled", err)
		}
		if _, _, err := cache.FindSimilar("", []float32{1, 0}, nil, 10); !errors.Is(err, graphview.ErrNotReady) {
			t.Fatalf("FindSimilar after Run returned = %v; want an error matching ErrNotReady", err)
		}
	})
}

// TestVectorCacheNotAuthoritative: FindSimilar returns an error matching graphview.ErrNotReady
// and no entities before any Run, before the watch's snapshot has been applied, after the watch
// was lost, and after Run returned. After Run returned the view answers ErrViewStopped, which
// does not itself match ErrNotReady, so the case proves the cache maps it.
func TestVectorCacheNotAuthoritative(t *testing.T) {
	query := []float32{1, 0}
	refused := func(t *testing.T, cache *VectorCache, when string) {
		t.Helper()
		got, unreadable, err := cache.FindSimilar("", query, nil, 10)
		if !errors.Is(err, graphview.ErrNotReady) || got != nil || unreadable != 0 {
			t.Fatalf("FindSimilar %s = %v, %d, %v; want nil, 0 and an error matching ErrNotReady",
				when, got, unreadable, err)
		}
	}

	t.Run("before any Run", func(t *testing.T) {
		refused(t, NewVectorCache(newTestStorage(t, newFeedKV(), newMemKV())), "before any Run")
	})

	t.Run("before the snapshot", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			kv := newFeedKV()
			cache, stop, _ := runCache(t, kv)
			synctest.Wait() // Run has opened the view's watch and waits for the first entry
			refused(t, cache, "before the first entry")
			kv.send(generatedEntry(t, "o.p.s.d.t.a", 1, 0))
			synctest.Wait() // Run has applied the entry and waits for the next
			refused(t, cache, "before the snapshot's end")
			kv.send(nil)
			synctest.Wait()
			if _, _, err := cache.FindSimilar("", query, nil, 10); err != nil {
				t.Fatalf("FindSimilar after the snapshot's end = %v; want nil", err)
			}
			if err := stop(); !errors.Is(err, context.Canceled) {
				t.Fatalf("Run returned %v; want context.Canceled", err)
			}
		})
	})

	t.Run("after the watch was lost", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			kv := newFeedKV()
			cache, _, done := runCache(t, kv)
			kv.send(generatedEntry(t, "o.p.s.d.t.a", 1, 0))
			kv.send(nil)
			synctest.Wait()
			kv.lose()
			// Run returns on its own once the channel closes; its context is still live.
			if err := <-done; !errors.Is(err, graphview.ErrWatcherLost) {
				t.Fatalf("Run returned %v; want an error matching ErrWatcherLost", err)
			}
			refused(t, cache, "after the watch was lost")
		})
	})

	t.Run("after Run returned", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			kv := newFeedKV()
			cache, stop, _ := runCache(t, kv)
			kv.send(generatedEntry(t, "o.p.s.d.t.a", 1, 0))
			kv.send(nil)
			synctest.Wait()
			if _, _, err := cache.FindSimilar("", query, nil, 10); err != nil {
				t.Fatalf("FindSimilar while running = %v; want nil", err)
			}
			if err := stop(); !errors.Is(err, context.Canceled) {
				t.Fatalf("Run returned %v; want context.Canceled", err)
			}
			refused(t, cache, "after Run returned")
		})
	})
}

// TestVectorCachePoisonCountedAndHeals: a record that does not decode is not scored and is
// counted, the other vectors still answer, and the count includes keys keep refuses (it is every
// unreadable record, not only those in scope). A later valid write of the key scores it again and
// stops counting it; a delete of an unreadable key stops counting it too.
func TestVectorCachePoisonCountedAndHeals(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kv := newFeedKV()
		cache, stop, _ := runCache(t, kv)
		kv.send(generatedEntry(t, "o.p.s.d.t.a", 1, 0))
		kv.send(generatedEntry(t, "o.p.s.d.t.b", 1, 0))
		kv.send(undecodableEntry("o.p.s.d.t.c")) // unreadable from the snapshot on
		kv.send(nil)
		kv.send(undecodableEntry("o.p.s.d.t.a")) // replaces a good vector
		synctest.Wait()

		query := []float32{1, 0}
		check := func(when string, keep func(string) bool, wantIDs []string, wantUnreadable int) {
			t.Helper()
			got, unreadable, err := cache.FindSimilar("", query, keep, 10)
			if err != nil {
				t.Fatalf("%s: FindSimilar = %v; want an answer", when, err)
			}
			if !slices.Equal(ids(got), wantIDs) || unreadable != wantUnreadable {
				t.Fatalf("%s: FindSimilar = %v, %d unreadable; want %v, %d", when, ids(got), unreadable, wantIDs, wantUnreadable)
			}
		}
		check("two unreadable", nil, []string{"o.p.s.d.t.b"}, 2)
		check("two unreadable, scoped to b", func(id string) bool { return id == "o.p.s.d.t.b" }, []string{"o.p.s.d.t.b"}, 2)

		kv.send(generatedEntry(t, "o.p.s.d.t.a", 1, 0))
		synctest.Wait()
		check("a healed", nil, []string{"o.p.s.d.t.a", "o.p.s.d.t.b"}, 1)

		kv.send(&memKVEntry{key: "o.p.s.d.t.c", op: jetstream.KeyValueDelete})
		synctest.Wait()
		check("c deleted", nil, []string{"o.p.s.d.t.a", "o.p.s.d.t.b"}, 0)

		if err := stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v; want context.Canceled", err)
		}
	})
}

// TestVectorCacheFollowsRecordStatus: a record that stops being generated (pending, failed,
// deleted or purged) leaves the cache even when it still carries a vector, and one without a
// vector never enters it; none of them is counted unreadable.
func TestVectorCacheFollowsRecordStatus(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kv := newFeedKV()
		cache, stop, _ := runCache(t, kv)
		for _, id := range []string{"o.p.s.d.t.a", "o.p.s.d.t.b", "o.p.s.d.t.c", "o.p.s.d.t.d", "o.p.s.d.t.e"} {
			kv.send(generatedEntry(t, id, 1, 0))
		}
		kv.send(generatedEntry(t, "o.p.s.d.t.f")) // generated, but no vector
		kv.send(nil)
		pending, err := json.Marshal(&Record{EntityID: "o.p.s.d.t.a", Vector: []float32{1, 0}, Status: StatusPending})
		if err != nil {
			t.Fatal(err)
		}
		failedRec, err := json.Marshal(&Record{EntityID: "o.p.s.d.t.b", Vector: []float32{1, 0}, Status: StatusFailed})
		if err != nil {
			t.Fatal(err)
		}
		kv.send(&memKVEntry{key: "o.p.s.d.t.a", value: pending, op: jetstream.KeyValuePut})
		kv.send(&memKVEntry{key: "o.p.s.d.t.b", value: failedRec, op: jetstream.KeyValuePut})
		kv.send(&memKVEntry{key: "o.p.s.d.t.c", op: jetstream.KeyValueDelete})
		kv.send(&memKVEntry{key: "o.p.s.d.t.d", op: jetstream.KeyValuePurge})
		synctest.Wait()
		got, unreadable, err := cache.FindSimilar("", []float32{1, 0}, nil, 10)
		if err != nil || unreadable != 0 || !slices.Equal(ids(got), []string{"o.p.s.d.t.e"}) {
			t.Fatalf("FindSimilar = %v, %d, %v; want [o.p.s.d.t.e], 0, nil", ids(got), unreadable, err)
		}
		if err := stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v; want context.Canceled", err)
		}
	})
}

// warmCache runs a cache inside the current bubble over the generated vectors given and
// returns it once the snapshot has been applied, with its stop function.
func warmCache(t *testing.T, vectors map[string][]float32) (*VectorCache, func() error) {
	t.Helper()
	kv := newFeedKV()
	cache, stop, _ := runCache(t, kv)
	for id, v := range vectors {
		kv.send(generatedEntry(t, id, v...))
	}
	kv.send(nil)
	synctest.Wait() // Run has applied the snapshot
	return cache, stop
}

// TestFindSimilarOrderAndLimit: results are ordered by similarity, ties by entity ID, before
// the limit cuts them, so a tie across the limit keeps the smaller IDs; the excluded ID is
// never scored; a limit of 0 or less returns no entities; a limit at or above the number of
// vectors returns them all; an empty cache answers no entities and no error. The expected orders
// are worked out by hand from the vectors: against (1,0) a, c and e score 1, d scores 1/sqrt(2),
// b scores 0.
func TestFindSimilarOrderAndLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		empty, stopEmpty := warmCache(t, nil)
		if got, unreadable, err := empty.FindSimilar("", []float32{1, 0}, nil, 10); err != nil || len(got) != 0 || unreadable != 0 {
			t.Fatalf("empty cache: FindSimilar = %v, %d, %v; want none, 0, nil", got, unreadable, err)
		}
		if err := stopEmpty(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v; want context.Canceled", err)
		}

		cache, stop := warmCache(t, map[string][]float32{
			"o.p.s.d.t.c": {1, 0},
			"o.p.s.d.t.a": {2, 0},
			"o.p.s.d.t.b": {0, 1},
			"o.p.s.d.t.d": {1, 1},
			"o.p.s.d.t.e": {3, 0},
		})
		all := []string{"o.p.s.d.t.a", "o.p.s.d.t.c", "o.p.s.d.t.e", "o.p.s.d.t.d", "o.p.s.d.t.b"}
		query := []float32{1, 0}
		for _, tc := range []struct {
			name    string
			exclude string
			limit   int
			want    []string
		}{
			{"all", "", 10, all},
			{"limit equal to the vectors", "", 5, all},
			{"limit 1", "", 1, []string{"o.p.s.d.t.a"}},
			{"a tie across the limit", "", 2, []string{"o.p.s.d.t.a", "o.p.s.d.t.c"}},
			{"excluded", "o.p.s.d.t.a", 2, []string{"o.p.s.d.t.c", "o.p.s.d.t.e"}},
			{"limit 4", "", 4, []string{"o.p.s.d.t.a", "o.p.s.d.t.c", "o.p.s.d.t.e", "o.p.s.d.t.d"}},
			{"limit 0", "", 0, nil},
			{"negative limit", "", -1, nil},
		} {
			got, unreadable, err := cache.FindSimilar(tc.exclude, query, nil, tc.limit)
			if err != nil || unreadable != 0 {
				t.Fatalf("%s: FindSimilar = %d unreadable, %v; want 0, nil", tc.name, unreadable, err)
			}
			if gotIDs := ids(got); !slices.Equal(gotIDs, tc.want) {
				t.Errorf("%s: FindSimilar = %v; want %v", tc.name, gotIDs, tc.want)
			}
		}
		if err := stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v; want context.Canceled", err)
		}
	})
}

// TestFindSimilarFromCache_DeterministicTopKAtEqualSimilarity (pin: similarity_determinism_test.go):
// over many calls the top-k of an all-equal-similarity corpus is the same, and is the k
// lexicographically smallest IDs.
func TestFindSimilarFromCache_DeterministicTopKAtEqualSimilarity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const k = 8
		var all []string
		vectors := make(map[string][]float32)
		for n := 1; n <= 20; n++ {
			id := "o.p.s.d.t.e" + string(rune('a'+n-1))
			all = append(all, id)
			vectors[id] = []float32{1, 0, 0}
		}
		cache, stop := warmCache(t, vectors)
		slices.Sort(all)
		want := all[:k]
		for run := range 100 {
			got, _, err := cache.FindSimilar("", []float32{1, 1, 0}, nil, k)
			if err != nil {
				t.Fatalf("FindSimilar = %v; want an answer", err)
			}
			if !slices.Equal(ids(got), want) {
				t.Fatalf("run %d: top-k = %v; want %v", run, ids(got), want)
			}
		}
		if err := stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v; want context.Canceled", err)
		}
	})
}

// TestFindSimilarFromCache_ScopeFilter (pin: storage_scope_test.go) guards ADR-071: keep is
// applied before scoring on the cache path, so a scoped search returns only in-scope IDs; a
// nil keep scores every cached entity.
func TestFindSimilarFromCache_ScopeFilter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache, stop := warmCache(t, map[string][]float32{
			"acme.web.python.svc.fn.parse":  {1, 0, 0},
			"acme.web.python.svc.fn.build":  {0, 1, 0},
			"acme.web.docs.site.doc.readme": {0, 0, 1},
			"acme.web.docs.site.doc.guide":  {1, 1, 0},
		})
		query := []float32{1, 1, 1}
		var scored []string
		keep := func(id string) bool {
			scored = append(scored, id)
			return strings.HasPrefix(id, "acme.web.docs.")
		}
		got, _, err := cache.FindSimilar("", query, keep, 10)
		if err != nil {
			t.Fatalf("FindSimilar = %v; want an answer", err)
		}
		want := []string{"acme.web.docs.site.doc.guide", "acme.web.docs.site.doc.readme"}
		if !slices.Equal(ids(got), want) {
			t.Fatalf("scoped FindSimilar = %v; want %v", ids(got), want)
		}
		if len(scored) != 4 {
			t.Fatalf("keep was asked about %d entities; want all 4", len(scored))
		}
		if got, _, err := cache.FindSimilar("", query, nil, 10); err != nil || len(got) != 4 {
			t.Fatalf("unscoped FindSimilar = %v, %v; want 4 entities", ids(got), err)
		}
		// Scope before the limit: against (1,0,0) the out-of-scope parse scores 1, above every
		// in-scope vector (guide 1/sqrt(2), readme 0), so a limit of 1 must still answer guide.
		inDocs := func(id string) bool { return strings.HasPrefix(id, "acme.web.docs.") }
		if got, _, err := cache.FindSimilar("", []float32{1, 0, 0}, inDocs, 1); err != nil ||
			!slices.Equal(ids(got), []string{"acme.web.docs.site.doc.guide"}) {
			t.Fatalf("scoped FindSimilar, limit 1 = %v, %v; want [acme.web.docs.site.doc.guide]", ids(got), err)
		}
		if err := stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v; want context.Canceled", err)
		}
	})
}

// TestFindSimilarReadsOneMoment: a FindSimilar answers from the vectors and the count it read
// at one moment. A record deleted, and another made unreadable, after that read and before the
// call returns are still in its answer as they were when read; the next call sees both changes.
func TestFindSimilarReadsOneMoment(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kv := newFeedKV()
		cache, stop, _ := runCache(t, kv)
		kv.send(generatedEntry(t, "o.p.s.d.t.a", 1, 0))
		kv.send(generatedEntry(t, "o.p.s.d.t.b", 1, 0))
		kv.send(nil)
		synctest.Wait()

		changed := false
		keep := func(string) bool {
			if !changed {
				changed = true
				kv.send(&memKVEntry{key: "o.p.s.d.t.b", op: jetstream.KeyValueDelete})
				kv.send(undecodableEntry("o.p.s.d.t.a"))
				synctest.Wait() // Run has applied both
			}
			return true
		}
		got, unreadable, err := cache.FindSimilar("", []float32{1, 0}, keep, 10)
		if err != nil || unreadable != 0 || !slices.Equal(ids(got), []string{"o.p.s.d.t.a", "o.p.s.d.t.b"}) {
			t.Fatalf("FindSimilar during the changes = %v, %d, %v; want [a b], 0, nil", ids(got), unreadable, err)
		}
		got, unreadable, err = cache.FindSimilar("", []float32{1, 0}, nil, 10)
		if err != nil || unreadable != 1 || len(got) != 0 {
			t.Fatalf("FindSimilar after the changes = %v, %d, %v; want [], 1, nil", ids(got), unreadable, err)
		}
		if err := stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v; want context.Canceled", err)
		}
	})
}

// TestVectorCacheInvalidationLinearizesWithActiveRead (pin: storage_cache_health_test.go): a
// FindSimilar whose read was made while the cache answered returns that answer even when Run
// stops and returns before the call does, and Run does not wait for the call; once Run has
// returned, a new FindSimilar is refused.
func TestVectorCacheInvalidationLinearizesWithActiveRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache, stop := warmCache(t, map[string][]float32{"o.p.s.d.t.a": {1, 0}})
		entered, release := make(chan struct{}), make(chan struct{})
		type answer struct {
			got []ScoredEntity
			err error
		}
		read := make(chan answer, 1)
		go func() {
			got, _, err := cache.FindSimilar("", []float32{1, 0}, func(string) bool {
				close(entered)
				<-release
				return true
			}, 10)
			read <- answer{got, err}
		}()
		<-entered // FindSimilar has read the vectors and waits inside keep
		if err := stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v; want context.Canceled", err)
		}
		close(release)
		if a := <-read; a.err != nil || !slices.Equal(ids(a.got), []string{"o.p.s.d.t.a"}) {
			t.Fatalf("a read made while the cache answered = %v, %v; want [o.p.s.d.t.a], nil", ids(a.got), a.err)
		}
		if _, _, err := cache.FindSimilar("", []float32{1, 0}, nil, 10); !errors.Is(err, graphview.ErrNotReady) {
			t.Fatalf("FindSimilar after Run returned = %v; want an error matching ErrNotReady", err)
		}
	})
}
