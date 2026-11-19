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
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"pgregory.net/rapid"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/revlag"
	"github.com/c360studio/semengine/pkg/errs"
)

// The plug-in reports a damaged entity with the refusal graph.UnmarshalEntityState returned for
// its stored value. These tests build each refusal the same way, so the reasons they expect are
// the decoder's.

const damagedPrefix = "acme.ops.test.system.widget."

func damagedID(n int) string { return fmt.Sprintf("%s%03d", damagedPrefix, n) }

// otherID is an entity outside damagedPrefix, for the count's ID filter.
const otherID = "acme.ops.test.other.widget.003"

// unreadable is a stored value that does not decode.
const unreadable = "{not json"

// refusalOf is the refusal graph.UnmarshalEntityState returns for value stored under key.
func refusalOf(t *testing.T, key, value string) error {
	t.Helper()
	var state graph.EntityState
	err := graph.UnmarshalEntityState(key, []byte(value), &state)
	if err == nil {
		t.Fatalf("UnmarshalEntityState(%q, %q) accepted the value; the test needs a refusal", key, value)
	}
	return err
}

// damagedFollower is a Follower with what the damage record, Complete, observe, Stuck and the
// envelope read; its plug-in has no failures and applies nothing.
func damagedFollower() *Follower {
	return &Follower{
		key:       "graph-index",
		logger:    slog.New(slog.DiscardHandler),
		deliver:   func(context.Context, Entry) {},
		failed:    func() (int64, map[string]uint64) { return 0, nil },
		watermark: revlag.New(),
	}
}

// deleteEntry is a delivered delete marker: only what observe reads is implemented.
type deleteEntry struct {
	jetstream.KeyValueEntry
	key      string
	revision uint64
}

func (e deleteEntry) Key() string                     { return e.key }
func (e deleteEntry) Revision() uint64                { return e.revision }
func (e deleteEntry) Created() time.Time              { return time.Unix(int64(e.revision), 0) }
func (e deleteEntry) Operation() jetstream.KeyValueOp { return jetstream.KeyValueDelete }

// deliverDelete hands the follower a delete of key at revision, as the watch delivers one.
func deliverDelete(ctx context.Context, f *Follower, key string, revision uint64) {
	f.observe(ctx, deleteEntry{key: key, revision: revision})
}

func requireRecorded(t *testing.T, f *Follower, key string, want bool, after string) {
	t.Helper()
	if got := f.DamagedRefusal(key) != nil; got != want {
		t.Fatalf("after %s: %s recorded %v; want %v", after, key, got, want)
	}
}

// TestDamagedEntityClearsOnHigherCompletion: a report is cleared only by a completion of a higher
// revision of the key (entity-state-follower spec, "Report, then repair", "A completion below the
// report", "A failed read, then the repair").
func TestDamagedEntityClearsOnHigherCompletion(t *testing.T) {
	b := damagedID(2)
	t.Run("report, then repair", func(t *testing.T) {
		f := damagedFollower()
		f.ReportDamaged(b, 5, refusalOf(t, b, unreadable))
		requireRecorded(t, f, b, true, "the report at 5")
		f.Complete(b, 5)
		requireRecorded(t, f, b, true, "a completion at 5, the reported revision")
		f.Complete(b, 7)
		requireRecorded(t, f, b, false, "a completion at 7")
	})
	t.Run("a completion below the report", func(t *testing.T) {
		// The plug-in read revision 7 and reports it, then completes revision 6, the one delivered.
		f := damagedFollower()
		f.ReportDamaged(b, 7, refusalOf(t, b, unreadable))
		f.Complete(b, 6)
		requireRecorded(t, f, b, true, "a completion at 6 below the report at 7")
	})
	t.Run("a failed read, then the repair", func(t *testing.T) {
		f := damagedFollower()
		f.ReportDamaged(b, 5, refusalOf(t, b, unreadable))
		f.Complete(b, 6) // after a failed read: the plug-in never saw revision 6's value
		requireRecorded(t, f, b, false, "a completion at 6 after a failed read")
		f.ReportDamaged(b, 6, refusalOf(t, b, unreadable))
		requireRecorded(t, f, b, true, "the repair's report at 6")
	})
	t.Run("other keys are untouched", func(t *testing.T) {
		f := damagedFollower()
		f.ReportDamaged(b, 5, refusalOf(t, b, unreadable))
		f.Complete(damagedID(1), 9)
		requireRecorded(t, f, b, true, "a completion of another key")
	})
}

// TestDamagedEntityClearsOnDelete: a delete of the key delivered above the report clears it; one
// at the reported revision does not (spec, "A delete clears it").
func TestDamagedEntityClearsOnDelete(t *testing.T) {
	b := damagedID(2)
	f := damagedFollower()
	f.ReportDamaged(b, 5, refusalOf(t, b, unreadable))
	deliverDelete(t.Context(), f, b, 5)
	requireRecorded(t, f, b, true, "a delete at 5, the reported revision")
	deliverDelete(t.Context(), f, damagedID(1), 6)
	requireRecorded(t, f, b, true, "a delete of another key")
	deliverDelete(t.Context(), f, b, 7)
	requireRecorded(t, f, b, false, "a delete at 7")
}

// TestDamagedEntityCountsOneError: a report returns true only when it newly records the key, so
// the plug-in counts one error per newly damaged entity (spec, "The same damage read twice").
func TestDamagedEntityCountsOneError(t *testing.T) {
	b := damagedID(2)
	refusal := refusalOf(t, b, unreadable)
	f := damagedFollower()
	steps := []struct {
		what string
		do   func() bool
		want bool
	}{
		{"the first report at 5", func() bool { return f.ReportDamaged(b, 5, refusal) }, true},
		{"the same report again", func() bool { return f.ReportDamaged(b, 5, refusal) }, false},
		{"a report at 6 while recorded", func() bool { return f.ReportDamaged(b, 6, refusal) }, false},
		{"a report at 4 while recorded", func() bool { return f.ReportDamaged(b, 4, refusal) }, false},
		{"a report after a completion at 7 cleared it", func() bool { f.Complete(b, 7); return f.ReportDamaged(b, 7, refusal) }, true},
	}
	for _, s := range steps {
		if got := s.do(); got != s.want {
			t.Errorf("%s: ReportDamaged = %v; want %v", s.what, got, s.want)
		}
	}
}

// TestDamagedCountAndRefusal: the count is of every recorded entity or of those the filter
// keeps, and the refusal for one ID is the one recorded for it, or nil (spec, "Count and
// refusal").
func TestDamagedCountAndRefusal(t *testing.T) {
	a, b, c := damagedID(1), damagedID(2), otherID
	f := damagedFollower()
	f.ReportDamaged(b, 5, refusalOf(t, b, unreadable))
	f.ReportDamaged(c, 6, refusalOf(t, c, unreadable))

	if got := f.DamagedCount(nil); got != 2 {
		t.Errorf("DamagedCount(nil) = %d; want 2", got)
	}
	underB := func(id string) bool { return strings.HasPrefix(id, damagedPrefix) }
	if got := f.DamagedCount(underB); got != 1 {
		t.Errorf("DamagedCount(under %s) = %d; want 1", damagedPrefix, got)
	}

	err := f.DamagedRefusal(b)
	var ce *errs.ClassifiedError
	if !errors.As(err, &ce) || ce.Class != errs.ErrorFatal || ce.Code != graph.ErrorCodeGraphStateResetRequired {
		t.Errorf("DamagedRefusal(B) = %v; want fatal %s", err, graph.ErrorCodeGraphStateResetRequired)
	}
	var stateErr *graph.StateContractError
	if !errors.As(err, &stateErr) || stateErr.EntityID != b || stateErr.Reason != graph.GraphStateReasonUnreadableEntity {
		t.Errorf("DamagedRefusal(B) = %v; want the state contract naming %s, %s", err, b, graph.GraphStateReasonUnreadableEntity)
	}
	if err := f.DamagedRefusal(a); err != nil {
		t.Errorf("DamagedRefusal(A) = %v; want nil, A is not damaged", err)
	}
}

// TestStuckNamesDamagedEntities: Stuck holds one sentence with the count, the first 10 IDs in
// sorted order each with its reason, the number not named, and the repair (spec, "Eleven damaged
// entities"). The IDs are reported out of order, so the sorting is the follower's.
func TestStuckNamesDamagedEntities(t *testing.T) {
	const wrongID = "acme.ops.test.system.widget.999"
	wrongValue := `{"id":"` + wrongID + `","triples":[]}`
	f := damagedFollower()
	for _, n := range []int{11, 3, 7, 1, 10, 5, 2, 9, 4, 8, 6} {
		value := unreadable
		if n == 4 {
			value = wrongValue
		}
		f.ReportDamaged(damagedID(n), uint64(n), refusalOf(t, damagedID(n), value))
	}
	var stateErr *graph.StateContractError
	if !errors.As(f.DamagedRefusal(damagedID(4)), &stateErr) || stateErr.Reason != graph.GraphStateReasonNoncanonicalEntityID {
		t.Fatalf("the value under %s is not refused as %s; the test reads nothing", damagedID(4), graph.GraphStateReasonNoncanonicalEntityID)
	}

	named := make([]string, 0, 10)
	for n := 1; n <= 10; n++ {
		reason := "unreadable_entity_state"
		if n == 4 {
			reason = "noncanonical_entity_id"
		}
		named = append(named, fmt.Sprintf("%s (%s)", damagedID(n), reason))
	}
	want := []string{"11 damaged entities: " + strings.Join(named, ", ") +
		", and 1 more; write a valid value for each key or delete it"}
	if got := f.Stuck(); !slices.Equal(got, want) {
		t.Errorf("Stuck = %q;\nwant %q", got, want)
	}

	one := damagedFollower()
	one.ReportDamaged(damagedID(1), 1, refusalOf(t, damagedID(1), unreadable))
	wantOne := []string{"1 damaged entity: " + damagedID(1) +
		" (unreadable_entity_state); write a valid value for each key or delete it"}
	if got := one.Stuck(); !slices.Equal(got, wantOne) {
		t.Errorf("one damaged: Stuck = %q;\nwant %q", got, wantOne)
	}
}

// TestEnvelopeCountsDamagedEntities: a recorded damaged entity reads not ready and degraded, is
// counted in failed_count beside the plug-in's failures and under failed_reasons
// damaged_entity, and names no ID (spec, "A damaged entity", "Failure reasons"). The floor has
// reached the snapshot's target in every case, so the first build completes with damage stored.
func TestEnvelopeCountsDamagedEntities(t *testing.T) {
	cases := []struct {
		name        string
		failed      int64
		reasons     map[string]uint64
		damaged     int
		wantCount   uint64
		wantReasons map[string]uint64
	}{
		{"one damaged, no failures", 0, nil, 1, 1, map[string]uint64{"damaged_entity": 1}},
		{"failures without reasons beside damage", 2, nil, 1, 3, map[string]uint64{"damaged_entity": 1}},
		{"failures with reasons beside damage", 3, map[string]uint64{"timeout": 3}, 2, 5,
			map[string]uint64{"timeout": 3, "damaged_entity": 2}},
		{"stray reasons, no failures", 0, map[string]uint64{"timeout": 1}, 1, 1, map[string]uint64{"damaged_entity": 1}},
		{"no damage, no failures", 0, nil, 0, 0, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := damagedFollower()
			f.failed = func() (int64, map[string]uint64) { return tc.failed, maps.Clone(tc.reasons) }
			observeAll(t, f, 1)
			f.Complete("k1", 1)
			f.bootstrapTarget.Store(1)
			f.snapshotDone.Store(true)
			for n := 1; n <= tc.damaged; n++ {
				f.ReportDamaged(damagedID(n), 1, refusalOf(t, damagedID(n), unreadable))
			}

			status := read(f, 1)
			if status.FailedCount != tc.wantCount || !maps.Equal(status.FailedReasons, tc.wantReasons) {
				t.Errorf("failed_count %d, failed_reasons %v; want %d and %v",
					status.FailedCount, status.FailedReasons, tc.wantCount, tc.wantReasons)
			}
			wantReady := tc.failed == 0 && tc.damaged == 0
			wantState := graph.IndexStateReady
			if !wantReady {
				wantState = graph.IndexStateDegraded
			}
			if status.Ready != wantReady || status.State != wantState {
				t.Errorf("ready %v, state %s; want %v, %s", status.Ready, status.State, wantReady, wantState)
			}
			if !status.BootstrapComplete {
				t.Error("bootstrap_complete false; the floor reached the snapshot's target")
			}
			data, err := json.Marshal(status)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), damagedPrefix) {
				t.Errorf("envelope %s names an entity", data)
			}
		})
	}
}

// TestDamagedEntityNeverWithholdsQuery: a recorded damaged entity does not make QueryReady refuse,
// before or after the first build, though Stuck names it (spec, "A damaged entity never withholds
// a query").
func TestDamagedEntityNeverWithholdsQuery(t *testing.T) {
	t.Run("after the first build", func(t *testing.T) {
		f := builtFollower("graph-index", func() (int64, map[string]uint64) { return 0, nil }, nil)
		f.ReportDamaged(damagedID(2), 5, refusalOf(t, damagedID(2), unreadable))
		if err := f.QueryReady(t.Context()); err != nil {
			t.Errorf("QueryReady = %v; want nil", err)
		}
		if len(f.Stuck()) == 0 {
			t.Error("Stuck is empty; want the damaged entity named")
		}
	})
	t.Run("damage stored at start", func(t *testing.T) {
		f := damagedFollower()
		observeAll(t, f, 2)
		f.bootstrapTarget.Store(2)
		f.snapshotDone.Store(true)
		f.ReportDamaged("k2", 2, refusalOf(t, "k2", unreadable))
		f.Complete("k1", 1)
		f.Complete("k2", 2) // the plug-in completes the damaged revision
		// The heartbeat's envelope latches the build once the floor reaches the target; QueryReady
		// is asked after it, so it never reads the stream.
		read(f, 2)
		if !f.bootstrapped.Load() {
			t.Fatal("the envelope did not latch the build with damage stored")
		}
		if err := f.QueryReady(t.Context()); err != nil {
			t.Errorf("QueryReady = %v; want nil", err)
		}
	})
}

// A damage history step: a report, a completion, or a delivered delete of one key at a revision.
const (
	damageReport = iota
	damageComplete
	damageDelete
)

type damageStep struct {
	kind     int
	key      int
	revision uint64
}

var damageKeys = []string{damagedID(1), damagedID(2), otherID}

// damageModel is the test's reference: every step applied, per key, in order. It holds no
// record of its own; recorded reads the requirement off the history.
type damageModel struct{ history map[string][]damageStep }

// recorded is the requirement: one entry per key at the highest reported revision, dropped by a
// completion or a delete above it. Read off a history, the key is recorded exactly when some
// report of it has no later completion or delete above that report's revision.
func (m damageModel) recorded(key string) bool {
	steps := m.history[key]
	for i, s := range steps {
		if s.kind != damageReport {
			continue
		}
		if !slices.ContainsFunc(steps[i+1:], func(later damageStep) bool {
			return later.kind != damageReport && later.revision > s.revision
		}) {
			return true
		}
	}
	return false
}

// lastReport is the revision of key's last report, 0 for none.
func (m damageModel) lastReport(key string) uint64 {
	steps := m.history[key]
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].kind == damageReport {
			return steps[i].revision
		}
	}
	return 0
}

// damageRun applies steps to one follower and to the model, checking after each step.
type damageRun struct {
	ctx      context.Context
	f        *Follower
	model    damageModel
	refusals map[string]error
}

func newDamageRun(ctx context.Context, refusals map[string]error) *damageRun {
	return &damageRun{ctx: ctx, f: damagedFollower(), model: damageModel{history: map[string][]damageStep{}}, refusals: refusals}
}

type fataler interface {
	Fatalf(format string, args ...any)
}

func (r *damageRun) apply(t fataler, s damageStep) {
	key := damageKeys[s.key]
	switch s.kind {
	case damageReport:
		wasRecorded := r.model.recorded(key)
		if got := r.f.ReportDamaged(key, s.revision, r.refusals[key]); got != !wasRecorded {
			t.Fatalf("report of %s at %d returned %v; want %v (recorded before: %v)", key, s.revision, got, !wasRecorded, wasRecorded)
		}
	case damageComplete:
		r.f.Complete(key, s.revision)
	case damageDelete:
		deliverDelete(r.ctx, r.f, key, s.revision)
	}
	r.model.history[key] = append(r.model.history[key], s)
	r.check(t, fmt.Sprintf("step %+v", s))
}

// check compares every read of the record with the model: each key's refusal, both counts, the
// envelope's damaged_entity and Stuck's sentence.
func (r *damageRun) check(t fataler, after string) {
	want, under := 0, 0
	for _, key := range damageKeys {
		recorded := r.model.recorded(key)
		if got := r.f.DamagedRefusal(key); (got != nil) != recorded {
			t.Fatalf("after %s: DamagedRefusal(%s) = %v; want recorded %v", after, key, got, recorded)
		}
		if recorded {
			want++
			if strings.HasPrefix(key, damagedPrefix) {
				under++
			}
		}
	}
	if got := r.f.DamagedCount(nil); got != want {
		t.Fatalf("after %s: DamagedCount(nil) = %d; want %d", after, got, want)
	}
	if got := r.f.DamagedCount(func(id string) bool { return strings.HasPrefix(id, damagedPrefix) }); got != under {
		t.Fatalf("after %s: filtered DamagedCount = %d; want %d", after, got, under)
	}
	status := read(r.f, 0)
	if got := status.FailedReasons["damaged_entity"]; got != uint64(want) || status.FailedCount != uint64(want) {
		t.Fatalf("after %s: envelope failed_count %d, damaged_entity %d; want %d", after, status.FailedCount, got, want)
	}
	// A drawn delete can arrive below a revision already delivered, which declares the feed
	// lost; that sentence is not the one under test.
	sentences := slices.DeleteFunc(r.f.Stuck(), func(s string) bool { return !strings.Contains(s, " damaged entit") })
	switch {
	case want == 0 && len(sentences) != 0:
		t.Fatalf("after %s: Stuck names damage %q; want none", after, sentences)
	case want > 0 && (len(sentences) != 1 || !strings.HasPrefix(sentences[0], fmt.Sprintf("%d damaged ", want))):
		t.Fatalf("after %s: Stuck damage sentences %q; want one counting %d", after, sentences, want)
	}
}

// drawStep draws a step for one key. A completion or delete is drawn below, at or above the
// key's last report, so each boundary the requirement names is reached by construction; with no
// report yet, at any revision.
func (r *damageRun) drawStep(t *rapid.T) damageStep {
	s := damageStep{kind: rapid.IntRange(damageReport, damageDelete).Draw(t, "kind"), key: rapid.IntRange(0, len(damageKeys)-1).Draw(t, "key")}
	if s.kind == damageReport {
		s.revision = rapid.Uint64Range(2, 9).Draw(t, "report revision")
		return s
	}
	last := r.model.lastReport(damageKeys[s.key])
	if last == 0 {
		s.revision = rapid.Uint64Range(1, 10).Draw(t, "revision")
		return s
	}
	switch rapid.IntRange(0, 2).Draw(t, "relative to the last report") {
	case 0:
		s.revision = last - 1
	case 1:
		s.revision = last
	default:
		s.revision = last + rapid.Uint64Range(1, 2).Draw(t, "above by")
	}
	return s
}

// TestPropDamagedSetFollowsLatestEvent: over generated histories of reports, completions at,
// below and above a report, and deletes, across three keys, every read of the record agrees with
// the reference model after every step (spec, "Any history"). The named witnesses run first
// through the same checks.
func TestPropDamagedSetFollowsLatestEvent(t *testing.T) {
	refusals := map[string]error{}
	for _, key := range damageKeys {
		refusals[key] = refusalOf(t, key, unreadable)
	}
	report := func(key int, rev uint64) damageStep { return damageStep{damageReport, key, rev} }
	complete := func(key int, rev uint64) damageStep { return damageStep{damageComplete, key, rev} }
	remove := func(key int, rev uint64) damageStep { return damageStep{damageDelete, key, rev} }
	witnesses := []struct {
		name  string
		steps []damageStep
	}{
		{"a completion at the report keeps it", []damageStep{report(0, 5), complete(0, 5), complete(0, 6)}},
		{"a lower report keeps the higher one", []damageStep{report(0, 5), report(0, 3), complete(0, 4), complete(0, 6)}},
		{"cleared, then reported lower", []damageStep{report(0, 5), complete(0, 7), report(0, 3), complete(0, 4)}},
		{"a delete at the report keeps it", []damageStep{report(1, 5), remove(1, 5), remove(1, 6)}},
		{"keys apart", []damageStep{report(0, 5), report(2, 5), complete(2, 6), remove(1, 7)}},
	}
	for _, w := range witnesses {
		t.Run(w.name, func(t *testing.T) {
			run := newDamageRun(t.Context(), refusals)
			for _, s := range w.steps {
				run.apply(t, s)
			}
		})
	}
	if t.Failed() {
		return // a failing witness is the finding; a generated run would only repeat it
	}

	ctx := t.Context()
	rapid.Check(t, func(t *rapid.T) {
		run := newDamageRun(ctx, refusals)
		for range rapid.IntRange(1, 30).Draw(t, "steps") {
			run.apply(t, run.drawStep(t))
		}
	})
}
