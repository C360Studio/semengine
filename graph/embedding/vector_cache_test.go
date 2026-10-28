package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/pkg/errs"
)

// feedKV is a memKV whose Watch returns a watcher on the test's own unbuffered channel: the
// test sends each entry, the end-of-snapshot nil included, and a send returns only once Run has
// received it. Stop closes the channel, so a test sends nothing after it ends Run's context.
type feedKV struct {
	*memKV
	updates chan jetstream.KeyValueEntry
	watches atomic.Int32
}

func newFeedKV() *feedKV {
	return &feedKV{memKV: newMemKV(), updates: make(chan jetstream.KeyValueEntry)}
}

func (f *feedKV) Watch(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	f.watches.Add(1)
	return &memWatcher{updates: f.updates}, nil
}

// runCache starts a VectorCache over index on its own goroutine and returns it with a stop
// function that ends Run's context and returns Run's error.
func runCache(t *testing.T, index jetstream.KeyValue) (*VectorCache, func() error) {
	t.Helper()
	cache := NewVectorCache(newTestStorage(t, index, newMemKV()))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- cache.Run(ctx) }()
	return cache, func() error { cancel(); return <-done }
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
		cache := NewVectorCache(newTestStorage(t, kv, newMemKV()))
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- cache.Run(ctx) }()
		kv.updates <- generatedEntry(t, "o.p.s.d.t.a", 1, 0)
		kv.updates <- nil
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
		if got, ok := cache.FindSimilar("", []float32{1, 0}, nil, 10); !ok || len(got) != 1 {
			t.Fatalf("FindSimilar = %v, %v; want the first Run's one vector, authoritative", got, ok)
		}

		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
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
		cache, stop := runCache(t, kv)
		kv.updates <- generatedEntry(t, "o.p.s.d.t.a", 1, 0)
		kv.updates <- nil
		synctest.Wait() // Run has applied the snapshot
		if got, ok := cache.FindSimilar("", []float32{1, 0}, nil, 10); !ok ||
			!slices.Equal(ids(got), []string{"o.p.s.d.t.a"}) {
			t.Fatalf("FindSimilar = %v, %v; want [o.p.s.d.t.a], true", got, ok)
		}
		if err := stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v; want context.Canceled", err)
		}
		if _, ok := cache.FindSimilar("", []float32{1, 0}, nil, 10); ok {
			t.Fatal("FindSimilar answered after Run returned")
		}
	})
}

// TestVectorCacheNotAuthoritative: FindSimilar returns false before the watch's snapshot has
// been applied, after a record that does not decode (for the rest of that Run, whatever
// arrives after it), and after Run returned.
func TestVectorCacheNotAuthoritative(t *testing.T) {
	query := []float32{1, 0}

	t.Run("before the snapshot", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			kv := newFeedKV()
			cache, stop := runCache(t, kv)
			kv.updates <- generatedEntry(t, "o.p.s.d.t.a", 1, 0)
			synctest.Wait() // Run has applied the entry and waits for the next
			if got, ok := cache.FindSimilar("", query, nil, 10); ok {
				t.Fatalf("FindSimilar before the snapshot's end = %v, true; want false", got)
			}
			kv.updates <- nil
			synctest.Wait()
			if _, ok := cache.FindSimilar("", query, nil, 10); !ok {
				t.Fatal("FindSimilar after the snapshot's end = false; want true")
			}
			if err := stop(); !errors.Is(err, context.Canceled) {
				t.Fatalf("Run returned %v; want context.Canceled", err)
			}
		})
	})

	t.Run("after an undecodable record", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			kv := newFeedKV()
			cache, stop := runCache(t, kv)
			kv.updates <- generatedEntry(t, "o.p.s.d.t.a", 1, 0)
			kv.updates <- nil
			kv.updates <- &memKVEntry{key: "o.p.s.d.t.a", value: []byte("{not json"), op: jetstream.KeyValuePut}
			synctest.Wait()
			if got, ok := cache.FindSimilar("", query, nil, 10); ok {
				t.Fatalf("FindSimilar after an undecodable record = %v, true; want false", got)
			}
			// A later good record does not make the cache authoritative again in this Run.
			kv.updates <- generatedEntry(t, "o.p.s.d.t.a", 1, 0)
			synctest.Wait()
			if got, ok := cache.FindSimilar("", query, nil, 10); ok {
				t.Fatalf("FindSimilar after a later good record = %v, true; want false", got)
			}
			if err := stop(); !errors.Is(err, context.Canceled) {
				t.Fatalf("Run returned %v; want context.Canceled", err)
			}
		})
	})

	t.Run("after Run returned", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			kv := newFeedKV()
			cache, stop := runCache(t, kv)
			kv.updates <- generatedEntry(t, "o.p.s.d.t.a", 1, 0)
			kv.updates <- nil
			synctest.Wait()
			if _, ok := cache.FindSimilar("", query, nil, 10); !ok {
				t.Fatal("FindSimilar while running = false; want true")
			}
			if err := stop(); !errors.Is(err, context.Canceled) {
				t.Fatalf("Run returned %v; want context.Canceled", err)
			}
			if got, ok := cache.FindSimilar("", query, nil, 10); ok {
				t.Fatalf("FindSimilar after Run returned = %v, true; want false", got)
			}
		})
	})
}

// TestVectorCacheFollowsRecordStatus: a record that stops being generated (pending, failed,
// deleted or purged) leaves the cache, and one without a vector never enters it.
func TestVectorCacheFollowsRecordStatus(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kv := newFeedKV()
		cache, stop := runCache(t, kv)
		for _, id := range []string{"o.p.s.d.t.a", "o.p.s.d.t.b", "o.p.s.d.t.c", "o.p.s.d.t.d", "o.p.s.d.t.e"} {
			kv.updates <- generatedEntry(t, id, 1, 0)
		}
		kv.updates <- generatedEntry(t, "o.p.s.d.t.f") // generated, but no vector
		kv.updates <- nil
		pending, err := json.Marshal(&Record{EntityID: "o.p.s.d.t.a", Status: StatusPending})
		if err != nil {
			t.Fatal(err)
		}
		failedRec, err := json.Marshal(&Record{EntityID: "o.p.s.d.t.b", Status: StatusFailed})
		if err != nil {
			t.Fatal(err)
		}
		kv.updates <- &memKVEntry{key: "o.p.s.d.t.a", value: pending, op: jetstream.KeyValuePut}
		kv.updates <- &memKVEntry{key: "o.p.s.d.t.b", value: failedRec, op: jetstream.KeyValuePut}
		kv.updates <- &memKVEntry{key: "o.p.s.d.t.c", op: jetstream.KeyValueDelete}
		kv.updates <- &memKVEntry{key: "o.p.s.d.t.d", op: jetstream.KeyValuePurge}
		synctest.Wait()
		got, ok := cache.FindSimilar("", []float32{1, 0}, nil, 10)
		if !ok || !slices.Equal(ids(got), []string{"o.p.s.d.t.e"}) {
			t.Fatalf("FindSimilar = %v, %v; want [o.p.s.d.t.e], true", ids(got), ok)
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
	cache, stop := runCache(t, kv)
	for id, v := range vectors {
		kv.updates <- generatedEntry(t, id, v...)
	}
	kv.updates <- nil
	synctest.Wait() // Run has applied the snapshot
	return cache, stop
}

// TestFindSimilarOrderAndLimit: results are ordered by similarity, ties by entity ID, before
// the limit cuts them, so a tie across the limit keeps the smaller IDs; the excluded ID is
// never scored; a limit of 0 or less returns no entities. The expected orders are worked out
// by hand from the vectors: against (1,0) a, c and e score 1, d scores 1/sqrt(2), b scores 0.
func TestFindSimilarOrderAndLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache, stop := warmCache(t, map[string][]float32{
			"o.p.s.d.t.c": {1, 0},
			"o.p.s.d.t.a": {2, 0},
			"o.p.s.d.t.b": {0, 1},
			"o.p.s.d.t.d": {1, 1},
			"o.p.s.d.t.e": {3, 0},
		})
		query := []float32{1, 0}
		for _, tc := range []struct {
			name    string
			exclude string
			limit   int
			want    []string
		}{
			{"all", "", 10, []string{"o.p.s.d.t.a", "o.p.s.d.t.c", "o.p.s.d.t.e", "o.p.s.d.t.d", "o.p.s.d.t.b"}},
			{"a tie across the limit", "", 2, []string{"o.p.s.d.t.a", "o.p.s.d.t.c"}},
			{"excluded", "o.p.s.d.t.a", 2, []string{"o.p.s.d.t.c", "o.p.s.d.t.e"}},
			{"limit 4", "", 4, []string{"o.p.s.d.t.a", "o.p.s.d.t.c", "o.p.s.d.t.e", "o.p.s.d.t.d"}},
			{"limit 0", "", 0, nil},
			{"negative limit", "", -1, nil},
		} {
			got, ok := cache.FindSimilar(tc.exclude, query, nil, tc.limit)
			if !ok {
				t.Fatalf("%s: FindSimilar reported the cache not authoritative", tc.name)
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
// the cache is a map, so Go's randomized iteration order permutes the input to the sort on
// every call. Over many calls the top-k of an all-equal-similarity corpus is the same, and is
// the k lexicographically smallest IDs.
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
			got, ok := cache.FindSimilar("", []float32{1, 1, 0}, nil, k)
			if !ok {
				t.Fatal("cache reported cold; expected warm")
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
		got, ok := cache.FindSimilar("", query, keep, 10)
		if !ok {
			t.Fatal("cache reported cold; expected warm")
		}
		want := []string{"acme.web.docs.site.doc.guide", "acme.web.docs.site.doc.readme"}
		if !slices.Equal(ids(got), want) {
			t.Fatalf("scoped FindSimilar = %v; want %v", ids(got), want)
		}
		if len(scored) != 4 {
			t.Fatalf("keep was asked about %d entities; want all 4", len(scored))
		}
		if got, ok := cache.FindSimilar("", query, nil, 10); !ok || len(got) != 4 {
			t.Fatalf("unscoped FindSimilar = %v, %v; want 4 entities", ids(got), ok)
		}
		if err := stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v; want context.Canceled", err)
		}
	})
}

// TestVectorCacheInvalidationLinearizesWithActiveRead (pin: storage_cache_health_test.go): a
// FindSimilar that began while the cache was authoritative answers true even when Run's
// context ends during it, because Run cannot withdraw the cache's authority until the read has
// finished; once Run has returned, FindSimilar answers false. It runs outside a bubble because
// a goroutine waiting on a mutex is not durably blocked, so the wait for Run to reach the
// withdrawal reads the goroutine stacks instead.
func TestVectorCacheInvalidationLinearizesWithActiveRead(t *testing.T) {
	kv := &replayKV{memKV: newMemKV()}
	putRecord(t, kv, &Record{EntityID: "o.p.s.d.t.a", Vector: []float32{1, 0}, Status: StatusGenerated})
	cache, stop := runCache(t, kv)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	if _, err := probe.Await(ctx, func(context.Context) (bool, error) {
		_, ok := cache.FindSimilar("", []float32{1, 0}, nil, 1)
		return ok, nil
	}, func(ok bool) bool { return ok }); err != nil {
		t.Fatalf("the cache never became authoritative: %v", err)
	}

	entered, release := make(chan struct{}), make(chan struct{})
	read := make(chan bool, 1)
	go func() {
		_, ok := cache.FindSimilar("", []float32{1, 0}, func(string) bool {
			close(entered)
			<-release
			return true
		}, 10)
		read <- ok
	}()
	<-entered // FindSimilar is inside its read.

	stopped := make(chan error, 1)
	go func() { stopped <- stop() }()
	// The stack dump showing Run waiting for the cache's lock orders the release after Run has
	// tried to withdraw the cache's authority.
	if _, err := probe.Await(ctx, func(context.Context) (bool, error) {
		return runWaitsForCacheLock(), nil
	}, func(waiting bool) bool { return waiting }); err != nil {
		t.Fatalf("Run never waited for the active read: %v", err)
	}
	close(release)
	if ok := <-read; !ok {
		t.Fatal("a read that began while the cache was authoritative answered false")
	}
	if err := <-stopped; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v; want context.Canceled", err)
	}
	if _, ok := cache.FindSimilar("", []float32{1, 0}, nil, 10); ok {
		t.Fatal("the cache answered after Run returned")
	}
}

// runWaitsForCacheLock reports whether a goroutine in VectorCache.Run is waiting to take the
// cache's lock for writing.
func runWaitsForCacheLock() bool {
	buf := make([]byte, 1<<20)
	for _, stack := range strings.Split(string(buf[:runtime.Stack(buf, true)]), "\n\n") {
		if strings.Contains(stack, "[sync.RWMutex.Lock]") && strings.Contains(stack, "embedding.(*VectorCache).Run") {
			return true
		}
	}
	return false
}
