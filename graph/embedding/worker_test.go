package embedding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/storage"
)

// TestWorkerRunRefusesNilContext: Run(nil) returns an invalid-input error and opens no watch.
func TestWorkerRunRefusesNilContext(t *testing.T) {
	kv := newWatchKV()
	w := NewWorker(newTestStorage(t, kv, newMemKV()), fixedEmbedder(), discardLogger())
	if err := w.Run(nil); !errs.IsInvalid(err) {
		t.Fatalf("Run(nil) = %v; want an invalid-input error", err)
	}
	if n := kv.watches.Load(); n != 0 {
		t.Fatalf("Run(nil) opened %d watches; want 0", n)
	}
}

// TestWorkerSecondRunRefused: a second Run while one runs returns an error without opening a
// watch, and the first keeps running and processing until its context ends.
func TestWorkerSecondRunRefused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kv := newWatchKV()
		s := newTestStorage(t, kv, newMemKV())
		log := &terminalLog{}
		w := NewWorker(s, fixedEmbedder(), discardLogger()).WithOnTerminal(log.callback)
		stop := runWorker(t, w)
		synctest.Wait() // the first Run is reading its watch

		second, cancelSecond := context.WithCancel(t.Context())
		defer cancelSecond()
		if err := w.Run(second); err == nil {
			t.Fatal("a second Run returned nil; want a refusal")
		}
		if n := kv.watches.Load(); n != 1 {
			t.Fatalf("watches opened = %d; want 1 (the second Run opened one)", n)
		}

		const id = "acme.ops.robotics.gcs.drone.second"
		if err := pendingText(t.Context(), s, id, "", "still running", 1); err != nil {
			t.Fatal(err)
		}
		synctest.Wait() // the first Run has processed the record
		if got := log.all(); !slices.Equal(got, []terminalEvent{{id, 1, OutcomeGenerated, ""}}) {
			t.Fatalf("terminal callbacks = %v; want the first Run to generate %s", got, id)
		}
		stopWorker(t, stop)
	})
}

// TestWorkerRunLeavesNothingBehind: Run started, used and cancelled inside a bubble returns
// context.Canceled, and the bubble ends with no goroutine left (synctest.Test fails otherwise):
// no reader, no lane of the keyed pool.
func TestWorkerRunLeavesNothingBehind(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kv := newWatchKV()
		s := newTestStorage(t, kv, newMemKV())
		var generated atomic.Int32
		w := NewWorker(s, fixedEmbedder(), discardLogger()).
			WithOnGenerated(func(context.Context, string, []float32) { generated.Add(1) })
		stop := runWorker(t, w)

		const id = "acme.ops.robotics.gcs.drone.used"
		if err := pendingText(t.Context(), s, id, "", "embed me", 1); err != nil {
			t.Fatal(err)
		}
		synctest.Wait() // the record has been processed
		rec, err := s.GetEmbedding(t.Context(), id)
		if err != nil || rec == nil || rec.Status != StatusGenerated {
			t.Fatalf("record = %+v, %v; want generated", rec, err)
		}
		if n := generated.Load(); n != 1 {
			t.Fatalf("onGenerated fired %d times; want 1", n)
		}
		stopWorker(t, stop)
	})
}

// TestTerminalCallbackOncePerOutcome: records that reach generated, failed, no-text and
// deleted outcomes each fire the terminal callback exactly once, with the record's source
// revision, its outcome and, for the failure only, its bounded reason. The Worker's own writes
// come back through its watch and fire nothing.
func TestTerminalCallbackOncePerOutcome(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			idGen     = "acme.ops.robotics.gcs.drone.gen"
			idFail    = "acme.ops.robotics.gcs.drone.fail"
			idNoText  = "acme.ops.robotics.gcs.drone.notext"
			idDeleted = "acme.ops.robotics.gcs.drone.deleted"
		)
		ctx := t.Context()
		kv := newWatchKV()
		s := newTestStorage(t, kv, newMemKV())
		embedder := &stubEmbedder{model: "m", dimensions: 3, generate: func(texts []string) ([][]float32, error) {
			switch texts[0] {
			case "fail me":
				return nil, errors.New("embedder exploded")
			case "delete me":
				// hop 1's tombstone lands during the embedder call
				if err := s.DeleteEmbedding(ctx, idDeleted); err != nil {
					return nil, err
				}
			}
			return [][]float32{{1, 2, 3}}, nil
		}}
		log := &terminalLog{}
		w := NewWorker(s, embedder, discardLogger()).WithWorkers(2).WithOnTerminal(log.callback)
		stop := runWorker(t, w)

		for _, p := range []struct {
			id, text string
			rev      uint64
		}{{idGen, "generate me", 11}, {idFail, "fail me", 12}, {idNoText, "", 13}, {idDeleted, "delete me", 14}} {
			if err := pendingText(ctx, s, p.id, "", p.text, p.rev); err != nil {
				t.Fatal(err)
			}
		}
		synctest.Wait() // every record, and every write the Worker made, has been processed
		stopWorker(t, stop)

		got := log.all()
		slices.SortFunc(got, func(a, b terminalEvent) int { return strings.Compare(a.entityID, b.entityID) })
		want := []terminalEvent{
			{idDeleted, 14, OutcomeDeleted, ""},
			{idFail, 12, OutcomeFailed, failReasonEmbedderError},
			{idGen, 11, OutcomeGenerated, ""},
			{idNoText, 13, OutcomeSkipped, ""},
		}
		if !slices.Equal(got, want) {
			t.Fatalf("terminal callbacks =\n%v\nwant once each:\n%v", got, want)
		}
	})
}

// TestCASExhaustionIsNotTerminal (the pin's TestSaveAndNotify_CASExhaustedIsNotAFailure):
// when the generated save's compare-and-set retries run out (natsclient.ErrKVMaxRetriesExceeded,
// TestSaveExhaustionMatchesMaxRetriesExceeded), the record stays pending, no terminal or
// generated callback fires, and no failure is counted: marking it failed could later flip a
// good vector at the same revision. In a bubble, so the retry backoff runs on the fake clock.
func TestCASExhaustionIsNotTerminal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const id = "acme.ops.robotics.gcs.drone.cas"
		kv := newWatchKV()
		// One attempt and no retry, so no backoff sleeps and synctest.Wait orders the
		// assertions after the entry. Under the default budget, ten jittered backoffs can
		// also outlast the KVStore's 5 s timeout, and the save then ends on the deadline.
		budget := func(o *natsclient.KVOptions) { o.MaxRetries = 0 }
		s := NewStorage(newTestKVStore(t, kv, budget), newTestKVStore(t, newMemKV(), budget))
		log := &terminalLog{}
		var generated atomic.Int32
		m := &recordingWorkerMetrics{}
		w := NewWorker(s, fixedEmbedder(), discardLogger()).
			WithMetrics(m).
			WithOnTerminal(log.callback).
			WithOnGenerated(func(context.Context, string, []float32) { generated.Add(1) })
		stop := runWorker(t, w)

		kv.failUpdates.Store(true) // the pending record is created; every later compare-and-set conflicts
		if err := pendingText(t.Context(), s, id, "", "text to embed", 5); err != nil {
			t.Fatal(err)
		}
		synctest.Wait() // the save has given up
		stopWorker(t, stop)

		if got := log.all(); len(got) != 0 {
			t.Fatalf("terminal callbacks = %v; want none for a save whose retries ran out", got)
		}
		if n := generated.Load(); n != 0 {
			t.Fatalf("onGenerated fired %d times; want 0", n)
		}
		if f := m.snapshot().failed; f != 0 {
			t.Fatalf("IncFailed = %d; want 0: retry exhaustion is not a generation failure", f)
		}
		rec, err := s.GetEmbedding(t.Context(), id)
		if err != nil || rec == nil || rec.Status != StatusPending || rec.SourceRevision != 5 {
			t.Fatalf("record = %+v, %v; want still pending at revision 5", rec, err)
		}
	})
}

// TestFailureWithCanceledTextIsAFailure (design D7, changed): a failure counts as the Worker
// shutting down only when its error matches context.Canceled and the Worker's context has
// ended. The pin searched the message for "context canceled", so a product embedder's error
// that only mentions it was dropped as a shutdown.
func TestFailureWithCanceledTextIsAFailure(t *testing.T) {
	t.Run("text only, through Run", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			const id = "acme.ops.robotics.gcs.drone.canceltext"
			kv := newWatchKV()
			s := newTestStorage(t, kv, newMemKV())
			embedder := &stubEmbedder{model: "m", dimensions: 3, generate: func([]string) ([][]float32, error) {
				return nil, errors.New("semembed: upstream request aborted: context canceled")
			}}
			log := &terminalLog{}
			m := &recordingWorkerMetrics{}
			w := NewWorker(s, embedder, discardLogger()).WithMetrics(m).WithOnTerminal(log.callback)
			stop := runWorker(t, w)
			if err := pendingText(t.Context(), s, id, "", "text", 3); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			stopWorker(t, stop)

			if got := log.all(); !slices.Equal(got, []terminalEvent{{id, 3, OutcomeFailed, failReasonEmbedderError}}) {
				t.Fatalf("terminal callbacks = %v; want one OutcomeFailed", got)
			}
			if f := m.snapshot().failed; f != 1 {
				t.Fatalf("IncFailed = %d; want 1", f)
			}
			rec, err := s.GetEmbedding(t.Context(), id)
			if err != nil || rec == nil || rec.Status != StatusFailed {
				t.Fatalf("record = %+v, %v; want failed", rec, err)
			}
		})
	})

	const id = "acme.ops.robotics.gcs.drone.cancelerr"
	cause := fmt.Errorf("generation failed: %w", context.Canceled)
	newCase := func(t *testing.T) (*Storage, *Worker, *countingMetrics) {
		s := newTestStorage(t, newMemKV(), newMemKV())
		if err := pendingText(t.Context(), s, id, "", "text", 1); err != nil {
			t.Fatal(err)
		}
		m := &countingMetrics{}
		return s, &Worker{storage: s, metrics: m, logger: discardLogger()}, m
	}

	t.Run("context.Canceled while the context is live", func(t *testing.T) {
		s, w, m := newCase(t)
		outcome, reason, terminal := w.markFailed(t.Context(), id, cause, failReasonEmbedderError, 1)
		if outcome != OutcomeFailed || reason != failReasonEmbedderError || !terminal || m.failed != 1 {
			t.Fatalf("markFailed = (%v, %q, %v), failed = %d; want a counted failure", outcome, reason, terminal, m.failed)
		}
		if rec, err := s.GetEmbedding(t.Context(), id); err != nil || rec.Status != StatusFailed {
			t.Fatalf("record = %+v, %v; want failed", rec, err)
		}
	})

	t.Run("context.Canceled after the context ended", func(t *testing.T) {
		s, w, m := newCase(t)
		ended, cancel := context.WithCancel(t.Context())
		cancel()
		outcome, reason, terminal := w.markFailed(ended, id, cause, failReasonEmbedderError, 1)
		if outcome != OutcomeSkipped || reason != "" || !terminal || m.failed != 0 {
			t.Fatalf("markFailed = (%v, %q, %v), failed = %d; want an uncounted shutdown", outcome, reason, terminal, m.failed)
		}
		if rec, err := s.GetEmbedding(t.Context(), id); err != nil || rec.Status != StatusPending {
			t.Fatalf("record = %+v, %v; want still pending", rec, err)
		}
	})
}

// TestBothLanesEmbedIdenticalBytes (design D7, changed): with the cap unset, the same over-cap
// text embeds identical bytes inline and by reference, and both records carry the same dedup
// key. 2,000 repeats of "abcd " are 10,000 runes; the default cap of 8,000 cuts after the
// 1,600th word and the word-boundary rule drops the trailing space. At the pin the inline lane
// read a cap of 0 as unlimited and embedded all 10,000 runes.
func TestBothLanesEmbedIdenticalBytes(t *testing.T) {
	text := strings.Repeat("abcd ", 2000)
	want := strings.TrimSuffix(strings.Repeat("abcd ", 1600), " ")
	const id = "acme.ops.robotics.gcs.drone.lane"

	// run runs one record through its own Worker and storage, so neither lane can be served
	// from the other's dedup record, and returns the texts the embedder received and the
	// stored record's dedup key.
	run := func(t *testing.T, seed func(context.Context, *Storage) error, resolver StoreResolver) ([]string, string) {
		var texts []string
		var hash string
		synctest.Test(t, func(t *testing.T) {
			kv := newWatchKV()
			s := newTestStorage(t, kv, newMemKV())
			var mu sync.Mutex
			embedder := &stubEmbedder{model: "m", dimensions: 3, generate: func(in []string) ([][]float32, error) {
				mu.Lock()
				texts = append(texts, in...)
				mu.Unlock()
				return [][]float32{{1, 2, 3}}, nil
			}}
			w := NewWorker(s, embedder, discardLogger()).WithStoreResolver(resolver)
			stop := runWorker(t, w)
			if err := seed(t.Context(), s); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			stopWorker(t, stop)
			rec, err := s.GetEmbedding(t.Context(), id)
			if err != nil || rec == nil || rec.Status != StatusGenerated {
				t.Fatalf("record = %+v, %v; want generated", rec, err)
			}
			hash = rec.ContentHash
		})
		return texts, hash
	}
	inlineTexts, inlineHash := run(t, func(ctx context.Context, s *Storage) error {
		return pendingText(ctx, s, id, "", text, 1)
	}, nil)
	ref := &StorageRef{StorageInstance: "objectstore", Key: "k"}
	resolver := fakeResolver{stores: map[string]storage.StreamableStore{"objectstore": readerStore{data: text}}}
	refTexts, refHash := run(t, func(ctx context.Context, s *Storage) error {
		return pendingRef(ctx, s, id, "", "", ref, 1)
	}, resolver)

	if !slices.Equal(inlineTexts, []string{want}) {
		t.Errorf("inline lane embedded %d texts, the first of %d runes; want one of 7,999 runes", len(inlineTexts), runesOf(inlineTexts))
	}
	if !slices.Equal(refTexts, []string{want}) {
		t.Errorf("offloaded lane embedded %d texts, the first of %d runes; want one of 7,999 runes", len(refTexts), runesOf(refTexts))
	}
	if inlineHash == "" || inlineHash != refHash {
		t.Errorf("dedup keys: inline %q, offloaded %q; want the same non-empty key", inlineHash, refHash)
	}
}

// runesOf is the rune count of the first text, or 0.
func runesOf(texts []string) int {
	if len(texts) == 0 {
		return 0
	}
	return len([]rune(texts[0]))
}

// TestKeyedCallbacksFollowRevisionOrder (design D6, #107; Codex's finding 2 on PR #458): N's
// failure is stored and its terminal callback is held until pending M is in the bucket. The
// callbacks complete as Failed(N) then Generated(M), and M's processing starts only after N's
// callback returned. Two lanes, so in the pin's shared-channel shape a second worker is free to
// take M while N's callback is held (notes/3.4.md shows this test failing on that shape).
func TestKeyedCallbacksFollowRevisionOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const id = "acme.ops.robotics.gcs.drone.order"
		ctx := t.Context()
		kv := newWatchKV()
		s := newTestStorage(t, kv, newMemKV())
		var nReturned, mBeforeN atomic.Bool
		embedder := &stubEmbedder{model: "m", dimensions: 3, generate: func(texts []string) ([][]float32, error) {
			if texts[0] == "text N" {
				return nil, errors.New("embedder down")
			}
			if !nReturned.Load() {
				mBeforeN.Store(true)
			}
			return [][]float32{{1, 2, 3}}, nil
		}}
		var mu sync.Mutex
		var completed []terminalEvent // in the order the callbacks returned
		entered, release := make(chan struct{}), make(chan struct{})
		w := NewWorker(s, embedder, discardLogger()).WithWorkers(2).
			WithOnTerminal(func(_ context.Context, entityID string, rev uint64, outcome TerminalOutcome, reason string) {
				if rev == 1 {
					close(entered)
					<-release
					nReturned.Store(true)
				}
				mu.Lock()
				completed = append(completed, terminalEvent{entityID, rev, outcome, reason})
				mu.Unlock()
			})
		stop := runWorker(t, w)

		if err := pendingText(ctx, s, id, "", "text N", 1); err != nil {
			t.Fatal(err)
		}
		<-entered // N's failure is stored and its callback is held
		if rec, err := s.GetEmbedding(ctx, id); err != nil || rec.Status != StatusFailed || rec.SourceRevision != 1 {
			t.Fatalf("record = %+v, %v; want failed at 1 while N's callback is held", rec, err)
		}
		if err := pendingText(ctx, s, id, "", "text M", 2); err != nil {
			t.Fatal(err)
		}
		if rec, err := s.GetEmbedding(ctx, id); err != nil || rec.Status != StatusPending || rec.SourceRevision != 2 {
			t.Fatalf("record = %+v, %v; want pending at 2", rec, err)
		}
		synctest.Wait() // all that can run while N's callback is held has run
		close(release)
		synctest.Wait()
		stopWorker(t, stop)

		want := []terminalEvent{{id, 1, OutcomeFailed, failReasonEmbedderError}, {id, 2, OutcomeGenerated, ""}}
		if !slices.Equal(completed, want) {
			t.Fatalf("terminal callbacks completed as %v; want %v", completed, want)
		}
		if mBeforeN.Load() {
			t.Fatal("M's embedding started before N's terminal callback returned")
		}
	})
}

// TestWorkerSaveRefusedWhenNewerPendingArrives (design D6; the revision guard against hop 1):
// N's embedding is held until hop 1 has written pending M and it is in the bucket. N's save is
// refused, onGenerated does not fire for it, and its outcome is OutcomeDeleted; M is then
// processed and the record ends generated at M with M's vector.
func TestWorkerSaveRefusedWhenNewerPendingArrives(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const id = "acme.ops.robotics.gcs.drone.refused"
		ctx := t.Context()
		kv := newWatchKV()
		s := newTestStorage(t, kv, newMemKV())
		nEntered, releaseN := make(chan struct{}), make(chan struct{})
		embedder := &stubEmbedder{model: "m", dimensions: 3, generate: func(texts []string) ([][]float32, error) {
			if texts[0] == "text N" {
				close(nEntered)
				<-releaseN
				return [][]float32{{1, 0, 0}}, nil
			}
			return [][]float32{{0, 1, 0}}, nil
		}}
		log := &terminalLog{}
		var mu sync.Mutex
		var generated [][]float32
		w := NewWorker(s, embedder, discardLogger()).WithWorkers(2).
			WithOnTerminal(log.callback).
			WithOnGenerated(func(_ context.Context, _ string, v []float32) {
				mu.Lock()
				generated = append(generated, v)
				mu.Unlock()
			})
		stop := runWorker(t, w)

		if err := pendingText(ctx, s, id, "", "text N", 1); err != nil {
			t.Fatal(err)
		}
		<-nEntered
		if err := pendingText(ctx, s, id, "", "text M", 2); err != nil {
			t.Fatal(err)
		}
		if rec, err := s.GetEmbedding(ctx, id); err != nil || rec.Status != StatusPending || rec.SourceRevision != 2 {
			t.Fatalf("record = %+v, %v; want pending at 2", rec, err)
		}
		close(releaseN)
		synctest.Wait()
		stopWorker(t, stop)

		want := []terminalEvent{{id, 1, OutcomeDeleted, ""}, {id, 2, OutcomeGenerated, ""}}
		if got := log.all(); !slices.Equal(got, want) {
			t.Fatalf("terminal callbacks = %v; want %v", got, want)
		}
		if len(generated) != 1 || !slices.Equal(generated[0], []float32{0, 1, 0}) {
			t.Fatalf("onGenerated vectors = %v; want only M's [0 1 0]", generated)
		}
		rec, err := s.GetEmbedding(ctx, id)
		if err != nil || rec.Status != StatusGenerated || rec.SourceRevision != 2 || !slices.Equal(rec.Vector, []float32{0, 1, 0}) {
			t.Fatalf("record = %+v, %v; want generated at 2 with M's vector", rec, err)
		}
	})
}

// TestNoTextDeleteFailureIsNotTerminal (design D3, changed): when the fenced removal of a
// record with no text fails with an error that is neither a moved key nor a missing one, no
// terminal callback fires, the record stays pending, and the failure is logged. The pin
// ignored the delete's error and reported the record skipped.
func TestNoTextDeleteFailureIsNotTerminal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const id = "acme.ops.robotics.gcs.drone.notextfail"
		kv := newWatchKV()
		kv.deleteErr = errors.New("transport failure")
		s := newTestStorage(t, kv, newMemKV())
		log := &terminalLog{}
		logs := &logRecorder{min: slog.LevelError}
		w := NewWorker(s, fixedEmbedder(), slog.New(logs)).WithOnTerminal(log.callback)
		stop := runWorker(t, w)
		if err := pendingText(t.Context(), s, id, "", "", 1); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		stopWorker(t, stop)

		if got := log.all(); len(got) != 0 {
			t.Fatalf("terminal callbacks = %v; want none", got)
		}
		rec, err := s.GetEmbedding(t.Context(), id)
		if err != nil || rec == nil || rec.Status != StatusPending || rec.SourceRevision != 1 {
			t.Fatalf("record = %+v, %v; want still pending at 1", rec, err)
		}
		if msgs := logs.messages(); len(msgs) != 1 || !strings.Contains(msgs[0], "no text") {
			t.Fatalf("error logs = %q; want one about the record with no text", msgs)
		}
	})
}
