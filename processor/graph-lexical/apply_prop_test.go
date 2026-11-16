package graphlexical

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"pgregory.net/rapid"

	"github.com/c360studio/semengine/graph/entitytext"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/internal/revlag"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/storage"
)

// TestStaleBodyReadDropped: a body read for revision R that returns after the same key's R+1 is
// applied is dropped: the index holds R+1's text and never R's body, and the follower's floor
// passes R. R+1 has an offloaded body of its own, under its own reference, so only R's own
// completion can move the floor past R. The floor is a revlag.Watermark, the follower's own, fed as the follower feeds it.
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
	freshRead, ok := deliver(stateEntry(t, pump, r+1, text{name: "newname", inline: "newinline", offloaded: true,
		bodyKey: "fresh"}))
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
	stepRepair                  // a repair pass asks again for the bodies left unread or with no store
)

// refChange is how an offloaded delivery's reference relates to the last one the key named.
type refChange int

const (
	refNewKey  refChange = iota // a reference at a key no delivery named before
	refKeep                     // the last reference the key named, whole; a new key if it named none
	refNewSize                  // the last reference but for its size
	refNewType                  // the last reference but for its content type
)

// outcomeErrs are the errors a failed read returns, by the state it leaves the body in.
var outcomeErrs = map[bodyState]error{
	bodyUnread:  errors.New("store failed"),
	bodyMissing: fmt.Errorf("open: %w", storage.ErrObjectNotFound),
	bodyNoStore: fmt.Errorf("%w: %q", entitytext.ErrNoStore, "elsewhere"),
}

// step is one step of a history. A delivery's new words are unique to its key and revision, and
// each distinct reference's body holds a word of its own, so a search for one says exactly which
// text and which body the index holds.
type step struct {
	kind      stepKind
	key       int
	name      bool      // stepDeliver: the entity has a name
	inline    bool      // stepDeliver: the entity has inline text besides its name
	keepText  bool      // stepDeliver: the text is the key's latest write's, while the key is live
	offloaded bool      // stepDeliver, stepOlder: the entity's body is offloaded
	ref       refChange // stepDeliver with offloaded: which reference it names
	older     uint64    // stepOlder: the revision delivered, at or below the key's
	read      int       // stepReturn: which outstanding read returns, by position
	outcome   bodyState // stepReturn: bodyNone for a body read, else the state its failure leaves
}

// keyModel is what the requirement says the index holds for one key.
type keyModel struct {
	live         bool
	revision     uint64 // the latest revision applied, delete or write
	name, inline string // the latest write's text
	// ref is the latest revision's reference, nil for none; body is where that reference's body
	// stands, bodyNone once a read of it succeeded since the key last named another reference or
	// changed its text.
	ref     *message.StorageReference
	body    bodyState
	lastRef *message.StorageReference // the last reference any revision of the key named
	words   []string                  // every word any delivery of the key carried
}

// awaitsRead reports whether the latest revision's body still takes a read: one not yet read,
// or left unread or with no store.
func (m *keyModel) awaitsRead() bool {
	return m.body == bodyQueued || m.body == bodyUnread || m.body == bodyNoStore
}

// completion is one completed revision of a key and the step it came in: a Complete call, or an
// outcome the requirement says is in.
type completion struct {
	key      string
	revision uint64
	at       int
}

// covers reports whether any of cs completes key's revision delivered at step at: the follower
// completes every revision of a key at or below a completed one that was delivered before it.
func covers(cs []completion, key string, revision uint64, at int) bool {
	return slices.ContainsFunc(cs, func(c completion) bool {
		return c.key == key && c.revision >= revision && c.at >= at
	})
}

// history runs steps against an applier and the reference model and checks them after each step.
type history struct {
	f          failer
	a          *applier
	at         int
	revision   uint64
	keys       map[string]*keyModel
	bodyWords  map[message.StorageReference]string // the word each reference's body holds
	reads      []bodyRead                          // the body reads asked for that have not returned
	deliveries []completion                        // every delivered revision, at the step it was delivered
	outcomes   []completion                        // the completions the requirement makes due
	calls      []completion                        // the Complete calls the applier made
}

func newHistory(f failer) *history {
	h := &history{f: f, keys: map[string]*keyModel{}, bodyWords: map[message.StorageReference]string{}}
	for _, k := range propKeys {
		h.keys[k] = &keyModel{}
	}
	h.a = newTestApplier(f, h.complete)
	return h
}

// expected is every word a search must find key by now.
func (h *history) expected(m *keyModel) []string {
	if !m.live {
		return nil
	}
	var want []string
	for _, w := range []string{m.name, m.inline} {
		if w != "" {
			want = append(want, w)
		}
	}
	if m.ref != nil && m.body == bodyNone {
		want = append(want, h.bodyWords[*m.ref])
	}
	return want
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

// due records that the requirement makes key's revision complete at this step.
func (h *history) due(key string, revision uint64) {
	h.outcomes = append(h.outcomes, completion{key: key, revision: revision, at: h.at})
}

// nextRef is the reference an offloaded delivery of key s.key names under s.ref; a reference
// not named before gets a body word of its own.
func (h *history) nextRef(s step, m *keyModel) message.StorageReference {
	fresh := message.StorageReference{StorageInstance: "objectstore", Key: fmt.Sprintf("k%dr%dref", s.key, h.revision)}
	var ref message.StorageReference
	switch {
	case m.lastRef == nil || s.ref == refNewKey:
		ref = fresh
	case s.ref == refNewSize:
		ref = *m.lastRef
		ref.Size = int64(h.revision)
	case s.ref == refNewType:
		ref = *m.lastRef
		ref.ContentType = fmt.Sprintf("text/r%d", h.revision)
	default:
		ref = *m.lastRef
	}
	if _, ok := h.bodyWords[ref]; !ok {
		w := fmt.Sprintf("k%dr%db", s.key, h.revision)
		m.words = append(m.words, w)
		h.bodyWords[ref] = w
	}
	return ref
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
		h.deliver(s, key, m, word)
	case stepDelete:
		h.revision++
		m.live, m.revision, m.name, m.inline, m.ref, m.body = false, h.revision, "", "", nil, bodyNone
		h.deliveries = append(h.deliveries, completion{key: key, revision: h.revision, at: h.at})
		h.due(key, h.revision)
		mustDeliver(h.f, h.a, deleteEntry(key, h.revision))
	case stepOlder:
		h.deliveries = append(h.deliveries, completion{key: key, revision: s.older, at: h.at})
		h.due(key, s.older)
		t := text{name: fmt.Sprintf("k%do%dn", s.key, h.at), offloaded: s.offloaded,
			bodyKey: fmt.Sprintf("k%do%dref", s.key, h.at)}
		m.words = append(m.words, t.name)
		if _, ok := mustDeliver(h.f, h.a, stateEntry(h.f, key, s.older, t)); ok {
			h.f.Fatalf("an older delivery of %s at %d asked for a body read", key, s.older)
		}
	case stepReturn:
		read := h.reads[s.read]
		h.reads = slices.Delete(h.reads, s.read, s.read+1)
		key = read.key
		m = h.keys[key]
		// A read serves its reference: it applies while the latest revision names that reference
		// and its body is not in the index, and completes that revision; otherwise it is dropped and
		// completes the revision it was asked for.
		if m.live && m.ref != nil && *m.ref == read.ref && m.awaitsRead() {
			m.body = s.outcome
			h.due(key, m.revision)
		} else {
			h.due(key, read.revision)
		}
		if s.outcome == bodyNone {
			h.a.bodyDone(read, h.bodyWords[read.ref], nil)
		} else {
			h.a.bodyDone(read, "", outcomeErrs[s.outcome])
		}
	case stepRepair:
		var want, got []string
		for _, k := range propKeys {
			if m := h.keys[k]; m.live && (m.body == bodyUnread || m.body == bodyNoStore) {
				want = append(want, fmt.Sprintf("%s@%d %+v", k, m.revision, *m.ref))
			}
		}
		for _, read := range h.a.retries() {
			got = append(got, fmt.Sprintf("%s@%d %+v", read.key, read.revision, read.ref))
			h.reads = append(h.reads, read)
		}
		if slices.Sort(got); !slices.Equal(got, want) {
			h.f.Fatalf("after step %d a repair pass asked for %v; want %v", h.at, got, want)
		}
	}
	h.checkKey(key, fmt.Sprintf("after step %d", h.at))
	h.checkSize()
	h.checkCompletions()
}

// deliver runs a write step. The requirement: a revision whose reference and text equal the
// latest's changes nothing and asks for no read; one that keeps a reference whose read is still
// out puts its text and waits on that read; any other with a reference puts its text and asks
// for a read of its reference; one with no reference puts its text and is complete at once. A
// revision completes at once unless a read of its reference is out.
func (h *history) deliver(s step, key string, m *keyModel, word func(string) string) {
	h.f.Helper()
	h.revision++
	t := text{offloaded: s.offloaded}
	if s.keepText && m.live {
		t.name, t.inline = m.name, m.inline
	} else {
		if s.name {
			t.name = word("n")
		}
		if s.inline {
			t.inline = word("i")
		}
	}
	h.deliveries = append(h.deliveries, completion{key: key, revision: h.revision, at: h.at})
	var ref message.StorageReference
	wantRead := false
	if s.offloaded {
		ref = h.nextRef(s, m)
		t.instance, t.bodyKey, t.contentType, t.size = ref.StorageInstance, ref.Key, ref.ContentType, ref.Size
		sameRef := m.live && m.ref != nil && *m.ref == ref
		sameText := m.live && m.name == t.name && m.inline == t.inline
		if !sameRef || !sameText && m.body != bodyQueued {
			wantRead = true
			m.ref, m.body = &ref, bodyQueued
		}
		m.lastRef = &ref
		if m.body != bodyQueued {
			h.due(key, h.revision)
		}
	} else {
		m.ref, m.body = nil, bodyNone
		h.due(key, h.revision)
	}
	m.live, m.revision, m.name, m.inline = true, h.revision, t.name, t.inline
	read, ok := mustDeliver(h.f, h.a, stateEntry(h.f, key, h.revision, t))
	if ok != wantRead || (ok && (read.key != key || read.revision != h.revision || read.ref != ref)) {
		h.f.Fatalf("delivery of %s at %d asked for read %+v, %t; want a read %t of %+v", key, h.revision, read, ok,
			wantRead, ref)
	}
	if ok {
		h.reads = append(h.reads, read)
	}
}

// checkKey requires every word key ever carried to be found on key exactly when the model
// expects it.
func (h *history) checkKey(key, when string) {
	h.f.Helper()
	m := h.keys[key]
	expected := h.expected(m)
	for _, w := range m.words {
		var want []string
		if slices.Contains(expected, w) {
			want = []string{key}
		}
		if got := holders(h.f, h.a.index, w); !slices.Equal(got, want) {
			h.f.Fatalf("%s: search %q found %v; want %v (model %+v)", when, w, got, want, *m)
		}
	}
}

// checkSize requires the index's entity count, the size gauges, the body gauges and the
// failures reported to match the model.
func (h *history) checkSize() {
	h.f.Helper()
	entities, words := 0, 0
	var bodies [len(bodyStateNames)]int
	for _, m := range h.keys {
		if n := len(h.expected(m)); n > 0 {
			entities++
			words += n
		}
		if m.live {
			bodies[m.body]++
		}
	}
	for s := bodyQueued; s < bodyState(len(bodyStateNames)); s++ {
		if g := testutil.ToFloat64(h.a.metrics.bodies.WithLabelValues(bodyStateNames[s])); g != float64(bodies[s]) {
			h.f.Fatalf("after step %d bodies_not_indexed{state=%q} reads %v; want %d", h.at, bodyStateNames[s], g, bodies[s])
		}
	}
	if n, _ := h.a.failed(); n != int64(bodies[bodyUnread]) {
		h.f.Fatalf("after step %d Failed reports %d; want %d unread", h.at, n, bodies[bodyUnread])
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

// checkCompletions requires each delivered revision to be complete exactly when the model says
// its outcome, or the outcome of a revision of its key at or above it delivered no earlier, is
// in: never before.
func (h *history) checkCompletions() {
	h.f.Helper()
	for _, d := range h.deliveries {
		want := covers(h.outcomes, d.key, d.revision, d.at)
		if got := covers(h.calls, d.key, d.revision, d.at); got != want {
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

// drawStep draws a step the history can take now: an older delivery only of a key with a revision
// applied, live or deleted, a return only while a read is out.
func (h *history) drawStep(t *rapid.T) step {
	s := step{key: rapid.IntRange(0, len(propKeys)-1).Draw(t, "key")}
	switch kind := rapid.IntRange(0, 9).Draw(t, "kind"); {
	case kind == 5:
		s.kind = stepDelete
	case kind == 6 && h.keys[propKeys[s.key]].revision > 0:
		s.kind = stepOlder
		s.older = rapid.Uint64Range(1, h.keys[propKeys[s.key]].revision).Draw(t, "older")
		s.offloaded = rapid.Bool().Draw(t, "offloaded")
	case kind == 7 && rapid.IntRange(0, 2).Draw(t, "repair") == 0:
		s.kind = stepRepair
	case kind >= 7 && len(h.reads) > 0:
		s.kind = stepReturn
		s.read = rapid.IntRange(0, len(h.reads)-1).Draw(t, "read")
		s.outcome = rapid.SampledFrom([]bodyState{bodyNone, bodyNone, bodyUnread, bodyMissing, bodyNoStore}).
			Draw(t, "outcome")
	default:
		s.kind = stepDeliver
		s.name = rapid.Bool().Draw(t, "name")
		s.inline = rapid.Bool().Draw(t, "inline")
		s.keepText = rapid.Bool().Draw(t, "keepText")
		s.offloaded = rapid.Bool().Draw(t, "offloaded")
		if s.offloaded {
			s.ref = rapid.SampledFrom([]refChange{refKeep, refKeep, refKeep, refNewKey, refNewSize, refNewType}).
				Draw(t, "ref")
		}
	}
	return s
}

// TestPropApplyMatchesModel: after every step of a history of deliveries that keep or change a
// key's text and reference, deletes, older deliveries and body reads returning late, out of
// order or failed, every live key holds its latest revision's inline text, plus the body of that
// revision's reference only if a read of that reference succeeded since the key last named
// another reference or changed its text; nothing else is found; a read is asked for exactly
// when the requirement says; the gauges count what the index holds; and every delivered
// revision completes once its outcome is in, never before, with its text already in the index.
// The named witnesses run first through the same checks; the stale read among them is
// TestStaleBodyReadDropped's case.
func TestPropApplyMatchesModel(t *testing.T) {
	deliverOffloaded := func(key int) step {
		return step{kind: stepDeliver, key: key, name: true, inline: true, offloaded: true}
	}
	// sameAgain is a revision of key 0 that changes no text and keeps the reference: only values
	// that are not text changed, as telemetry does.
	sameAgain := step{kind: stepDeliver, name: true, inline: true, keepText: true, offloaded: true, ref: refKeep}
	newTextSameRef := step{kind: stepDeliver, name: true, inline: true, offloaded: true, ref: refKeep}
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
			deliverOffloaded(0), {kind: stepReturn, read: 0, outcome: bodyUnread},
		}},
		{"unread body read at the repair pass", []step{
			deliverOffloaded(0), {kind: stepReturn, read: 0, outcome: bodyUnread},
			{kind: stepRepair}, {kind: stepReturn, read: 0},
		}},
		{"no store, then a store at the repair pass", []step{
			deliverOffloaded(0), {kind: stepReturn, read: 0, outcome: bodyNoStore},
			{kind: stepRepair}, {kind: stepReturn, read: 0},
		}},
		{"missing body not asked for again", []step{
			deliverOffloaded(0), {kind: stepReturn, read: 0, outcome: bodyMissing}, {kind: stepRepair},
		}},
		{"repair read returns after a newer delivery", []step{
			deliverOffloaded(0), {kind: stepReturn, read: 0, outcome: bodyUnread},
			{kind: stepRepair}, deliverOffloaded(0), {kind: stepReturn, read: 0}, {kind: stepReturn, read: 0},
		}},
		{"older deliveries", []step{
			{kind: stepDeliver, key: 0, name: true}, deliverOffloaded(1),
			{kind: stepOlder, key: 0, older: 1, offloaded: true}, {kind: stepOlder, key: 1, older: 1},
			{kind: stepReturn, read: 0},
		}},
		{"older delivery after a delete", []step{
			{kind: stepDeliver, key: 0, name: true}, {kind: stepDelete, key: 0},
			{kind: stepOlder, key: 0, older: 1}, {kind: stepOlder, key: 0, older: 2, offloaded: true},
		}},
		{"a write with no text", []step{
			{kind: stepDeliver, key: 0, name: true}, {kind: stepDeliver, key: 0},
		}},
		{"a non-text revision after a read keeps the body and reads nothing", []step{
			deliverOffloaded(0), {kind: stepReturn, read: 0}, sameAgain, sameAgain,
		}},
		{"three same-reference revisions while the first read is out", []step{
			deliverOffloaded(0), sameAgain, sameAgain, sameAgain, {kind: stepReturn, read: 0},
		}},
		{"an inline-text change with the same reference reads once", []step{
			deliverOffloaded(0), {kind: stepReturn, read: 0}, newTextSameRef, {kind: stepReturn, read: 0},
		}},
		{"a reference change drops the old body at once", []step{
			deliverOffloaded(0), {kind: stepReturn, read: 0},
			{kind: stepDeliver, name: true, inline: true, keepText: true, offloaded: true, ref: refNewKey},
			{kind: stepReturn, read: 0},
		}},
		{"a missing body is not read again by a same-text revision", []step{
			deliverOffloaded(0), {kind: stepReturn, read: 0, outcome: bodyMissing}, sameAgain, {kind: stepRepair},
		}},
		{"an unread body is not read again by a same-text revision, only by the repair pass", []step{
			deliverOffloaded(0), {kind: stepReturn, read: 0, outcome: bodyUnread}, sameAgain,
			{kind: stepRepair}, {kind: stepReturn, read: 0},
		}},
		{"a late read joins the latest inline text", []step{
			deliverOffloaded(0), newTextSameRef, {kind: stepReturn, read: 0},
		}},
		{"a reference equal but for its size or content type is read", []step{
			deliverOffloaded(0), {kind: stepReturn, read: 0},
			{kind: stepDeliver, name: true, inline: true, keepText: true, offloaded: true, ref: refNewSize},
			{kind: stepReturn, read: 0},
			{kind: stepDeliver, name: true, inline: true, keepText: true, offloaded: true, ref: refNewType},
			{kind: stepReturn, read: 0},
		}},
		{"a revision waiting on a read that a reference change drops", []step{
			deliverOffloaded(0), sameAgain,
			{kind: stepDeliver, name: true, inline: true, keepText: true, offloaded: true, ref: refNewKey},
			{kind: stepReturn, read: 0}, {kind: stepReturn, read: 0},
		}},
		{"a reference named again after a delete is served by the read still out", []step{
			deliverOffloaded(0), {kind: stepDelete, key: 0},
			{kind: stepDeliver, name: true, inline: true, offloaded: true, ref: refKeep},
			{kind: stepReturn, read: 0}, {kind: stepReturn, read: 0},
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
