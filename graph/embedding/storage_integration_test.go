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

// observe reads the key's stored record and KV revision straight from the bucket.
func observe(t *rapid.T, ctx context.Context, kv jetstream.KeyValue, key string) (present bool, rec Record, revision uint64) {
	entry, err := kv.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, Record{}, 0
	}
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	if err := json.Unmarshal(entry.Value(), &rec); err != nil {
		t.Fatalf("decode %s: %v", key, err)
	}
	return true, rec, entry.Revision()
}

// TestRevisionGuardHistoryMatchesModel (design D3, D9): a generated sequential history of
// up to 30 calls of SavePendingGuarded, SaveGenerated, SaveFailed, DeleteEmbedding and the
// fenced no-text removal over two entities and source revisions 1-4, on a real bucket.
// After every call, its result and each entity's stored status, source revision and KV
// revision are checked against a reference model written from D3:
//
//   - pending is refused only over a generated record at the same or a newer source
//     revision (an older pending record replaces a newer pending or failed one);
//   - generated and failed return ErrRecordGone on an absent key and ErrSupersededRevision
//     below the stored source revision; failed is also refused over a generated record at
//     an equal source revision;
//   - the fenced removal removes only at the KV revision it is given, and otherwise
//     removes nothing and reports no error;
//   - a refused call leaves the key's KV revision where it was; a write moves it forward.
func TestRevisionGuardHistoryMatchesModel(t *testing.T) {
	client := fixtureClient(t)
	ctx := t.Context()
	entities := []string{"acme.ops.robotics.gcs.drone.h0", "acme.ops.robotics.gcs.drone.h1"}
	runs := 0
	// reached counts each outcome the model distinguishes, so the test can show every
	// branch of D3's rules was exercised and not only generated.
	reached := map[string]int{}

	rapid.Check(t, func(rt *rapid.T) {
		runs++
		n := runs
		indexName, dedupName := fmt.Sprintf("EMB_HIST_IDX_%d", n), fmt.Sprintf("EMB_HIST_DEDUP_%d", n)
		index := createBucket(t, client, indexName)
		dedup := createBucket(t, client, dedupName)
		defer func() {
			for _, name := range []string{indexName, dedupName} {
				if err := client.DeleteKeyValueBucket(ctx, name); err != nil {
					rt.Errorf("delete bucket %s: %v", name, err)
				}
			}
		}()
		s := NewStorage(client.NewKVStore(index), client.NewKVStore(dedup))
		model := map[string]*modelRecord{entities[0]: {}, entities[1]: {}}

		steps := rapid.IntRange(1, 30).Draw(rt, "steps")
		for step := range steps {
			e := rapid.SampledFrom(entities).Draw(rt, "entity")
			m := model[e]
			wrote := false
			// Weighted so a history keeps records long enough to reach the equal-revision and
			// newer-record rules: pending 3, generated 2, failed 2, delete 1, fenced removal 2.
			op := []int{0, 0, 0, 1, 1, 2, 2, 3, 4, 4}[rapid.IntRange(0, 9).Draw(rt, "op")]
			switch op {
			case 0:
				r := drawRevision(rt, m)
				saved, err := s.SavePendingGuarded(ctx, &Record{EntityID: e, SourceText: "t", SourceRevision: r})
				if err != nil {
					rt.Fatalf("step %d: SavePendingGuarded(%s, %d): %v", step, e, r, err)
				}
				want := !(m.present && m.status == StatusGenerated && m.source >= r)
				if saved != want {
					rt.Fatalf("step %d: SavePendingGuarded(%s, %d) saved = %v, model says %v (stored %+v)", step, e, r, saved, want, *m)
				}
				if want {
					if m.present && m.source > r {
						reached["pending saved over a newer record"]++
					}
					if m.present && m.status == StatusGenerated {
						reached["pending saved over an older generated record"]++
					}
					m.present, m.status, m.source, wrote = true, StatusPending, r, true
					reached["pending saved"]++
				} else {
					reached["pending refused over generated"]++
					if m.source == r {
						reached["pending refused over generated at equal revision"]++
					}
				}
			case 1, 2:
				r := drawRevision(rt, m)
				var err error
				status := StatusGenerated
				if op == 1 {
					err = s.SaveGenerated(ctx, e, []float32{float32(r)}, "m", 1, "h", r)
				} else {
					status = StatusFailed
					err = s.SaveFailed(ctx, e, "boom", failReasonInternal, r)
				}
				var want error
				switch {
				case !m.present:
					want = ErrRecordGone
				case m.source > r:
					want = ErrSupersededRevision
				case status == StatusFailed && m.status == StatusGenerated && m.source == r:
					want = ErrSupersededRevision
					reached["failed refused over generated at equal revision"]++
				}
				if (want == nil && err != nil) || (want != nil && !errors.Is(err, want)) {
					rt.Fatalf("step %d: save %s(%s, %d) = %v, model says %v (stored %+v)", step, status, e, r, err, want, *m)
				}
				if want == nil {
					reached[string(status)+" saved"]++
					m.status, m.source, wrote = status, r, true
				} else {
					reached[fmt.Sprintf("%s refused: %v", status, want)]++
				}
			case 3:
				if err := s.DeleteEmbedding(ctx, e); err != nil {
					rt.Fatalf("step %d: DeleteEmbedding(%s): %v", step, e, err)
				}
				m.present = false
				reached["deleted"]++
			case 4:
				current := rapid.Bool().Draw(rt, "current")
				// A stale revision: one past the current one while the key is present (never
				// equal to it); while it is absent, the revision of its last stored record
				// (the delete marker after it is newer), or for a key never stored one no
				// write in this run reaches. A guess at a delete marker's own revision is
				// not a stale read: the Worker's revision comes from a stored record.
				revision := m.kv + 1
				switch {
				case m.present && current:
					revision = m.kv
				case !m.present && m.kv > 0:
					revision = m.kv
				case !m.present:
					revision = 1 << 40
				}
				removed, err := s.deleteAtRevision(ctx, e, revision)
				if err != nil {
					rt.Fatalf("step %d: deleteAtRevision(%s, %d): %v", step, e, revision, err)
				}
				want := m.present && current
				if removed != want {
					rt.Fatalf("step %d: deleteAtRevision(%s, %d) removed = %v, model says %v (stored %+v)", step, e, revision, removed, want, *m)
				}
				if want {
					m.present = false
				}
				reached[fmt.Sprintf("fenced removal present=%v current=%v", m.present || want, current)]++
			}

			present, rec, revision := observe(rt, ctx, index, e)
			if present != m.present {
				rt.Fatalf("step %d: %s present = %v, model says %v", step, e, present, m.present)
			}
			if !present {
				continue
			}
			if rec.Status != m.status || rec.SourceRevision != m.source {
				rt.Fatalf("step %d: %s stored (%s, %d), model says (%s, %d)", step, e, rec.Status, rec.SourceRevision, m.status, m.source)
			}
			if wrote && revision <= m.kv {
				rt.Fatalf("step %d: %s written but its KV revision %d did not move past %d", step, e, revision, m.kv)
			}
			if !wrote && revision != m.kv {
				rt.Fatalf("step %d: %s not written but its KV revision moved from %d to %d", step, e, m.kv, revision)
			}
			m.kv = revision
		}
	})

	t.Logf("outcomes reached: %v", reached)
	for _, outcome := range []string{
		"pending saved", "pending saved over a newer record", "pending refused over generated",
		"pending saved over an older generated record", "pending refused over generated at equal revision",
		"generated saved", "generated refused: " + ErrRecordGone.Error(), "generated refused: " + ErrSupersededRevision.Error(),
		"failed saved", "failed refused: " + ErrRecordGone.Error(), "failed refused: " + ErrSupersededRevision.Error(),
		"failed refused over generated at equal revision",
		"deleted",
		"fenced removal present=true current=true", "fenced removal present=true current=false",
		"fenced removal present=false current=true", "fenced removal present=false current=false",
	} {
		if reached[outcome] == 0 {
			t.Errorf("no generated history reached %q; the generator does not exercise that rule", outcome)
		}
	}
}

// drawRevision draws a source revision in 1-4, or, by construction, the stored record's own
// revision: the equal-revision boundary D3's pending and failed rules turn on.
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
