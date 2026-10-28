//go:build integration

package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"pgregory.net/rapid"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/natsclient"
)

// shared is this package's NATS server. It starts at the first reset; TestMain removes it
// after the tests.
var shared natsfixture.Shared

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	flag.Parse()
	// Design D9 bounds TestRevisionGuardHistoryMatchesModel at 50 generated runs; an explicit
	// -rapid.checks on the command line still wins.
	checksSet := false
	flag.Visit(func(f *flag.Flag) { checksSet = checksSet || f.Name == "rapid.checks" })
	if !checksSet {
		if err := flag.Set("rapid.checks", "50"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	code := m.Run()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := shared.Stop(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

// clientTimeout is the request timeout of the fixture client, the value the pin's
// NewTestClient family used.
const clientTimeout = 15 * time.Second

// fixtureClient resets the package's shared server and returns a Client connected to it,
// closed on t.Cleanup before the reset's connection check runs.
func fixtureClient(t *testing.T) *natsclient.Client {
	t.Helper()
	return natsfixture.Open(t, shared.Reset(t), openClient)
}

func openClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(clientTimeout),
		natsclient.WithMaxReconnects(0),
		natsclient.WithHealthInterval(0),
	)
	if err != nil {
		return nil, nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	if err := client.WaitForConnection(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	return client, client.Close, nil
}

func createBucket(t testing.TB, client *natsclient.Client, name string) jetstream.KeyValue {
	t.Helper()
	kv, err := client.CreateKeyValueBucket(t.Context(), jetstream.KeyValueConfig{Bucket: name})
	if err != nil {
		t.Fatalf("create bucket %s: %v", name, err)
	}
	return kv
}

// modelRecord is the reference model's view of one entity's index record.
type modelRecord struct {
	present bool
	status  Status
	source  uint64 // the record's source revision
	kv      uint64 // the KV revision last observed for the key (kept after removal)
}

// The calls a history makes.
const (
	opPending = iota
	opGenerated
	opFailed
	opDelete
	opFenced
)

// historyOp is one call: op on entity (an index into historyEntities) at source revision
// (the saves) or, for the fenced removal, at the key's current KV revision or a stale one.
type historyOp struct {
	op       int
	entity   int
	revision uint64
	current  bool
}

var historyEntities = []string{"acme.ops.robotics.gcs.drone.h0", "acme.ops.robotics.gcs.drone.h1"}

// The outcomes the model distinguishes, one per rule of design D3 and each side of its
// boundaries. Each has a witness history in TestRevisionGuardHistoryMatchesModel.
const (
	outPendingSaved              = "pending saved"
	outPendingOverNewer          = "pending saved over a newer record"
	outPendingOverOlderGenerated = "pending saved over an older generated record"
	outPendingRefused            = "pending refused over a newer generated record"
	outPendingRefusedEqual       = "pending refused over generated at equal revision"
	outGeneratedSaved            = "generated saved"
	outGeneratedGone             = "generated refused: record gone"
	outGeneratedSuperseded       = "generated refused: superseded"
	outFailedSaved               = "failed saved"
	outFailedGone                = "failed refused: record gone"
	outFailedSuperseded          = "failed refused: superseded"
	outFailedRefusedEqual        = "failed refused over generated at equal revision"
	outDeleted                   = "deleted"
	outFencedRemoved             = "fenced removal at the current revision removes"
	outFencedStalePresent        = "fenced removal at a stale revision keeps the record"
	outFencedAbsentCurrent       = "fenced removal of a removed key at its last revision"
	outFencedNeverStored         = "fenced removal of a key never stored"
)

// history runs calls against one fresh EMBEDDING_INDEX bucket and the reference model.
type history struct {
	ctx   context.Context
	s     *Storage
	index jetstream.KeyValue
	model []modelRecord
}

var historyBuckets int

// newHistory creates a fresh index bucket (deleted on cleanup) for one history.
func newHistory(t *testing.T, client *natsclient.Client, dedup *natsclient.KVStore) (*history, func() error) {
	t.Helper()
	historyBuckets++
	name := fmt.Sprintf("EMB_HIST_IDX_%d", historyBuckets)
	index := createBucket(t, client, name)
	h := &history{
		ctx:   t.Context(),
		s:     NewStorage(client.NewKVStore(index), dedup),
		index: index,
		model: make([]modelRecord, len(historyEntities)),
	}
	return h, func() error { return client.DeleteKeyValueBucket(t.Context(), name) }
}

// apply makes one call, checks its result and the stored record against the model, and
// returns the outcome it reached, or an error naming the first disagreement.
func (h *history) apply(o historyOp) (string, error) {
	e, m := historyEntities[o.entity], &h.model[o.entity]
	r := o.revision
	wrote := false
	var outcome string
	switch o.op {
	case opPending:
		saved, err := h.s.SavePendingGuarded(h.ctx, &Record{EntityID: e, SourceText: "t", SourceRevision: r})
		if err != nil {
			return "", fmt.Errorf("SavePendingGuarded(%s, %d): %w", e, r, err)
		}
		want := !(m.present && m.status == StatusGenerated && m.source >= r)
		if saved != want {
			return "", fmt.Errorf("SavePendingGuarded(%s, %d) saved = %v, model says %v (stored %+v)", e, r, saved, want, *m)
		}
		switch {
		case want && m.present && m.status == StatusGenerated:
			outcome = outPendingOverOlderGenerated
		case want && m.present && m.source > r:
			outcome = outPendingOverNewer
		case want:
			outcome = outPendingSaved
		case m.source == r:
			outcome = outPendingRefusedEqual
		default:
			outcome = outPendingRefused
		}
		if want {
			m.present, m.status, m.source, wrote = true, StatusPending, r, true
		}
	case opGenerated, opFailed:
		var err error
		status := StatusGenerated
		if o.op == opGenerated {
			err = h.s.SaveGenerated(h.ctx, e, []float32{float32(r)}, "m", 1, "h", r)
		} else {
			status = StatusFailed
			err = h.s.SaveFailed(h.ctx, e, "boom", failReasonInternal, r)
		}
		var want error
		switch {
		case !m.present:
			want = ErrRecordGone
			outcome = map[Status]string{StatusGenerated: outGeneratedGone, StatusFailed: outFailedGone}[status]
		case m.source > r:
			want = ErrSupersededRevision
			outcome = map[Status]string{StatusGenerated: outGeneratedSuperseded, StatusFailed: outFailedSuperseded}[status]
		case status == StatusFailed && m.status == StatusGenerated && m.source == r:
			want = ErrSupersededRevision
			outcome = outFailedRefusedEqual
		default:
			outcome = map[Status]string{StatusGenerated: outGeneratedSaved, StatusFailed: outFailedSaved}[status]
		}
		if (want == nil && err != nil) || (want != nil && !errors.Is(err, want)) {
			return "", fmt.Errorf("save %s(%s, %d) = %v, model says %v (stored %+v)", status, e, r, err, want, *m)
		}
		if want == nil {
			m.status, m.source, wrote = status, r, true
		}
	case opDelete:
		if err := h.s.DeleteEmbedding(h.ctx, e); err != nil {
			return "", fmt.Errorf("DeleteEmbedding(%s): %w", e, err)
		}
		m.present, outcome = false, outDeleted
	case opFenced:
		// A stale revision: one past the current one while the key is present (never equal
		// to it); while it is absent, the revision of its last stored record (the delete
		// marker after it is newer), or for a key never stored one no write reaches. A guess
		// at a delete marker's own revision is not a stale read: the Worker's revision comes
		// from a stored record.
		revision := m.kv + 1
		switch {
		case m.present && o.current:
			revision, outcome = m.kv, outFencedRemoved
		case m.present:
			outcome = outFencedStalePresent
		case m.kv > 0:
			revision, outcome = m.kv, outFencedAbsentCurrent
		default:
			revision, outcome = 1<<40, outFencedNeverStored
		}
		removed, err := h.s.deleteAtRevision(h.ctx, e, revision)
		if err != nil {
			return "", fmt.Errorf("deleteAtRevision(%s, %d): %w", e, revision, err)
		}
		want := m.present && o.current
		if removed != want {
			return "", fmt.Errorf("deleteAtRevision(%s, %d) removed = %v, model says %v (stored %+v)", e, revision, removed, want, *m)
		}
		if want {
			m.present = false
		}
	}
	return outcome, h.check(o.entity, wrote)
}

// check compares the entity's stored record and KV revision, read straight from the
// bucket, with the model.
func (h *history) check(entity int, wrote bool) error {
	e, m := historyEntities[entity], &h.model[entity]
	entry, err := h.index.Get(h.ctx, e)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		if m.present {
			return fmt.Errorf("%s is absent, model says present (%+v)", e, *m)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", e, err)
	}
	if !m.present {
		return fmt.Errorf("%s is present, model says absent", e)
	}
	var rec Record
	if err := json.Unmarshal(entry.Value(), &rec); err != nil {
		return fmt.Errorf("decode %s: %w", e, err)
	}
	if rec.Status != m.status || rec.SourceRevision != m.source {
		return fmt.Errorf("%s stored (%s, %d), model says (%s, %d)", e, rec.Status, rec.SourceRevision, m.status, m.source)
	}
	revision := entry.Revision()
	if wrote && revision <= m.kv {
		return fmt.Errorf("%s written but its KV revision %d did not move past %d", e, revision, m.kv)
	}
	if !wrote && revision != m.kv {
		return fmt.Errorf("%s not written but its KV revision moved from %d to %d", e, m.kv, revision)
	}
	m.kv = revision
	return nil
}

func pending(e int, r uint64) historyOp   { return historyOp{op: opPending, entity: e, revision: r} }
func generated(e int, r uint64) historyOp { return historyOp{op: opGenerated, entity: e, revision: r} }
func failed(e int, r uint64) historyOp    { return historyOp{op: opFailed, entity: e, revision: r} }
func deleted(e int) historyOp             { return historyOp{op: opDelete, entity: e} }
func fenced(e int, current bool) historyOp {
	return historyOp{op: opFenced, entity: e, current: current}
}

// TestRevisionGuardHistoryMatchesModel (design D3, D9): sequential histories of
// SavePendingGuarded, SaveGenerated, SaveFailed, DeleteEmbedding and the fenced no-text
// removal over two entities and source revisions 1-4, on a real bucket. After every call,
// its result and the entity's stored status, source revision and KV revision are checked
// against a reference model written from D3:
//
//   - pending is refused only over a generated record at the same or a newer source
//     revision (an older pending record replaces a newer pending or failed one);
//   - generated and failed return ErrRecordGone on an absent key and ErrSupersededRevision
//     below the stored source revision; failed is also refused over a generated record at
//     an equal source revision;
//   - the fenced removal removes only at the KV revision it is given, and otherwise
//     removes nothing and reports no error;
//   - a refused call leaves the key's KV revision where it was; a write moves it forward.
//
// Coverage of every rule comes from the witnesses: one fixed history per outcome, whose
// last call must reach it. The generated histories (Rapid, up to 30 calls) then explore
// orderings against the same model and assert no coverage of their own, so no seed can
// fail for lack of reach (#466; -rapid.seed=203 did, at 50 checks).
func TestRevisionGuardHistoryMatchesModel(t *testing.T) {
	client := fixtureClient(t)
	dedup := client.NewKVStore(createBucket(t, client, "EMB_HIST_DEDUP"))

	witnesses := []struct {
		want string
		ops  []historyOp
	}{
		{outPendingSaved, []historyOp{pending(0, 1)}},
		{outPendingOverNewer, []historyOp{pending(0, 3), pending(0, 1)}},
		{outPendingOverOlderGenerated, []historyOp{pending(0, 1), generated(0, 1), pending(0, 2)}},
		{outPendingRefused, []historyOp{pending(0, 2), generated(0, 2), pending(0, 1)}},
		{outPendingRefusedEqual, []historyOp{pending(0, 2), generated(0, 2), pending(0, 2)}},
		{outGeneratedSaved, []historyOp{pending(0, 1), generated(0, 1)}},
		{outGeneratedGone, []historyOp{generated(0, 1)}},
		{outGeneratedSuperseded, []historyOp{pending(0, 3), generated(0, 1)}},
		{outFailedSaved, []historyOp{pending(0, 1), failed(0, 1)}},
		{outFailedGone, []historyOp{pending(1, 1), failed(0, 1)}},
		{outFailedSuperseded, []historyOp{pending(0, 3), failed(0, 1)}},
		{outFailedRefusedEqual, []historyOp{pending(0, 1), generated(0, 1), failed(0, 1)}},
		{outDeleted, []historyOp{pending(0, 1), deleted(0)}},
		{outFencedRemoved, []historyOp{pending(0, 1), fenced(0, true)}},
		{outFencedStalePresent, []historyOp{pending(0, 1), pending(0, 2), fenced(0, false)}},
		{outFencedAbsentCurrent, []historyOp{pending(0, 1), deleted(0), fenced(0, true)}},
		{outFencedNeverStored, []historyOp{deleted(0), fenced(0, true)}},
	}
	for _, w := range witnesses {
		t.Run("witness/"+w.want, func(t *testing.T) {
			h, drop := newHistory(t, client, dedup)
			for i, o := range w.ops {
				outcome, err := h.apply(o)
				if err != nil {
					t.Fatalf("call %d %+v: %v", i, o, err)
				}
				if i == len(w.ops)-1 && outcome != w.want {
					t.Fatalf("the witness's last call reached %q, want %q", outcome, w.want)
				}
			}
			if err := drop(); err != nil {
				t.Fatalf("delete bucket: %v", err)
			}
		})
	}

	// The random histories run in their own subtest, so a failed witness above does not hand rapid
	// an already-failed *testing.T (rapid panics on one).
	t.Run("random", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			h, drop := newHistory(t, client, dedup)
			defer func() {
				if err := drop(); err != nil {
					rt.Errorf("delete bucket: %v", err)
				}
			}()
			steps := rapid.IntRange(1, 30).Draw(rt, "steps")
			for step := range steps {
				o := historyOp{entity: rapid.IntRange(0, len(historyEntities)-1).Draw(rt, "entity")}
				// Weighted so records live long enough to meet each other: pending 3, generated 2,
				// failed 2, delete 1, fenced removal 2.
				o.op = []int{opPending, opPending, opPending, opGenerated, opGenerated, opFailed, opFailed,
					opDelete, opFenced, opFenced}[rapid.IntRange(0, 9).Draw(rt, "op")]
				switch o.op {
				case opPending, opGenerated, opFailed:
					o.revision = drawRevision(rt, &h.model[o.entity])
				case opFenced:
					o.current = rapid.Bool().Draw(rt, "current")
				}
				if _, err := h.apply(o); err != nil {
					rt.Fatalf("step %d %+v: %v", step, o, err)
				}
			}
		})
	})
}

// drawRevision draws a source revision in 1-4, or the stored record's own revision: the
// equal-revision boundary D3's pending and failed rules turn on.
func drawRevision(t *rapid.T, m *modelRecord) uint64 {
	if m.present && rapid.Bool().Draw(t, "same revision") {
		return m.source
	}
	return rapid.Uint64Range(1, 4).Draw(t, "revision")
}

// TestSaveDedupKeepsSameSpaceReplacesOther (design D3, changed): on a real bucket, a
// SaveDedup for a key holding a record of the same model and dimensions writes nothing (the
// key's KV revision and bytes are unchanged), and one for a key holding another model's or
// another width's record replaces it.
func TestSaveDedupKeepsSameSpaceReplacesOther(t *testing.T) {
	client := fixtureClient(t)
	ctx := t.Context()
	index := createBucket(t, client, "EMBEDDING_INDEX")
	dedup := createBucket(t, client, "EMBEDDING_DEDUP")
	s := NewStorage(client.NewKVStore(index), client.NewKVStore(dedup))
	const key = "dedup-key"

	read := func() (DedupRecord, []byte, uint64) {
		t.Helper()
		entry, err := dedup.Get(ctx, key)
		if err != nil {
			t.Fatalf("read %s: %v", key, err)
		}
		var rec DedupRecord
		if err := json.Unmarshal(entry.Value(), &rec); err != nil {
			t.Fatalf("decode %s: %v", key, err)
		}
		return rec, entry.Value(), entry.Revision()
	}

	if err := s.SaveDedup(ctx, key, []float32{1, 2, 3}, "model-a", 3); err != nil {
		t.Fatalf("SaveDedup (first): %v", err)
	}
	_, firstBytes, firstRev := read()

	if err := s.SaveDedup(ctx, key, []float32{4, 5, 6}, "model-a", 3); err != nil {
		t.Fatalf("SaveDedup (same space): %v", err)
	}
	_, sameBytes, sameRev := read()
	if sameRev != firstRev || string(sameBytes) != string(firstBytes) {
		t.Fatalf("a same-space SaveDedup rewrote the record: revision %d -> %d, %s -> %s", firstRev, sameRev, firstBytes, sameBytes)
	}

	if err := s.SaveDedup(ctx, key, []float32{7, 8, 9}, "model-b", 3); err != nil {
		t.Fatalf("SaveDedup (other model): %v", err)
	}
	rec, _, otherRev := read()
	if otherRev <= sameRev || rec.Model != "model-b" || rec.Dimensions != 3 || len(rec.Vector) != 3 || rec.Vector[0] != 7 {
		t.Fatalf("another model's SaveDedup left %+v at revision %d, want model-b's [7 8 9] past revision %d", rec, otherRev, sameRev)
	}

	if err := s.SaveDedup(ctx, key, []float32{1, 1}, "model-b", 2); err != nil {
		t.Fatalf("SaveDedup (other width): %v", err)
	}
	rec, _, widthRev := read()
	if widthRev <= otherRev || rec.Dimensions != 2 || len(rec.Vector) != 2 {
		t.Fatalf("another width's SaveDedup left %+v at revision %d, want the 2-wide vector past revision %d", rec, widthRev, otherRev)
	}
}
