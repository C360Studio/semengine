//go:build integration

package graphindex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/vocabulary"
)

// TestProbeReconcileCost measures a warm restart over a seeded store: how long the second
// graph-index takes to become ready, how many consumers the server creates meanwhile, and,
// once it is ready, the cost of each step of one entity's reconcile over a sample of entities.
// PROBE_765_SIZES lists the store sizes (comma separated); unset, it runs one small size so the
// probe stays a quick check. PROBE_765_WARM_WORKERS sets the second start's lane count.
func TestProbeReconcileCost(t *testing.T) {
	sizes := []int{200}
	if raw := os.Getenv("PROBE_765_SIZES"); raw != "" {
		sizes = nil
		for _, s := range strings.Split(raw, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil {
				t.Fatalf("PROBE_765_SIZES: %v", err)
			}
			sizes = append(sizes, n)
		}
	}
	warmWorkers := 1
	if raw := os.Getenv("PROBE_765_WARM_WORKERS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("PROBE_765_WARM_WORKERS: %v", err)
		}
		warmWorkers = n
	}
	for _, n := range sizes {
		t.Run(strconv.Itoa(n), func(t *testing.T) { probeReconcileCost(t, n, warmWorkers) })
	}
}

func probeEntityID(n int) string { return fmt.Sprintf("acme.ops.probe.system.widget.%06d", n) }

// probeEntity is one realistic stored entity: a name, an alias, two relationships to other
// entities, and three plain properties, so every index gets rows.
func probeEntity(t *testing.T, i, total int) []byte {
	id := probeEntityID(i)
	return entityValue(t, id,
		stmt{vocabulary.DCTermsTitle, fmt.Sprintf("Widget %d", i)},
		stmt{vocabulary.CoreIdentityAlias, fmt.Sprintf("w-%06d", i)},
		stmt{"test.link.to", probeEntityID((i + 1) % total)},
		stmt{"test.link.near", probeEntityID((i * 7) % total)},
		stmt{"test.state.armed", i%2 == 0},
		stmt{"test.state.level", i % 10},
		stmt{"test.state.zone", fmt.Sprintf("zone-%d", i%25)},
	)
}

// advisoryCounter counts consumer-create advisories the server publishes in this account.
type advisoryCounter struct {
	n atomic.Int64
}

func probeReconcileCost(t *testing.T, total, warmWorkers int) {
	url := shared.Reset(t)
	client := natsfixture.Open(t, url, openClient)
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("advisory connection: %v", err)
	}
	t.Cleanup(nc.Close)
	var created advisoryCounter
	sub, err := nc.Subscribe("$JS.EVENT.ADVISORY.CONSUMER.CREATED.>", func(*nats.Msg) { created.n.Add(1) })
	if err != nil {
		t.Fatalf("subscribe advisories: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	// flushed returns the advisory count once every advisory published before the call has been
	// delivered: the server sends in order on this connection, so a flush orders the read.
	flushed := func() int64 {
		if err := nc.Flush(); err != nil {
			t.Fatalf("flush: %v", err)
		}
		return created.n.Load()
	}
	apiTotal := func() uint64 {
		info, err := js.AccountInfo(t.Context())
		if err != nil {
			t.Fatalf("account info: %v", err)
		}
		return info.API.Total
	}

	// Seed.
	states := ensureEntityStates(t, client)
	seedStart := time.Now()
	var wg sync.WaitGroup
	var seedErr atomic.Value
	const seeders = 16
	for w := range seeders {
		wg.Go(func() {
			for i := w; i < total; i += seeders {
				if _, err := states.Put(t.Context(), probeEntityID(i), probeEntity(t, i, total)); err != nil {
					seedErr.Store(err)
					return
				}
			}
		})
	}
	wg.Wait()
	if err, _ := seedErr.Load().(error); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Logf("seeded %d entities in %s", total, time.Since(seedStart).Round(time.Millisecond))

	// Cold build, with all 16 lanes to save time; its speed is not the question.
	cold, _ := newIndex(t, client, `{"workers": 16}`)
	coldElapsed, coldConsumers, _ := timedStartToReady(t, cold, total, 0, flushed)
	t.Logf("cold build: %d entities ready in %s (%.1f/s), consumer-create advisories %d (%.2f per entity)",
		total, coldElapsed.Round(time.Millisecond), float64(total)/coldElapsed.Seconds(),
		coldConsumers, float64(coldConsumers)/float64(total))
	rowCounts(t, js)

	// Per-entity breakdown, one entity at a time like one lane, on the ready, idle component: every
	// row is stored, which is what a warm restart's reconcile finds.
	breakdown(t, cold, total, flushed)
	if n := cold.failedTotal(); n != 0 {
		t.Errorf("failed entities = %d, want 0", n)
	}
	stopComponent(t, cold)

	// Warm restart: a second graph-index over the same store and indexes. PROBE_765_WARM_WINDOW,
	// when set, ends the observation after that long and reports the speed reached.
	var window time.Duration
	if raw := os.Getenv("PROBE_765_WARM_WINDOW"); raw != "" {
		if window, err = time.ParseDuration(raw); err != nil {
			t.Fatalf("PROBE_765_WARM_WINDOW: %v", err)
		}
	}
	warm, _ := newIndex(t, client, fmt.Sprintf(`{"workers": %d}`, warmWorkers))
	apiBefore := apiTotal()
	warmElapsed, warmConsumers, reached := timedStartToReady(t, warm, total, window, flushed)
	apiDuring := apiTotal() - apiBefore
	what := "ready"
	if reached < uint64(total) {
		what = fmt.Sprintf("NOT ready, window ended at indexed revision %d", reached)
	}
	t.Logf("warm restart (workers %d): %d entities, %s after %s (%.1f/s, %.2f ms per entity), "+
		"consumer-create advisories %d (%.2f per indexed entity), JetStream API calls %d (%.2f per indexed entity)",
		warmWorkers, total, what, warmElapsed.Round(time.Millisecond), float64(reached)/warmElapsed.Seconds(),
		float64(warmElapsed.Microseconds())/1000/float64(reached),
		warmConsumers, float64(warmConsumers)/float64(reached), apiDuring, float64(apiDuring)/float64(reached))
}

// timedStartToReady starts c and returns the time until its follower answers ready and its floor
// has reached the store's last revision, the consumer-create advisories counted meanwhile, and the
// floor reached. A window above zero ends the wait after that long, ready or not.
func timedStartToReady(t *testing.T, c *Component, total int, window time.Duration,
	flushed func() int64) (time.Duration, int64, uint64) {
	t.Helper()
	before := flushed()
	start := time.Now()
	startComponent(t, c)
	f := c.follower.Load()
	done := make(chan struct{})
	var progress sync.WaitGroup
	progress.Go(func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				el := time.Since(start)
				idx := f.Indexed()
				t.Logf("  progress %s: indexed revision %d (%.1f/s overall)", el.Round(time.Second), idx,
					float64(idx)/el.Seconds())
			}
		}
	})
	ctx := t.Context()
	if window > 0 {
		// Own deadline: the window is the measurement, not work that must finish.
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, window)
		defer cancel()
	}
	_, err := probe.Await(ctx, func(ctx context.Context) (bool, error) {
		return f.QueryReady(ctx) == nil && f.Indexed() >= uint64(total), nil
	}, func(ok bool) bool { return ok })
	elapsed := time.Since(start)
	reached := f.Indexed()
	close(done)
	progress.Wait()
	if err != nil && (window == 0 || t.Context().Err() != nil) {
		t.Fatalf("await ready: %v", err)
	}
	return elapsed, flushed() - before, reached
}

// rowCounts logs each index bucket's row count.
func rowCounts(t *testing.T, js jetstream.JetStream) {
	t.Helper()
	for _, bucket := range []string{graph.BucketEntityStates, graph.BucketIncomingIndex, graph.BucketOutgoingIndex,
		graph.BucketPredicateIndex, graph.BucketAliasIndex, graph.BucketNameIndex} {
		stream, err := js.Stream(t.Context(), "KV_"+bucket)
		if err != nil {
			t.Fatalf("stream of %s: %v", bucket, err)
		}
		info, err := stream.Info(t.Context())
		if err != nil {
			t.Fatalf("info of %s: %v", bucket, err)
		}
		t.Logf("  %s: %d stored messages, %d subjects, %d bytes", bucket, info.State.Msgs,
			info.State.NumSubjects, info.State.Bytes)
	}
}

// stepTimes is one sampled entity's timings.
type stepTimes struct {
	get, plan, listIncoming, listPredicate, listAlias, listName, writes, sum, total time.Duration
}

func breakdown(t *testing.T, c *Component, total int, flushed func() int64) {
	t.Helper()
	ctx := t.Context()
	sample := min(total, 1000)
	stride := total / sample
	samples := make([]stepTimes, 0, sample)
	var piecesConsumers, totalConsumers int64
	for s := range sample {
		id := probeEntityID(s * stride)
		var st stepTimes
		before := flushed()

		t0 := time.Now()
		entry, err := c.entities.Get(ctx, id)
		st.get = time.Since(t0)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}

		t0 = time.Now()
		var state graph.EntityState
		if err := graph.UnmarshalEntityState(id, entry.Value(), &state); err != nil {
			t.Fatalf("unmarshal %s: %v", id, err)
		}
		plan, err := c.buildEntityIndexPlan(state, id)
		if err != nil {
			t.Fatalf("plan %s: %v", id, err)
		}
		st.plan = time.Since(t0)

		list := func(bucket interface {
			KeysByFilter(context.Context, string) ([]string, error)
		}, filter string, into *time.Duration) {
			t0 := time.Now()
			if _, err := bucket.KeysByFilter(ctx, filter); err != nil {
				t.Fatalf("list %q: %v", filter, err)
			}
			*into = time.Since(t0)
		}
		list(c.incomingBucket, incomingIndexSourceFilter(id), &st.listIncoming)
		list(c.predicateBucket, predicateIndexEntityFilter(id), &st.listPredicate)
		list(c.aliasBucket, aliasIndexEntityFilter(id), &st.listAlias)
		list(c.nameBucket, nameIndexEntityFilter(id), &st.listName)

		// The writes a warm reconcile makes: the outgoing list, and each name row, which is
		// overwritten even when stored. Incoming, predicate and alias rows already stored are kept.
		t0 = time.Now()
		if err := c.writeOutgoingIndex(ctx, id, plan.outgoing); err != nil {
			t.Fatalf("outgoing %s: %v", id, err)
		}
		for _, w := range plan.nameWrites {
			value, err := json.Marshal(nameCompositeValue{Name: w.name, Priority: w.priority})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.nameBucket.Put(ctx, nameCompositeKey(nameIndexKey(w.name), id, w.predicate), value); err != nil {
				t.Fatalf("name put %s: %v", id, err)
			}
		}
		st.writes = time.Since(t0)
		st.sum = st.get + st.plan + st.listIncoming + st.listPredicate + st.listAlias + st.listName + st.writes
		mid := flushed()
		piecesConsumers += mid - before

		// The real reconcile, as a repair pass runs it (revision 0 completes nothing).
		t0 = time.Now()
		c.reconcileEntity(ctx, indexWork{entityID: id})
		st.total = time.Since(t0)
		totalConsumers += flushed() - mid
		samples = append(samples, st)
	}

	pick := func(name string, f func(stepTimes) time.Duration) {
		ds := make([]time.Duration, len(samples))
		var sum time.Duration
		for i, s := range samples {
			ds[i] = f(s)
			sum += ds[i]
		}
		slices.Sort(ds)
		q := func(p float64) time.Duration { return ds[min(len(ds)-1, int(p*float64(len(ds))))] }
		t.Logf("  | %-16s | %9.3f | %9.3f | %9.3f | %9.3f |", name,
			ms(q(0.5)), ms(q(0.9)), ms(q(0.99)), ms(sum/time.Duration(len(ds))))
	}
	t.Logf("breakdown over %d entities (every %dth), ms:", len(samples), stride)
	t.Logf("  | %-16s | %9s | %9s | %9s | %9s |", "step", "median", "p90", "p99", "mean")
	pick("get", func(s stepTimes) time.Duration { return s.get })
	pick("decode+plan", func(s stepTimes) time.Duration { return s.plan })
	pick("list incoming", func(s stepTimes) time.Duration { return s.listIncoming })
	pick("list predicate", func(s stepTimes) time.Duration { return s.listPredicate })
	pick("list alias", func(s stepTimes) time.Duration { return s.listAlias })
	pick("list name", func(s stepTimes) time.Duration { return s.listName })
	pick("four listings", func(s stepTimes) time.Duration {
		return s.listIncoming + s.listPredicate + s.listAlias + s.listName
	})
	pick("writes", func(s stepTimes) time.Duration { return s.writes })
	pick("sum of steps", func(s stepTimes) time.Duration { return s.sum })
	pick("reconcileEntity", func(s stepTimes) time.Duration { return s.total })
	t.Logf("consumer-create advisories: %.2f per entity for the four timed listings, %.2f per reconcileEntity",
		float64(piecesConsumers)/float64(len(samples)), float64(totalConsumers)/float64(len(samples)))
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
