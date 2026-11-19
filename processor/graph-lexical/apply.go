package graphlexical

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/entitytext"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/graph/lexical"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/storage"
)

// bodyRead is an offloaded body to read: the entity's key, the revision that asked for the read,
// and where the body is.
type bodyRead struct {
	key      string
	revision uint64
	ref      message.StorageReference
}

// bodyState is where an entity's offloaded body stands while the index does not hold it.
type bodyState int

const (
	bodyNone    bodyState = iota // no offloaded body, or it is in the index
	bodyQueued                   // not yet read
	bodyUnread                   // a read failed, other than with a missing object, or its store was skipped
	bodyMissing                  // the store has no such object
	bodyNoStore                  // no store is registered under the body's storage instance
)

// bodyStateNames are the states' names on the bodies_not_indexed gauge.
var bodyStateNames = [...]string{
	bodyQueued:  "queued",
	bodyUnread:  "unread",
	bodyMissing: "missing",
	bodyNoStore: "no_store",
}

// failureBodyUnread is the failure reason an unread body is reported under.
const failureBodyUnread = "body_unread"

// readState is the state a body read's outcome leaves the body in: in the index when err is nil,
// missing for a missing object, no store for an unregistered instance, and unread for any other
// failure, whatever its cause.
func readState(err error) bodyState {
	switch {
	case err == nil:
		return bodyNone
	case errors.Is(err, storage.ErrObjectNotFound):
		return bodyMissing
	case errors.Is(err, entitytext.ErrNoStore):
		return bodyNoStore
	default:
		return bodyUnread
	}
}

// applier keeps the word index in step with ENTITY_STATES. Two paths write one key: deliver, on
// the follower's goroutine, applies each delivered value, and bodyDone applies a body read once
// it returns. A delivery at or below the revision held for its key is ignored. A body read serves
// its reference, not the revision that asked for it: it is indexed if the key's latest revision
// still names that reference and its body still awaits a read (queued, unread or no store),
// joined to that revision's inline text. So an entity whose other values change often while its reference stays, such as
// one carrying telemetry, keeps its body's words and costs no read per update. Every delivered
// revision is completed through complete once its outcome is in, after the index holds it, so
// the follower's floor never passes text the index does not hold.
type applier struct {
	rule     entitytext.Rule
	maxLen   int // the rule's cap, named when a body is first cut
	index    *lexical.Index
	complete func(key string, revision uint64)
	metrics  *lexicalMetrics
	logger   *slog.Logger

	// mu orders the two writers: a revision check and the write it allows happen as one step.
	mu     sync.Mutex
	held   map[string]*held // by key, every key delivered since start; a deleted key keeps only its revision
	words  int              // the words the index holds, every occurrence counted
	bodies [len(bodyStateNames)]int
	cuts   int // held entities whose body is cut at the cap
	// logged records the first cut, missing body and unregistered store since start, each logged once.
	logged struct{ cut, missing, noStore bool }
}

// held is what the applier knows of one key.
type held struct {
	revision uint64
	words    int
	cut      bool
	body     bodyState
	// offloaded is the latest revision's inline text and reference when it names an offloaded
	// body, whatever body says of it, and nil when it names none. It stays after the body is read,
	// so a later revision with the same text and reference can be told apart from a change.
	offloaded *pending
}

// pending is what is kept of an entity whose body is offloaded, and no more of its state: the
// document of its inline text, which Rule.WithBody joins the body to, and where the body is. The
// body itself is not kept: the index holds its words.
type pending struct {
	inline entitytext.Document
	ref    message.StorageReference
}

// newApplier returns an applier over an empty index that completes revisions through complete
// and reports the index's size on metrics. maxLen is rule's cap.
func newApplier(rule entitytext.Rule, maxLen int, complete func(key string, revision uint64),
	metrics *lexicalMetrics, logger *slog.Logger) *applier {
	a := &applier{
		rule:     rule,
		maxLen:   maxLen,
		index:    lexical.NewIndex(),
		complete: complete,
		metrics:  metrics,
		logger:   logger,
		held:     make(map[string]*held),
	}
	a.report()
	return a
}

// deliver applies one delivered entry. A delivery at or below the key's held revision changes
// nothing and is completed. A delete removes the entity and is completed. A write with no
// offloaded body puts the entity's inline text and is completed.
//
// A write that names an offloaded body is compared with what is held, the reference as a whole
// value. The same inline text and reference change nothing in the index and ask for no read: the
// key moves to the new revision, which completes at once unless the body is queued, that is a
// read deliver asked for is still queued or out, whose outcome then completes it. A changed
// inline text under the same reference, while the body is queued, puts the new inline text and
// waits on that read. A repair pass's read is not counted: while the body is unread or has no
// store, a changed inline text asks for a read of its own. Anything else puts the inline text,
// queues the body, returns the read to make, and completes when bodyDone takes the read's
// outcome. A reference names fixed content, so content replaced behind an unchanged
// reference is read again only when the inline text or reference changes, or at start.
//
// A value that is not a canonical entity state is refused with the *graph.StateContractError
// the decoder returned, nothing is applied, and the revision is not completed: the caller
// reports the damage and then calls removeDamaged, so the text goes only once it is counted.
func (a *applier) deliver(e follower.Entry) (bodyRead, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if h, ok := a.held[e.Key]; ok && e.Revision <= h.revision {
		a.complete(e.Key, e.Revision)
		return bodyRead{}, false, nil
	}
	if e.Deleted {
		a.drop(e.Key, e.Revision)
		return bodyRead{}, false, nil
	}
	var state graph.EntityState
	if err := graph.UnmarshalEntityState(e.Key, e.Value, &state); err != nil {
		return bodyRead{}, false, err
	}
	inline := a.rule.Document(&state, "")
	prev := a.held[e.Key]
	sameRef := prev != nil && prev.offloaded != nil && state.StorageRef != nil &&
		prev.offloaded.ref == *state.StorageRef
	if sameRef && prev.offloaded.inline == inline {
		prev.revision = e.Revision
		if prev.body != bodyQueued {
			a.complete(e.Key, e.Revision)
		}
		return bodyRead{}, false, nil
	}
	readOut := sameRef && prev.body == bodyQueued
	h, err := a.put(e.Key, e.Revision, inline)
	if err != nil {
		return bodyRead{}, false, err
	}
	if state.StorageRef == nil {
		a.setBody(h, bodyNone, nil)
		a.complete(e.Key, e.Revision)
		return bodyRead{}, false, nil
	}
	a.setBody(h, bodyQueued, &pending{inline: inline, ref: *state.StorageRef})
	if readOut {
		return bodyRead{}, false, nil
	}
	return bodyRead{key: e.Key, revision: e.Revision, ref: *state.StorageRef}, true, nil
}

// serves returns what is held for read's key and whether the key still waits on read: its latest
// revision names read's reference and that body still awaits a read (queued, unread or no store).
// The caller holds mu.
func (a *applier) serves(read bodyRead) (*held, bool) {
	h, ok := a.held[read.key]
	if !ok || h.offloaded == nil || h.offloaded.ref != read.ref {
		return nil, false
	}
	return h, h.body == bodyQueued || h.body == bodyUnread || h.body == bodyNoStore
}

// wanted reports whether read is still to be made: its key's latest revision still names read's
// reference and that body still awaits a read (queued, unread or no store). A read no longer wanted has the revision that
// asked for it completed, as a dropped read's is.
func (a *applier) wanted(read bodyRead) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.serves(read); ok {
		return true
	}
	a.complete(read.key, read.revision)
	return false
}

// bodyDone takes the outcome of read: the body, or the error that kept it from being read. The
// outcome applies only if the key's latest revision still names read's reference and that body
// still awaits a read (queued, unread or no store), whichever revision asked for the read; then it completes that latest
// revision, and with it every earlier one waiting on the read. Otherwise the read is dropped and
// completes the revision that asked for it. A body read is joined to the latest inline text; a
// failed read leaves the inline text and puts the body in the state readState names.
func (a *applier) bodyDone(read bodyRead, body string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	h, ok := a.serves(read)
	if !ok {
		a.complete(read.key, read.revision)
		return
	}
	switch s := readState(err); s {
	case bodyNone:
		// put refuses only an ID that is not canonical, and this key's was accepted when its
		// inline text was put.
		_, _ = a.put(read.key, h.revision, a.rule.WithBody(h.offloaded.inline, body))
		a.setBody(h, bodyNone, h.offloaded)
	case bodyMissing:
		a.setBody(h, s, h.offloaded)
		a.logFirst(&a.logged.missing, "offloaded body missing from its store; the entity is indexed without it",
			read, err)
	case bodyNoStore:
		a.setBody(h, s, h.offloaded)
		a.logFirst(&a.logged.noStore, "no store registered for an offloaded body; looked up again each repair pass",
			read, err)
	default:
		a.setBody(h, s, h.offloaded)
	}
	a.complete(read.key, h.revision)
}

// retries returns a read, at the held revision, of every body left unread or with no store.
func (a *applier) retries() []bodyRead {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.bodies[bodyUnread]+a.bodies[bodyNoStore] == 0 {
		return nil
	}
	var reads []bodyRead
	for key, h := range a.held {
		if h.body == bodyUnread || h.body == bodyNoStore {
			reads = append(reads, bodyRead{key: key, revision: h.revision, ref: h.offloaded.ref})
		}
	}
	return reads
}

// failed is the follower's Failed: the bodies left unread, under one reason.
func (a *applier) failed() (int64, map[string]uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := a.bodies[bodyUnread]
	if n == 0 {
		return 0, nil
	}
	return int64(n), map[string]uint64{failureBodyUnread: uint64(n)}
}

// notIndexed is the number of held entities whose offloaded body the index does not hold, in any
// state.
func (a *applier) notIndexed() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, count := range a.bodies {
		n += count
	}
	return n
}

// put replaces key's text with doc at revision and returns what is held for key; the body's state
// is the caller's to set. A refused ID is returned as the state contract's noncanonical-ID error,
// with nothing changed.
func (a *applier) put(key string, revision uint64, doc entitytext.Document) (*held, error) {
	if err := a.index.Put(key, lexical.Document{Name: doc.Name, Body: doc.Body}); err != nil {
		return nil, &graph.StateContractError{Reason: graph.GraphStateReasonNoncanonicalEntityID, EntityID: key, Err: err}
	}
	words := len(lexical.Tokens(doc.Name)) + len(lexical.Tokens(doc.Body))
	h, ok := a.held[key]
	if !ok {
		h = &held{}
		a.held[key] = h
	}
	a.words += words - h.words
	switch {
	case doc.Cut && !h.cut:
		a.cuts++
	case !doc.Cut && h.cut:
		a.cuts--
	}
	if doc.Cut && !a.logged.cut {
		a.logged.cut = true
		a.logger.Warn("entity text cut at max_text_len; words past it are not indexed",
			slog.String("entity", key), slog.Int("max_text_len", a.maxLen))
	}
	h.revision, h.words, h.cut = revision, words, doc.Cut
	a.report()
	return h, nil
}

// setBody puts h's body in state s, with offloaded the latest revision's inline text and
// reference, and reports it.
func (a *applier) setBody(h *held, s bodyState, offloaded *pending) {
	if h.body != bodyNone {
		a.bodies[h.body]--
	}
	if s != bodyNone {
		a.bodies[s]++
	}
	h.body, h.offloaded = s, offloaded
	a.report()
}

// removeDamaged deletes a damaged entity's text and completes revision, as a delete does. The
// caller has reported the entity damaged first.
func (a *applier) removeDamaged(key string, revision uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.drop(key, revision)
}

// drop deletes key's text and completes revision, under mu.
func (a *applier) drop(key string, revision uint64) {
	a.index.Delete(key)
	a.remove(key, revision)
	a.report()
	a.complete(key, revision)
}

// remove drops what is held for key but revision, the delete's, so a delivery of key at or below
// the delete is still ignored. Each deleted key keeps that one revision until restart.
func (a *applier) remove(key string, revision uint64) {
	h, ok := a.held[key]
	if !ok {
		h = &held{}
		a.held[key] = h
	}
	if h.body != bodyNone {
		a.bodies[h.body]--
	}
	a.words -= h.words
	if h.cut {
		a.cuts--
	}
	*h = held{revision: revision}
}

// logFirst logs msg at warning level for read's entity, once since start: *done records it.
func (a *applier) logFirst(done *bool, msg string, read bodyRead, err error) {
	if *done {
		return
	}
	*done = true
	a.logger.Warn(msg, slog.String("entity", read.key), slog.String("storage_instance", read.ref.StorageInstance),
		slog.Any("error", err))
}

// report sets the gauges to what the index holds and the bodies it does not.
func (a *applier) report() {
	a.metrics.entities.Set(float64(a.index.Len()))
	a.metrics.words.Set(float64(a.words))
	a.metrics.cut.Set(float64(a.cuts))
	for s, name := range bodyStateNames {
		if name != "" {
			a.metrics.bodies.WithLabelValues(name).Set(float64(a.bodies[s]))
		}
	}
}
