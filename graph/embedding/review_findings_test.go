package embedding

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// seedFailedRecord writes a durable StatusFailed record for entityID at sourceRevision,
// via the real pending -> failed transition (SaveFailed is an UPDATE lane).
func seedFailedRecord(t *testing.T, s *Storage, entityID, sourceText, reason string, sourceRevision uint64) {
	t.Helper()
	ctx := t.Context()
	if err := pendingText(ctx, s, entityID, "", sourceText, sourceRevision); err != nil {
		t.Fatalf("seed SavePending: %v", err)
	}
	if err := s.SaveFailed(ctx, entityID, "seed failure", reason, sourceRevision); err != nil {
		t.Fatalf("seed SaveFailed: %v", err)
	}
}

// TestHandleKVEntry_SaveFailedPersistenceMissIsNonTerminal is the #613 F2 worker-side
// statement: when generation fails AND the failure cannot be durably recorded (SaveFailed's
// revision CAS never converges — a persistence MISS), handleKVEntry must return
// terminal==false so the readiness watermark does NOT advance over a record that is still
// durably pending. Recovery rides re-delivery. This mirrors saveAndNotify's retry-exhaustion
// handling; the deadlock-avoidance property is preserved for failures that DID durably record.
//
// Fails without the fix because markFailed's non-superseded/non-gone error path still
// reported a terminal OutcomeSkipped, advancing the watermark over the pending record.
func TestHandleKVEntry_SaveFailedPersistenceMissIsNonTerminal(t *testing.T) {
	t.Parallel()
	// In a bubble, so the KVStore's retry backoff runs on the fake clock.
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()

		// conflictKV (storage_test.go) makes every Update conflict, so SaveFailed exhausts its
		// retries (natsclient.ErrKVMaxRetriesExceeded): a persistence miss.
		index := &conflictKV{memKV: newMemKV()}
		s := newTestStorage(t, index, newMemKV())

		const entityID = "acme.ops.robotics.gcs.drone.persistmiss"
		const rev = uint64(5)
		if err := pendingText(ctx, s, entityID, "", "text to embed", rev); err != nil {
			t.Fatalf("SavePending: %v", err)
		}

		// The embedder fails generation, forcing markFailed → SaveFailed → CAS exhausted.
		embedder := &stubEmbedder{model: "m", dimensions: 3, generate: func([]string) ([][]float32, error) {
			return nil, errors.New("dial tcp: connect: connection refused")
		}}
		w := &Worker{storage: s, embedder: embedder, logger: discardLogger()}

		entry, err := index.Get(ctx, entityID)
		if err != nil {
			t.Fatalf("Get seed: %v", err)
		}
		_, srcRev, terminal, _, _ := w.handleKVEntry(ctx, entry, false /*live phase*/, 0)

		require.Equal(t, rev, srcRev)
		require.False(t, terminal,
			"a SaveFailed persistence miss must be non-terminal so the watermark does not advance and re-delivery retries (#613 F2)")

		rec, err := s.GetEmbedding(ctx, entityID)
		require.NoError(t, err)
		require.NotNil(t, rec)
		require.Equal(t, StatusPending, rec.Status,
			"nothing durable transitioned — the record must remain pending for re-drive")
	})
}

// TestHandleKVEntry_FailedRecordReprocessGatedByPhase is the #613 F3 statement that the
// snapshot-vs-live phase, captured at dequeue and passed as an immutable parameter, gates
// FAILED-record reprocessing: a failed record classified snapshot IS re-embedded, one
// classified live is SKIPPED (the hot-loop guard). The two are asserted deterministically
// against handleKVEntry directly, independent of the goroutine scheduling.
//
// Fails without the fix because handleKVEntry re-read the shared snapshotComplete flag
// (defaulting false in a bare Worker), so the live case would reprocess too.
func TestHandleKVEntry_FailedRecordReprocessGatedByPhase(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	const entityID = "acme.ops.robotics.gcs.drone.phase"
	const rev = uint64(5)

	newFailedWorker := func() (*Storage, jetstream.KeyValueEntry, *genCounter) {
		index := newMemKV()
		s := newTestStorage(t, index, newMemKV())
		seedFailedRecord(t, s, entityID, "recover me", failReasonConnectionRefused, rev)
		entry, err := index.Get(ctx, entityID)
		if err != nil {
			t.Fatalf("Get seed: %v", err)
		}
		return s, entry, &genCounter{}
	}

	t.Run("snapshot phase reprocesses the failed record", func(t *testing.T) {
		s, entry, gen := newFailedWorker()
		w := &Worker{storage: s, embedder: &stubEmbedder{model: "m", dimensions: 3, generate: gen.generate}, logger: discardLogger()}

		_, _, terminal, outcome, _ := w.handleKVEntry(ctx, entry, true /*snapshot*/, 0)
		require.True(t, terminal)
		require.Equal(t, OutcomeGenerated, outcome)
		require.Equal(t, 1, gen.count(), "a failed record delivered in the restart snapshot must be re-embedded (#613 F3/D4)")

		rec, err := s.GetEmbedding(ctx, entityID)
		require.NoError(t, err)
		require.Equal(t, StatusGenerated, rec.Status)
	})

	t.Run("live phase skips the failed record", func(t *testing.T) {
		s, entry, gen := newFailedWorker()
		w := &Worker{storage: s, embedder: &stubEmbedder{model: "m", dimensions: 3, generate: gen.generate}, logger: discardLogger()}

		_, _, terminal, _, _ := w.handleKVEntry(ctx, entry, false /*live*/, 0)
		require.False(t, terminal, "a live failed record must not be reprocessed (hot-loop guard)")
		require.Equal(t, 0, gen.count(), "a live failed record must not be re-embedded")

		rec, err := s.GetEmbedding(ctx, entityID)
		require.NoError(t, err)
		require.Equal(t, StatusFailed, rec.Status,
			"still failed; recovery rides the hop-1 re-SavePending backstop, not a live re-embed loop")
	})
}

// TestFailedRecordRecoveredByHop1RepublishBackstop proves the #613 F3 backstop: even if a
// failed snapshot record were misclassified as live and skipped (the residual sub-instruction
// window between the channel receive and the phase load), it is NOT stranded — hop-1
// re-delivers the entity from ENTITY_STATES on restart and re-writes a fresh StatusPending
// record, which is ALWAYS processed regardless of phase and regenerates it. This is why the
// phase gate can fail safe toward not-reprocessing without losing recovery.
func TestFailedRecordRecoveredByHop1RepublishBackstop(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	const entityID = "acme.ops.robotics.gcs.drone.backstop"
	index := newMemKV()
	s := newTestStorage(t, index, newMemKV())
	seedFailedRecord(t, s, entityID, "text", failReasonConnectionRefused, 5)

	gen := &genCounter{}
	w := &Worker{storage: s, embedder: &stubEmbedder{model: "m", dimensions: 3, generate: gen.generate}, logger: discardLogger()}

	// Live phase: the failed record is skipped (as if the race had misclassified it).
	failedEntry, err := index.Get(ctx, entityID)
	require.NoError(t, err)
	_, _, terminal, _, _ := w.handleKVEntry(ctx, failedEntry, false, 0)
	require.False(t, terminal)
	require.Equal(t, 0, gen.count())

	// BACKSTOP: hop-1's re-SavePending writes a fresh pending record (a LIVE pending write).
	require.NoError(t, pendingText(ctx, s, entityID, "", "text", 6))
	pendingEntry, err := index.Get(ctx, entityID)
	require.NoError(t, err)
	_, _, terminal2, outcome2, _ := w.handleKVEntry(ctx, pendingEntry, false, 0)
	require.True(t, terminal2)
	require.Equal(t, OutcomeGenerated, outcome2)
	require.Equal(t, 1, gen.count(),
		"hop-1's re-SavePending recovers a failed record even when snapshot reprocessing was skipped (#613 F3 backstop)")

	rec, err := s.GetEmbedding(ctx, entityID)
	require.NoError(t, err)
	require.Equal(t, StatusGenerated, rec.Status)
}

// manualWatchKV is a memKV whose Watch delivers entries on a channel the test drives, so
// a >1-lane Run test can deliver a snapshot failed record and (only after it is observed
// reprocessed) the end-of-snapshot sentinel — deterministically, without depending on the
// goroutine interleaving.
type manualWatchKV struct {
	*memKV
	updates  chan jetstream.KeyValueEntry
	watchCtx chan context.Context
	ioCtx    chan context.Context
}

func (k *manualWatchKV) Watch(ctx context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	if k.watchCtx != nil {
		k.watchCtx <- ctx
	}
	return &memWatcher{updates: k.updates}, nil
}

func (k *manualWatchKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if k.ioCtx != nil {
		k.ioCtx <- ctx
	}
	return k.memKV.Get(ctx, key)
}

func (k *manualWatchKV) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	if k.ioCtx != nil {
		k.ioCtx <- ctx
	}
	return k.memKV.Update(ctx, key, value, revision)
}

type workerContextKey struct{}

// TestWorkerPassesExactRunContextThroughWatcherIOEmbedderAndCallbacks: the context Run is
// given reaches the embedder and both callbacks unchanged, and the bucket reads and writes
// under a descendant of it; the watch is opened under a context derived from it, which has
// ended when Run returns.
func TestWorkerPassesExactRunContextThroughWatcherIOEmbedderAndCallbacks(t *testing.T) {
	parent, cancel := context.WithCancel(context.WithValue(t.Context(), workerContextKey{}, "generation-1"))
	defer cancel()

	base := newMemKV()
	kv := &manualWatchKV{
		memKV:    base,
		updates:  make(chan jetstream.KeyValueEntry, 2),
		watchCtx: make(chan context.Context, 1),
	}
	storage := newTestStorage(t, kv, newMemKV())
	const entityID = "acme.ops.robotics.gcs.drone.context"
	require.NoError(t, pendingText(parent, storage, entityID, "", "context propagation", 1))
	entry, err := base.Get(parent, entityID)
	require.NoError(t, err)
	kv.ioCtx = make(chan context.Context, 4)

	embedCtx := make(chan context.Context, 1)
	generatedCtx := make(chan context.Context, 1)
	terminalCtx := make(chan context.Context, 1)
	w := NewWorker(storage, &contextCapturingEmbedder{ctxs: embedCtx}, discardLogger()).
		WithWorkers(1).
		WithOnGenerated(func(ctx context.Context, _ string, _ []float32) { generatedCtx <- ctx }).
		WithOnTerminal(func(ctx context.Context, _ string, _ uint64, _ TerminalOutcome, _ string) { terminalCtx <- ctx })

	done := make(chan error, 1)
	go func() { done <- w.Run(parent) }()
	watchCtx := <-kv.watchCtx
	kv.updates <- entry

	require.Same(t, parent, <-embedCtx)
	require.Same(t, parent, <-generatedCtx)
	require.Same(t, parent, <-terminalCtx)
	// natsclient.KVStore bounds each bucket call with a context derived from the one it is
	// given, so the bucket sees a descendant of Run's context, not the same value.
	for i := 0; i < 2; i++ {
		require.Equal(t, "generation-1", (<-kv.ioCtx).Value(workerContextKey{}))
	}
	require.Equal(t, "generation-1", watchCtx.Value(workerContextKey{}))

	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after its context ended")
	}
	require.ErrorIs(t, watchCtx.Err(), context.Canceled)
}

type contextCapturingEmbedder struct {
	ctxs chan context.Context
}

func (e *contextCapturingEmbedder) Generate(ctx context.Context, _ []string) ([][]float32, error) {
	e.ctxs <- ctx
	return [][]float32{{1, 2, 3}}, nil
}

func (e *contextCapturingEmbedder) GenerateQuery(ctx context.Context, texts []string) ([][]float32, error) {
	return e.Generate(ctx, texts)
}

func (*contextCapturingEmbedder) Model() string   { return "m" }
func (*contextCapturingEmbedder) Dimensions() int { return 3 }
func (*contextCapturingEmbedder) Close() error    { return nil }

// TestFailedSnapshotRecordReprocessed_MultiWorker is the finding's deterministic multi-worker
// (>1 worker) statement: a failed record delivered in the restart snapshot is reprocessed
// through the real Run -> Watch -> keyed-pool path. The sentinel is delivered only AFTER
// the reprocessing terminal is observed, so the phase captured at dequeue is unambiguously
// snapshot and the assertion cannot flake on scheduling.
func TestFailedSnapshotRecordReprocessed_MultiWorker(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	base := newMemKV()
	kv := &manualWatchKV{memKV: base, updates: make(chan jetstream.KeyValueEntry, 8)}
	s := newTestStorage(t, kv, newMemKV())

	const entityID = "acme.ops.robotics.gcs.drone.snapmulti"
	seedFailedRecord(t, s, entityID, "recover me", failReasonConnectionRefused, 5)
	failedEntry, err := base.Get(ctx, entityID)
	require.NoError(t, err)

	gen := &genCounter{}
	done := make(chan string, 8)
	w := NewWorker(s, &stubEmbedder{model: "m", dimensions: 3, generate: gen.generate}, discardLogger()).
		WithWorkers(3).
		WithOnTerminal(func(_ context.Context, id string, _ uint64, _ TerminalOutcome, _ string) { done <- id })

	stop := runWorker(t, w)

	// Deliver the failed record while still in the snapshot phase (no sentinel sent yet).
	kv.updates <- failedEntry
	waitForTerminal(t, done, entityID)
	// Complete the snapshot cleanly, then stop.
	kv.updates <- nil

	stopWorker(t, stop)

	rec, err := s.GetEmbedding(ctx, entityID)
	require.NoError(t, err)
	require.Equal(t, StatusGenerated, rec.Status,
		"a failed record in the restart snapshot must be reprocessed with >1 worker (#613 F3)")
	require.Equal(t, 1, gen.count())
}

// TestColdStartIdenticalContentCollapsesToOneGenerate is the #630 cold-start statement: when
// the embedder has not resolved its vector width (Dimensions()==0), DedupKey withholds a
// durable key, so generateShared receives "". Even then a burst of K workers holding
// byte-identical content must collapse to ONE paid Generate call via a stable in-process
// singleflight key derived from the content (width excluded).
//
// Fails without the fix because generateShared bypassed the singleflight for an empty key and
// called the embedder directly, issuing one paid call per worker at cold start.
func TestColdStartIdenticalContentCollapsesToOneGenerate(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		var genCount atomic.Int64
		var firstOnce sync.Once
		firstIn := make(chan struct{})
		release := make(chan struct{})

		// dimensions 0 == unresolved width == DedupKey withholds a durable key.
		embedder := &stubEmbedder{model: "m", dimensions: 0, generate: func([]string) ([][]float32, error) {
			genCount.Add(1)
			firstOnce.Do(func() { close(firstIn) })
			<-release
			return [][]float32{{1, 2, 3}}, nil
		}}
		w := &Worker{embedder: embedder, embedderType: "http", logger: discardLogger()}

		const K = 8
		start := make(chan struct{})
		results := make([][]float32, K)
		errsOut := make([]error, K)
		var wg sync.WaitGroup
		for i := 0; i < K; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				// EMPTY durable key — the cold-start condition.
				results[i], errsOut[i] = w.generateShared(ctx, "", "identical cold-start content")
			}(i)
		}
		close(start)

		select {
		case <-firstIn:
		case <-time.After(5 * time.Second):
			t.Fatal("no Generate call arrived")
		}
		// Every peer is blocked in the singleflight call or on release once the bubble is idle, so the
		// release comes after all of them joined.
		synctest.Wait()
		close(release)
		wg.Wait()

		if got := genCount.Load(); got != 1 {
			t.Fatalf("Generate calls = %d, want 1: cold-start identical content (unresolved width) must still collapse to one call via the in-process singleflight key (#630)", got)
		}
		for i := 0; i < K; i++ {
			require.NoError(t, errsOut[i])
			require.Equal(t, []float32{1, 2, 3}, results[i])
		}
	})
}
