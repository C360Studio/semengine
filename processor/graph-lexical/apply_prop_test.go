package graphlexical

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"pgregory.net/rapid"

	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/internal/revlag"
)

// TestStaleBodyReadDropped: a body read for revision R that returns after the same key's R+1 is
// applied is dropped: the index holds R+1's text and never R's body, and the follower's floor
// passes R. R+1 has an offloaded body of its own, so only R's own completion can move the floor
// past R. The floor is a revlag.Watermark, the follower's own, fed as the follower feeds it.
func TestStaleBodyReadDropped(t *testing.T) {
	floor := revlag.New()
	a := newTestApplier(t, floor.Complete)
	deliver := func(e follower.Entry) (bodyRead, bool) {
		t.Helper()
		if err := floor.Observe(e.Revision, e.Key, time.Time{}); err != nil {
			t.Fatalf("observe: %v", err)
		}
		return mustDeliver(t, a, e)
	}
	indexed := func() uint64 { r, _ := floor.IndexedAt(); return r }

	const r = 5
	staleRead, ok := deliver(stateEntry(t, pump, r, text{name: "oldname", inline: "oldinline", offloaded: true}))
	if !ok {
		t.Fatal("a delivery with an offloaded body asked for no body read")
	}
	freshRead, ok := deliver(stateEntry(t, pump, r+1, text{name: "newname", inline: "newinline", offloaded: true}))
	if !ok {
		t.Fatal("the second delivery with an offloaded body asked for no body read")
	}
	if got := indexed(); got != r-1 {
		t.Fatalf("floor = %d with both body reads out; want %d", got, r-1)
	}

	a.bodyDone(staleRead, "stalebody", nil)
	requireHeld(t, a.index, []string{pump}, "newname", "newinline")
	requireHeld(t, a.index, nil, "stalebody", "oldname", "oldinline")
	if got := indexed(); got != r {
		t.Fatalf("floor = %d after the stale read returned; want %d, past the dropped revision and not past R+1", got, r)
	}

	a.bodyDone(freshRead, "freshbody", nil)
	requireHeld(t, a.index, []string{pump}, "newname", "newinline", "freshbody")
	requireHeld(t, a.index, nil, "stalebody")
	if got := indexed(); got != r+1 {
		t.Fatalf("floor = %d after R+1's read returned; want %d", got, r+1)
	}
}

// propKeys are the entities a generated history writes.
var propKeys = []string{"c360.lex.prop.doc.item.a", "c360.lex.prop.doc.item.b", "c360.lex.prop.doc.item.c"}

// stepKind is one kind of step in a history.
type stepKind int

const (
	stepDeliver stepKind = iota // a write of the key at the next revision
	stepDelete                  // a delete of the key at the next revision
	stepOlder                   // a delivery of the key at a revision it already passed
	stepReturn                  // a body read that is out returns
)

// step is one step of a history. A delivery's words are unique to its key and revision, so a
// search for one says exactly which revision's text the index holds.
type step struct {
	kind       stepKind
	key        int
	name       bool   // stepDeliver: the entity has a name
	inline     bool   // stepDeliver: the entity has inline text besides its name
	offloaded  bool   // stepDeliver, stepOlder: the entity's body is offloaded
	older      uint64 // stepOlder: the revision delivered, at or below the key's
	read       int    // stepReturn: which outstanding read returns, by position
	readFailed bool   // stepReturn: the read failed
}

// keyModel is what the requirement says the index holds for one key.
type keyModel struct {
	live     bool
	revision uint64 // the latest revision applied, delete or write
	expected []string
	words    []string // every word any delivery of the key carried
}

// delivery is one delivered revision: when it was delivered and when its outcome was in (its
// text applied, or its body read returned), -1 while not.
type delivery struct {
	key       string
	revision  uint64
	at        int
	outcomeAt int
}

// completion is one Complete call and the step it came in.
type completion struct {
	key      string
	revision uint64
	at       int
}

// outstanding is a body read the applier asked for and the word its body holds.
type outstanding struct {
	read bodyRead
	body string
}

// history runs steps against an applier and the reference model and checks them after each step.
type history struct {
	f          failer
	a          *applier
	at         int
	revision   uint64
	keys       map[string]*keyModel
	reads      []outstanding
	deliveries []delivery
	calls      []completion
}

func newHistory(f failer) *history {
	h := &history{f: f, keys: map[string]*keyModel{}}
	for _, k := range propKeys {
		h.keys[k] = &keyModel{}
	}
	h.a = newTestApplier(f, h.complete)
	return h
}

// complete is the applier's completion. A revision completes only once the index holds it, so
// when the completed revision is the key's latest, the index must already hold what the model
// expects for it.
func (h *history) complete(key string, revision uint64) {
	h.f.Helper()
	h.calls = append(h.calls, completion{key: key, revision: revision, at: h.at})
	if m := h.keys[key]; revision == m.revision {
		h.checkKey(key, fmt.Sprintf("when revision %d of %s completed", revision, key))
	}
}

// run applies s to the model first, so a completion inside the applier is checked against the
// model's new state, then to the applier, then checks every rule.
func (h *history) run(s step) {
	h.f.Helper()
	h.at++
	key := propKeys[s.key]
	m := h.keys[key]
	word := func(part string) string {
		w := fmt.Sprintf("k%dr%d%s", s.key, h.revision, part)
		m.words = append(m.words, w)
		return w
	}
	switch s.kind {
	case stepDeliver:
		h.revision++
		t := text{offloaded: s.offloaded}
		m.live, m.revision, m.expected = true, h.revision, nil
		if s.name {
			t.name = word("n")
			m.expected = append(m.expected, t.name)
		}
		if s.inline {
			t.inline = word("i")
			m.expected = append(m.expected, t.inline)
		}
		d := delivery{key: key, revision: h.revision, at: h.at, outcomeAt: h.at}
		if s.offloaded {
			d.outcomeAt = -1
		}
		h.deliveries = append(h.deliveries, d)
		read, ok := mustDeliver(h.f, h.a, stateEntry(h.f, key, h.revision, t))
		if ok != s.offloaded || (ok && (read.key != key || read.revision != h.revision)) {
			h.f.Fatalf("delivery of %s at %d (offloaded %t) asked for read %+v, %t", key, h.revision, s.offloaded, read, ok)
		}
		if ok {
			h.reads = append(h.reads, outstanding{read: read, body: word("b")})
		}
	case stepDelete:
		h.revision++
		m.live, m.revision, m.expected = false, h.revision, nil
		h.deliveries = append(h.deliveries, delivery{key: key, revision: h.revision, at: h.at, outcomeAt: h.at})
		mustDeliver(h.f, h.a, deleteEntry(key, h.revision))
	case stepOlder:
		h.deliveries = append(h.deliveries, delivery{key: key, revision: s.older, at: h.at, outcomeAt: h.at})
		t := text{name: fmt.Sprintf("k%do%dn", s.key, h.at), offloaded: s.offloaded}
		m.words = append(m.words, t.name)
		if _, ok := mustDeliver(h.f, h.a, stateEntry(h.f, key, s.older, t)); ok {
			h.f.Fatalf("an older delivery of %s at %d asked for a body read", key, s.older)
		}
	case stepReturn:
		o := h.reads[s.read]
		h.reads = slices.Delete(h.reads, s.read, s.read+1)
		key = o.read.key
		m = h.keys[key]
		if m.live && m.revision == o.read.revision && !s.readFailed {
			m.expected = append(m.expected, o.body)
		}
		for i := range h.deliveries {
			if d := &h.deliveries[i]; d.key == key && d.revision == o.read.revision && d.outcomeAt < 0 {
				d.outcomeAt = h.at
			}
		}
		if s.readFailed {
			h.a.bodyDone(o.read, "", errors.New("store failed"))
		} else {
			h.a.bodyDone(o.read, o.body, nil)
		}
	}
	h.checkKey(key, fmt.Sprintf("after step %d", h.at))
	h.checkSize()
	h.checkCompletions()
}

// checkKey requires every word key ever carried to be found on key exactly when the model
// expects it.
func (h *history) checkKey(key, when string) {
	h.f.Helper()
	m := h.keys[key]
	for _, w := range m.words {
		var want []string
		if m.live && slices.Contains(m.expected, w) {
			want = []string{key}
		}
		if got := holders(h.f, h.a.index, w); !slices.Equal(got, want) {
			h.f.Fatalf("%s: search %q found %v; want %v (model %+v)", when, w, got, want, *m)
		}
	}
}

// checkSize requires the index's entity count and both gauges to match the model.
func (h *history) checkSize() {
	h.f.Helper()
	entities, words := 0, 0
	for _, m := range h.keys {
		if m.live && len(m.expected) > 0 {
			entities++
			words += len(m.expected)
		}
	}
	if n := h.a.index.Len(); n != entities {
		h.f.Fatalf("after step %d the index holds %d entities; want %d", h.at, n, entities)
	}
	if g := testutil.ToFloat64(h.a.metrics.entities); g != float64(entities) {
		h.f.Fatalf("after step %d the entity gauge reads %v; want %d", h.at, g, entities)
	}
	if g := testutil.ToFloat64(h.a.metrics.words); g != float64(words) {
		h.f.Fatalf("after step %d the word gauge reads %v; want %d", h.at, g, words)
	}
}

// checkCompletions requires each delivered revision to be complete exactly when the follower's
// rule says it is: Complete(key, r) completes every revision of key at or below r delivered
// before the call. So a delivered revision must be complete once its own outcome, or a later
// outcome of a revision of its key at or above it, is in, and must not be complete before.
func (h *history) checkCompletions() {
	h.f.Helper()
	for _, d := range h.deliveries {
		want := false
		for _, other := range h.deliveries {
			if other.key == d.key && other.revision >= d.revision && other.outcomeAt >= d.at {
				want = true
				break
			}
		}
		got := slices.ContainsFunc(h.calls, func(c completion) bool {
			return c.key == d.key && c.revision >= d.revision && c.at >= d.at
		})
		if got != want {
			h.f.Fatalf("after step %d revision %d of %s (delivered at step %d) complete = %t; want %t",
				h.at, d.revision, d.key, d.at, got, want)
		}
	}
}

// checkAll requires every key's words to be found exactly as the model expects.
func (h *history) checkAll() {
	h.f.Helper()
	for _, key := range propKeys {
		h.checkKey(key, "at the end")
	}
}

// drawStep draws a step the history can take now: an older delivery only of a live key, a return
// only while a read is out.
func (h *history) drawStep(t *rapid.T) step {
	s := step{key: rapid.IntRange(0, len(propKeys)-1).Draw(t, "key")}
	switch kind := rapid.IntRange(0, 9).Draw(t, "kind"); {
	case kind == 5:
		s.kind = stepDelete
	case kind == 6 && h.keys[propKeys[s.key]].live:
		s.kind = stepOlder
		s.older = rapid.Uint64Range(1, h.keys[propKeys[s.key]].revision).Draw(t, "older")
		s.offloaded = rapid.Bool().Draw(t, "offloaded")
	case kind >= 7 && len(h.reads) > 0:
		s.kind = stepReturn
		s.read = rapid.IntRange(0, len(h.reads)-1).Draw(t, "read")
		s.readFailed = rapid.Bool().Draw(t, "readFailed")
	default:
		s.kind = stepDeliver
		s.name = rapid.Bool().Draw(t, "name")
		s.inline = rapid.Bool().Draw(t, "inline")
		s.offloaded = rapid.Bool().Draw(t, "offloaded")
	}
	return s
}

// TestPropApplyMatchesModel: after every step of a history of deliveries, deletes, older
// deliveries and body reads returning late, out of order or failed, every live key holds its
// latest revision's inline text, plus that revision's body only if its read succeeded; nothing
// else is found; the gauges count what the index holds; and every delivered revision completes
// once its outcome is in, never before, with its text already in the index. The named witnesses
// run first through the same checks; the stale read among them is TestStaleBodyReadDropped's case.
func TestPropApplyMatchesModel(t *testing.T) {
	deliverOffloaded := func(key int) step {
		return step{kind: stepDeliver, key: key, name: true, inline: true, offloaded: true}
	}
	witnesses := []struct {
		name  string
		steps []step
	}{
		{"stale body read dropped", []step{
			deliverOffloaded(0), deliverOffloaded(0),
			{kind: stepReturn, read: 0}, {kind: stepReturn, read: 0},
		}},
		{"read returns after an inline write", []step{
			deliverOffloaded(0), {kind: stepDeliver, key: 0, name: true},
			{kind: stepReturn, read: 0},
		}},
		{"read returns after a delete", []step{
			deliverOffloaded(0), {kind: stepDelete, key: 0}, {kind: stepReturn, read: 0},
		}},
		{"read returns after a delete and a new write", []step{
			deliverOffloaded(0), {kind: stepDelete, key: 0}, deliverOffloaded(0),
			{kind: stepReturn, read: 0}, {kind: stepReturn, read: 0},
		}},
		{"reads return out of order", []step{
			deliverOffloaded(0), deliverOffloaded(1),
			{kind: stepReturn, read: 1}, {kind: stepReturn, read: 0},
		}},
		{"read fails", []step{
			deliverOffloaded(0), {kind: stepReturn, read: 0, readFailed: true},
		}},
		{"older deliveries", []step{
			{kind: stepDeliver, key: 0, name: true}, deliverOffloaded(1),
			{kind: stepOlder, key: 0, older: 1, offloaded: true}, {kind: stepOlder, key: 1, older: 1},
			{kind: stepReturn, read: 0},
		}},
		{"a write with no text", []step{
			{kind: stepDeliver, key: 0, name: true}, {kind: stepDeliver, key: 0},
		}},
	}
	for _, w := range witnesses {
		t.Run(w.name, func(t *testing.T) {
			h := newHistory(t)
			for _, s := range w.steps {
				h.run(s)
			}
			h.checkAll()
		})
	}
	if t.Failed() {
		return // a failing witness is the finding; a generated run would only repeat it
	}

	rapid.Check(t, func(t *rapid.T) {
		h := newHistory(t)
		for range rapid.IntRange(1, 40).Draw(t, "steps") {
			h.run(h.drawStep(t))
		}
		h.checkAll()
	})
}
