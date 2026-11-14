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

// bodyRead is an offloaded body to read: the entity's key, the revision whose state names the
// body, and where the body is.
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
// the follower's goroutine, applies each delivered value, and bodyDone applies a body read for a
// revision once the read returns. The order rule between them is the revision: a delivery at or
// below the revision held for its key is ignored, and a body read for revision R is indexed only
// if the key is still at R. Every delivered revision is completed through complete once its
// outcome is in, after the index holds it, so the follower's floor never passes text the index
// does not hold.
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
	// waiting is what the offloaded body still to be read for revision is joined to; nil when no
	// read is wanted.
	waiting *pending
}

// pending is what an offloaded body still to be read needs, and no more of the entity's state:
// the document of the entity's inline text, which Rule.WithBody joins the body to, and where the
// body is.
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
// nothing and is completed. A delete removes the entity and is completed. A write puts the
// entity's inline text and is completed, unless its state names an offloaded body: then the body
// is queued, deliver returns the read to make, and the revision completes when bodyDone takes the
// read's outcome.
//
// A value that is not a canonical entity state is refused with the *graph.StateContractError
// the decoder returned, nothing is applied, and the revision is not completed: state the index
// cannot read is the follower's reset to latch, never a skip the floor passes.
func (a *applier) deliver(e follower.Entry) (bodyRead, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if h, ok := a.held[e.Key]; ok && e.Revision <= h.revision {
		a.complete(e.Key, e.Revision)
		return bodyRead{}, false, nil
	}
	if e.Deleted {
		a.index.Delete(e.Key)
		a.remove(e.Key, e.Revision)
		a.report()
		a.complete(e.Key, e.Revision)
		return bodyRead{}, false, nil
	}
	var state graph.EntityState
	if err := graph.UnmarshalEntityState(e.Key, e.Value, &state); err != nil {
		return bodyRead{}, false, err
	}
	inline := a.rule.Document(&state, "")
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
	return bodyRead{key: e.Key, revision: e.Revision, ref: *state.StorageRef}, true, nil
}

// wanted reports whether read is still to be made: its key is still at read's revision and its
// body still waits to be read. A read no longer wanted has its revision completed, as a dropped
// read's is.
func (a *applier) wanted(read bodyRead) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if h, ok := a.held[read.key]; ok && h.revision == read.revision && h.waiting != nil {
		return true
	}
	a.complete(read.key, read.revision)
	return false
}

// bodyDone takes the outcome of read: the body, or the error that kept it from being read. The
// outcome applies only if the key is still at read's revision; a key that moved on or was
// deleted drops it. A body read joins its state's text; a failed read leaves the inline text and
// puts the body in the state readState names, keeping what a retry needs unless the body is
// missing. The read's revision completes either way.
func (a *applier) bodyDone(read bodyRead, body string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if h, ok := a.held[read.key]; ok && h.revision == read.revision && h.waiting != nil {
		switch s := readState(err); s {
		case bodyNone:
			// put refuses only an ID that is not canonical, and this key's was accepted when its
			// inline text was put.
			_, _ = a.put(read.key, read.revision, a.rule.WithBody(h.waiting.inline, body))
			a.setBody(h, bodyNone, nil)
		case bodyMissing:
			a.setBody(h, s, nil)
			a.logFirst(&a.logged.missing, "offloaded body missing from its store; the entity is indexed without it",
				read, err)
		case bodyNoStore:
			a.setBody(h, s, h.waiting)
			a.logFirst(&a.logged.noStore, "no store registered for an offloaded body; looked up again each repair pass",
				read, err)
		default:
			a.setBody(h, s, h.waiting)
		}
	}
	a.complete(read.key, read.revision)
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
			reads = append(reads, bodyRead{key: key, revision: h.revision, ref: h.waiting.ref})
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

// setBody puts h's body in state s, waiting on waiting, and reports it.
func (a *applier) setBody(h *held, s bodyState, waiting *pending) {
	if h.body != bodyNone {
		a.bodies[h.body]--
	}
	if s != bodyNone {
		a.bodies[s]++
	}
	h.body, h.waiting = s, waiting
	a.report()
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
