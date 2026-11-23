package embedding

import (
	"errors"
	"slices"
	"testing"
)

// overCapText is longer than the cap of 10 runes the tests below set.
const overCapText = "long text that the cap cuts"

// TestFailedGenerationCountsNoCut: a text the cap cut, whose embedding the provider refuses, ends
// in a failed record. No entity was embedded, so nothing is counted under either label.
func TestFailedGenerationCountsNoCut(t *testing.T) {
	ctx := t.Context()
	index := newMemKV()
	s := newTestStorage(t, index, newMemKV())
	const id = "acme.ops.robotics.gcs.drone.refused"
	if err := pendingText(ctx, s, id, "", overCapText, 1); err != nil {
		t.Fatal(err)
	}
	entry, err := index.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	m := &countingMetrics{}
	refused := &stubEmbedder{model: "m", dimensions: 3, generate: func([]string) ([][]float32, error) {
		return nil, errors.New("dial tcp: connect: connection refused")
	}}
	w := NewWorker(s, refused, discardLogger()).WithMaxSourceTextLen(10).WithMetrics(m)

	if _, _, terminal, outcome, _ := w.handleKVEntry(ctx, entry, false, 0); !terminal || outcome != OutcomeFailed {
		t.Fatalf("outcome %v, terminal %v; want a terminal failure", outcome, terminal)
	}
	if m.truncated != 0 {
		t.Fatalf("truncated counted %v, want nothing: the provider refused, so no entity was embedded", m.truncatedBy)
	}
}

// TestFailedThenRecoveredCountsOnce: the same entity fails once, then its failed record is
// processed again from the initial snapshot, as after a restart, and is embedded. The cap cut its
// text both times; it is counted once, under text_cap, for the one stored vector.
func TestFailedThenRecoveredCountsOnce(t *testing.T) {
	ctx := t.Context()
	index := newMemKV()
	s := newTestStorage(t, index, newMemKV())
	const id = "acme.ops.robotics.gcs.drone.recovered"
	if err := pendingText(ctx, s, id, "", overCapText, 1); err != nil {
		t.Fatal(err)
	}
	calls := 0
	flaky := &stubEmbedder{model: "m", dimensions: 3, generate: func([]string) ([][]float32, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("dial tcp: connect: connection refused")
		}
		return [][]float32{{1, 2, 3}}, nil
	}}
	m := &countingMetrics{}
	w := NewWorker(s, flaky, discardLogger()).WithMaxSourceTextLen(10).WithMetrics(m)

	pending, err := index.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, outcome, _ := w.handleKVEntry(ctx, pending, false, 0); outcome != OutcomeFailed {
		t.Fatalf("first attempt: outcome %v, want failed", outcome)
	}
	failed, err := index.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, terminal, outcome, _ := w.handleKVEntry(ctx, failed, true, 0); !terminal || outcome != OutcomeGenerated {
		t.Fatalf("replay: outcome %v, terminal %v; want generated", outcome, terminal)
	}
	if want := []string{TruncatedByTextCap}; !slices.Equal(m.truncatedBy, want) {
		t.Fatalf("truncated counted %v, want %v: one entity embedded once from its opening", m.truncatedBy, want)
	}
}
