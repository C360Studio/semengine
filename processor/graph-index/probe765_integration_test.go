//go:build integration

package graphindex

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"runtime"
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
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/vocabulary"
)

// TestProbeReconcileCost measures graph-index over a seeded store with its per-entity records:
// the cold build (no records, so every entity lists), a warm restart over the unchanged store, an
// equal re-offer of every entity, and live changes, each with the KV operations it made.
// PROBE_765_SIZES lists the store sizes (comma separated); unset, it runs one small size so the
// probe stays a quick check. PROBE_765_WORKERS sets the lane count of every phase (default 1).
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
	workers := 1
	if raw := os.Getenv("PROBE_765_WORKERS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("PROBE_765_WORKERS: %v", err)
		}
		workers = n
	}
	for _, n := range sizes {
		t.Run(strconv.Itoa(n), func(t *testing.T) { probeReconcileCost(t, n, workers) })
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

func probeReconcileCost(t *testing.T, total, workers int) {
	url := shared.Reset(t)
	client := natsfixture.Open(t, url, openClient)
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("advisory connection: %v", err)
	}
	t.Cleanup(nc.Close)
	t.Logf("nats-server %s", nc.ConnectedServerVersion())
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
	logLoad(t, "seed")
	seedElapsed := putAll(t, states, total, func(i int) []byte { return probeEntity(t, i, total) })
	t.Logf("seeded %d entities in %s", total, seedElapsed.Round(time.Millisecond))

	// Cold build: no records yet, so every entity lists its owned rows.
	logLoad(t, "cold build start")
	cold, _ := newIndex(t, client, fmt.Sprintf(`{"workers": %d}`, workers))
	apiBefore := apiTotal()
	coldElapsed, coldConsumers, _ := timedStartToReady(t, cold, uint64(total), flushed)
	coldAPI := apiTotal() - apiBefore
	logLoad(t, "cold build end")
	t.Logf("cold build (workers %d): %d entities ready in %s (%.1f/s, %.3f ms per entity), "+
		"consumer-create advisories %d (%.2f per entity), JetStream API calls %d (%.2f per entity)",
		workers, total, coldElapsed.Round(time.Millisecond), float64(total)/coldElapsed.Seconds(),
		perEntityMs(coldElapsed, total), coldConsumers, float64(coldConsumers)/float64(total),
		coldAPI, float64(coldAPI)/float64(total))
	logOps(t, "cold build", nil, kvOperations(t, cold), total)
	rowCounts(t, js)
	if n := cold.failedTotal(); n != 0 {
		t.Errorf("cold: failed entities = %d, want 0", n)
	}
	stopComponent(t, cold)

	// Warm restart over the unchanged store: every record is clean and equal to the plan.
	logLoad(t, "warm restart start")
	warm, _ := newIndex(t, client, fmt.Sprintf(`{"workers": %d}`, workers))
	sequences := indexSequences(t, client)
	apiBefore = apiTotal()
	warmElapsed, warmConsumers, _ := timedStartToReady(t, warm, uint64(total), flushed)
	warmAPI := apiTotal() - apiBefore
	logLoad(t, "warm restart end")
	t.Logf("warm restart (workers %d): %d entities ready in %s (%.1f/s, %.3f ms per entity), "+
		"consumer-create advisories %d, JetStream API calls %d (%.2f per entity)",
		workers, total, warmElapsed.Round(time.Millisecond), float64(total)/warmElapsed.Seconds(),
		perEntityMs(warmElapsed, total), warmConsumers, warmAPI, float64(warmAPI)/float64(total))
	warmOps := kvOperations(t, warm)
	logOps(t, "warm restart", nil, warmOps, total)
	logSequenceDelta(t, "warm restart", sequences, indexSequences(t, client))

	// Equal re-offer: every entity written again with equal statements, so each gets a new
	// revision and nothing to change.
	logLoad(t, "re-offer start")
	sequences = indexSequences(t, client)
	consumersBefore := flushed()
	apiBefore = apiTotal()
	f := warm.follower.Load()
	reofferStart := time.Now()
	putElapsed := putAll(t, states, total, func(i int) []byte { return probeEntity(t, i, total) })
	awaitIndexed(t, f, uint64(2*total))
	reofferElapsed := time.Since(reofferStart)
	reofferAPI := apiTotal() - apiBefore
	logLoad(t, "re-offer end")
	t.Logf("equal re-offer (workers %d): %d writes in %s, all applied %s after the first write "+
		"(%.1f/s, %.3f ms per entity), consumer-create advisories %d, JetStream API calls %d (%.2f per entity)",
		workers, total, putElapsed.Round(time.Millisecond), reofferElapsed.Round(time.Millisecond),
		float64(total)/reofferElapsed.Seconds(), perEntityMs(reofferElapsed, total),
		flushed()-consumersBefore, reofferAPI, float64(reofferAPI)/float64(total))
	reofferOps := kvOperations(t, warm)
	logOps(t, "equal re-offer", warmOps, reofferOps, total)
	logSequenceDelta(t, "equal re-offer", sequences, indexSequences(t, client))

	// Live changes, one at a time on the ready component: first the step breakdown, then the
	// same number of real writes to ENTITY_STATES, each awaited until applied.
	logLoad(t, "breakdown start")
	breakdown(t, warm, total, flushed)
	logLoad(t, "live changes start")
	liveChanges(t, warm, states, total, client, flushed)
	logLoad(t, "live changes end")
	if n := warm.failedTotal(); n != 0 {
		t.Errorf("warm: failed entities = %d, want 0", n)
	}
	t.Logf("after the run:")
	rowCounts(t, js)
}

// liveVariant is entity i changed: a new title and a new near target, so its NAME row moves and
// its OUTGOING list and one INCOMING row change. tag tells two variants apart.
func liveVariant(t *testing.T, i, total int, tag string, shift int) []byte {
	id := probeEntityID(i)
	return entityValue(t, id,
		stmt{vocabulary.DCTermsTitle, fmt.Sprintf("Widget %d %s", i, tag)},
		stmt{vocabulary.CoreIdentityAlias, fmt.Sprintf("w-%06d", i)},
		stmt{"test.link.to", probeEntityID((i + 1) % total)},
		stmt{"test.link.near", probeEntityID((i*7 + shift) % total)},
		stmt{"test.state.armed", i%2 == 0},
		stmt{"test.state.level", i % 10},
		stmt{"test.state.zone", fmt.Sprintf("zone-%d", i%25)},
	)
}

// putAll writes value(i) for every entity, 16 writers, and returns how long it took.
func putAll(t *testing.T, states jetstream.KeyValue, total int, value func(int) []byte) time.Duration {
	t.Helper()
	start := time.Now()
	var wg sync.WaitGroup
	var putErr atomic.Value
	const writers = 16
	for w := range writers {
		wg.Go(func() {
			for i := w; i < total; i += writers {
				if _, err := states.Put(t.Context(), probeEntityID(i), value(i)); err != nil {
					putErr.Store(err)
					return
				}
			}
		})
	}
	wg.Wait()
	if err, _ := putErr.Load().(error); err != nil {
		t.Fatalf("put: %v", err)
	}
	return time.Since(start)
}

// awaitIndexed returns once f's indexed floor reaches revision. It reads an atomic and yields
// between reads, with no poll interval, so a millisecond apply is timed to well under a
// millisecond; go test -timeout bounds it.
func awaitIndexed(t *testing.T, f *follower.Follower, revision uint64) {
	t.Helper()
	for f.Indexed() < revision {
		if t.Context().Err() != nil {
			t.Fatalf("await indexed %d: %v", revision, t.Context().Err())
		}
		runtime.Gosched()
	}
}

func perEntityMs(d time.Duration, n int) float64 {
	return float64(d.Microseconds()) / 1000 / float64(n)
}

// logLoad logs the host's load average.
func logLoad(t *testing.T, phase string) {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		t.Logf("load at %s: %v", phase, err)
		return
	}
	t.Logf("load at %s: %s", phase, strings.TrimSpace(string(out)))
}

// logOps logs the KV operations a phase made: after minus before (nil before counts from zero).
func logOps(t *testing.T, phase string, before, after map[string]map[string]float64, total int) {
	t.Helper()
	delta := func(op, bucket string) float64 {
		var b float64
		if before != nil {
			b = before[op][bucket]
		}
		return after[op][bucket] - b
	}
	var lists, rowPuts, rowDeletes float64
	for _, label := range indexRowLabels {
		lists += delta("list", label)
		rowPuts += delta("put", label)
		rowDeletes += delta("delete", label)
	}
	t.Logf("%s ops: listings %.0f (%.2f per entity), row puts %.0f, row deletes %.0f, record gets %.0f, "+
		"record puts %.0f, record deletes %.0f", phase, lists, lists/float64(total), rowPuts,
		rowDeletes, delta("get", "record"), delta("put", "record"), delta("delete", "record"))
}

// logSequenceDelta logs how far each index bucket's stream moved: every row put or delete appends
// one message, whatever the component counts.
func logSequenceDelta(t *testing.T, phase string, before, after map[string]uint64) {
	t.Helper()
	var sum uint64
	parts := make([]string, 0, len(after))
	for _, bucket := range slices.Sorted(maps.Keys(after)) {
		sum += after[bucket] - before[bucket]
		parts = append(parts, fmt.Sprintf("%s +%d", bucket, after[bucket]-before[bucket]))
	}
	t.Logf("%s index stream messages appended: %d (%s)", phase, sum, strings.Join(parts, ", "))
}

// timedStartToReady starts c and returns the time until its follower answers ready and its floor
// has reached revision, and the consumer-create advisories counted meanwhile.
func timedStartToReady(t *testing.T, c *Component, revision uint64, flushed func() int64) (time.Duration, int64, uint64) {
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
	awaitIndexed(t, f, revision)
	_, err := probe.Await(t.Context(), func(ctx context.Context) (bool, error) {
		return f.QueryReady(ctx) == nil, nil
	}, func(ok bool) bool { return ok })
	elapsed := time.Since(start)
	close(done)
	progress.Wait()
	if err != nil {
		t.Fatalf("await ready: %v", err)
	}
	return elapsed, flushed() - before, f.Indexed()
}

// rowCounts logs each index bucket's row count.
func rowCounts(t *testing.T, js jetstream.JetStream) {
	t.Helper()
	for _, bucket := range []string{graph.BucketEntityStates, graph.BucketIncomingIndex, graph.BucketOutgoingIndex,
		graph.BucketPredicateIndex, graph.BucketAliasIndex, graph.BucketNameIndex, graph.BucketGraphIndexRecords} {
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
	get, plan, recordGet, diff, pendingPut, rowWrites, cleanPut, sum, change, unchanged time.Duration
	rows                                                                                int
}

// breakdown times, one entity at a time like one lane, each step of a live change applied from
// the record, then the real reconcile. For each sampled entity the steps apply a changed value to
// the index by the calls runApply makes, in its order; ENTITY_STATES is not written, so the
// follower stays idle. The real reconcileEntity then reads the stored value and takes the index
// back to it: a change of the same size, from the record. A second reconcileEntity finds the record
// equal to the plan: the unchanged path a restart or a re-offer takes.
func breakdown(t *testing.T, c *Component, total int, flushed func() int64) {
	t.Helper()
	ctx := t.Context()
	sample := min(total, 1000)
	stride := total / sample
	samples := make([]stepTimes, 0, sample)
	var stepOps, changeOps, unchangedOps map[string]map[string]float64
	add := func(into *map[string]map[string]float64, before, after map[string]map[string]float64) {
		if *into == nil {
			*into = map[string]map[string]float64{}
		}
		for op, buckets := range after {
			if (*into)[op] == nil {
				(*into)[op] = map[string]float64{}
			}
			for bucket, n := range buckets {
				(*into)[op][bucket] += n - before[op][bucket]
			}
		}
	}
	var changeConsumers, unchangedConsumers int64
	for s := range sample {
		i := s * stride
		id := probeEntityID(i)
		var st stepTimes
		opsBefore := kvOperations(t, c)

		t0 := time.Now()
		if _, err := c.entities.Get(ctx, id); err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		st.get = time.Since(t0)

		changed := liveVariant(t, i, total, "A", 3)
		t0 = time.Now()
		var state graph.EntityState
		if err := graph.UnmarshalEntityState(id, changed, &state); err != nil {
			t.Fatalf("unmarshal %s: %v", id, err)
		}
		plan, err := c.buildEntityIndexPlan(state, id)
		if err != nil {
			t.Fatalf("plan %s: %v", id, err)
		}
		planned, err := c.plannedRows(plan)
		if err != nil {
			t.Fatalf("planned rows %s: %v", id, err)
		}
		st.plan = time.Since(t0)

		t0 = time.Now()
		entry, err := c.recordBucket.Get(ctx, id)
		if err != nil {
			t.Fatalf("record get %s: %v", id, err)
		}
		record, err := decodeRecord(entry.Value)
		st.recordGet = time.Since(t0)
		if err != nil || record.pending || !maps.Equal(record.generations, c.generations) {
			t.Fatalf("record of %s not clean and usable: pending %v, err %v", id, record.pending, err)
		}

		t0 = time.Now()
		if sameRows(record.rows, planned) {
			t.Fatalf("variant of %s plans the stored rows", id)
		}
		ops := c.changeOps(record.rows, planned)
		st.diff = time.Since(t0)

		var pendingRevision uint64
		for _, op := range ops {
			t0 = time.Now()
			switch op.kind {
			case opPending:
				pendingRevision, err = c.putRecord(ctx, id, op.value, entry.Revision)
				st.pendingPut += time.Since(t0)
			case opClean:
				_, err = c.putRecord(ctx, id, op.value, pendingRevision)
				st.cleanPut += time.Since(t0)
			case opPut, opDelete:
				err = c.writeRow(ctx, op)
				st.rowWrites += time.Since(t0)
				st.rows++
			default:
				t.Fatalf("unexpected op %d for %s", op.kind, id)
			}
			if err != nil {
				t.Fatalf("op %d for %s: %v", op.kind, id, err)
			}
		}
		st.sum = st.get + st.plan + st.recordGet + st.diff + st.pendingPut + st.rowWrites + st.cleanPut
		opsMid := kvOperations(t, c)
		add(&stepOps, opsBefore, opsMid)

		// The real reconcile, as a repair pass runs it (revision 0 completes nothing): the stored
		// value differs from the record, so it is a change.
		consumersBefore := flushed()
		t0 = time.Now()
		c.reconcileEntity(ctx, indexWork{entityID: id})
		st.change = time.Since(t0)
		changeConsumers += flushed() - consumersBefore
		opsAfterChange := kvOperations(t, c)
		add(&changeOps, opsMid, opsAfterChange)

		consumersBefore = flushed()
		t0 = time.Now()
		c.reconcileEntity(ctx, indexWork{entityID: id})
		st.unchanged = time.Since(t0)
		unchangedConsumers += flushed() - consumersBefore
		add(&unchangedOps, opsAfterChange, kvOperations(t, c))
		samples = append(samples, st)
	}

	t.Logf("breakdown of a live change over %d entities (every %dth), ms:", len(samples), stride)
	t.Logf("  | %-24s | %9s | %9s | %9s | %9s |", "step", "median", "p90", "p99", "mean")
	pick := func(name string, f func(stepTimes) time.Duration) {
		quantiles(t, name, samples, f)
	}
	pick("ENTITY_STATES get", func(s stepTimes) time.Duration { return s.get })
	pick("decode + plan", func(s stepTimes) time.Duration { return s.plan })
	pick("record get + decode", func(s stepTimes) time.Duration { return s.recordGet })
	pick("diff (changeOps)", func(s stepTimes) time.Duration { return s.diff })
	pick("pending record put", func(s stepTimes) time.Duration { return s.pendingPut })
	pick("row writes", func(s stepTimes) time.Duration { return s.rowWrites })
	pick("clean record put", func(s stepTimes) time.Duration { return s.cleanPut })
	pick("record puts (both)", func(s stepTimes) time.Duration { return s.pendingPut + s.cleanPut })
	pick("sum of steps", func(s stepTimes) time.Duration { return s.sum })
	pick("reconcileEntity, change", func(s stepTimes) time.Duration { return s.change })
	pick("reconcileEntity, same", func(s stepTimes) time.Duration { return s.unchanged })
	rows := 0
	for _, s := range samples {
		rows += s.rows
	}
	t.Logf("row writes per change: %.2f", float64(rows)/float64(len(samples)))
	logOps(t, "breakdown steps", nil, stepOps, len(samples))
	logOps(t, "breakdown reconcileEntity change", nil, changeOps, len(samples))
	logOps(t, "breakdown reconcileEntity same", nil, unchangedOps, len(samples))
	t.Logf("consumer-create advisories: %.2f per change reconcile, %.2f per unchanged reconcile",
		float64(changeConsumers)/float64(len(samples)), float64(unchangedConsumers)/float64(len(samples)))
}

// liveChanges writes a changed value for each sampled entity to ENTITY_STATES, one at a time, and
// times each from the put until the follower's indexed floor reaches its revision: watch delivery,
// the lane, the reconcile and the completion together.
func liveChanges(t *testing.T, c *Component, states jetstream.KeyValue, total int, client *natsclient.Client,
	flushed func() int64) {
	t.Helper()
	ctx := t.Context()
	f := c.follower.Load()
	sample := min(total, 1000)
	stride := total / sample
	samples := make([]stepTimes, 0, sample)
	opsBefore := kvOperations(t, c)
	sequences := indexSequences(t, client)
	consumersBefore := flushed()
	for s := range sample {
		i := s * stride
		value := liveVariant(t, i, total, "B", 5)
		t0 := time.Now()
		revision, err := states.Put(ctx, probeEntityID(i), value)
		if err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
		put := time.Since(t0)
		awaitIndexed(t, f, revision)
		samples = append(samples, stepTimes{get: put, change: time.Since(t0)})
	}
	t.Logf("live changes: %d, one at a time, each awaited until applied, ms:", len(samples))
	t.Logf("  | %-24s | %9s | %9s | %9s | %9s |", "step", "median", "p90", "p99", "mean")
	quantiles(t, "ENTITY_STATES put", samples, func(s stepTimes) time.Duration { return s.get })
	quantiles(t, "put to applied", samples, func(s stepTimes) time.Duration { return s.change })
	logOps(t, "live changes", opsBefore, kvOperations(t, c), len(samples))
	logSequenceDelta(t, "live changes", sequences, indexSequences(t, client))
	t.Logf("live changes consumer-create advisories: %d", flushed()-consumersBefore)
}

// quantiles logs one table row: the median, p90, p99 and mean of f over samples.
func quantiles(t *testing.T, name string, samples []stepTimes, f func(stepTimes) time.Duration) {
	t.Helper()
	ds := make([]time.Duration, len(samples))
	var sum time.Duration
	for i, s := range samples {
		ds[i] = f(s)
		sum += ds[i]
	}
	slices.Sort(ds)
	q := func(p float64) time.Duration { return ds[min(len(ds)-1, int(p*float64(len(ds))))] }
	t.Logf("  | %-24s | %9.3f | %9.3f | %9.3f | %9.3f |", name,
		ms(q(0.5)), ms(q(0.9)), ms(q(0.99)), ms(sum/time.Duration(len(ds))))
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
