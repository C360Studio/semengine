package graphlexical

import (
	"sync"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/entitytext"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/graph/lexical"
	"github.com/c360studio/semengine/message"
)

// bodyRead is an offloaded body to read: the entity's key, the revision whose state names the
// body, and where the body is.
type bodyRead struct {
	key      string
	revision uint64
	ref      message.StorageReference
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
	index    *lexical.Index
	complete func(key string, revision uint64)
	metrics  *lexicalMetrics

	// mu orders the two writers: a revision check and the write it allows happen as one step.
	mu    sync.Mutex
	held  map[string]*held // by key, every entity written and not deleted since start
	words int              // the words the index holds, every occurrence counted
}

// held is what the applier knows of one key.
type held struct {
	revision uint64
	words    int
	// waiting is the state whose offloaded body is being read for revision, so the body joins
	// that state's text when it returns; nil when no read is out for revision.
	waiting *graph.EntityState
}

// newApplier returns an applier over an empty index that completes revisions through complete
// and reports the index's size on metrics.
func newApplier(rule entitytext.Rule, complete func(key string, revision uint64), metrics *lexicalMetrics) *applier {
	a := &applier{
		rule:     rule,
		index:    lexical.NewIndex(),
		complete: complete,
		metrics:  metrics,
		held:     make(map[string]*held),
	}
	a.report()
	return a
}

// deliver applies one delivered entry. A delivery at or below the key's held revision changes
// nothing and is completed. A delete removes the entity and is completed. A write puts the
// entity's inline text and is completed, unless its state names an offloaded body: then deliver
// returns the read to make, and the revision completes when bodyDone takes the read's outcome.
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
		a.forget(e.Key)
		a.report()
		a.complete(e.Key, e.Revision)
		return bodyRead{}, false, nil
	}
	var state graph.EntityState
	if err := graph.UnmarshalEntityState(e.Key, e.Value, &state); err != nil {
		return bodyRead{}, false, err
	}
	h, err := a.put(e.Key, e.Revision, a.rule.Document(&state, ""))
	if err != nil {
		return bodyRead{}, false, err
	}
	if state.StorageRef == nil {
		a.complete(e.Key, e.Revision)
		return bodyRead{}, false, nil
	}
	h.waiting = &state
	return bodyRead{key: e.Key, revision: e.Revision, ref: *state.StorageRef}, true, nil
}

// bodyDone takes the outcome of read: the body, or the error that kept it from being read. The
// body joins its state's text only if the key is still at read's revision; a key that moved on
// or was deleted drops it. A failed read leaves the inline text. The read's revision completes
// either way.
func (a *applier) bodyDone(read bodyRead, body string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if h, ok := a.held[read.key]; ok && h.revision == read.revision && h.waiting != nil {
		state := h.waiting
		h.waiting = nil
		if err == nil {
			// put refuses only an ID that is not canonical, and this key's was accepted when its
			// inline text was put.
			_, _ = a.put(read.key, read.revision, a.rule.Document(state, body))
		}
	}
	a.complete(read.key, read.revision)
}

// put replaces key's text with doc at revision and returns what is held for key. A refused ID is
// returned as the state contract's noncanonical-ID error, with nothing changed.
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
	h.revision, h.words, h.waiting = revision, words, nil
	a.report()
	return h, nil
}

// forget drops what is held for key.
func (a *applier) forget(key string) {
	if h, ok := a.held[key]; ok {
		a.words -= h.words
		delete(a.held, key)
	}
}

// report sets the size gauges to what the index holds.
func (a *applier) report() {
	a.metrics.entities.Set(float64(a.index.Len()))
	a.metrics.words.Set(float64(a.words))
}
